package etcdutil

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/welllog/golib/randz"
	"github.com/welllog/golt/contract"
	"github.com/welllog/olog"
	clientv3 "go.etcd.io/etcd/client/v3"
)

const (
	defaultLeaseTTL      = 20              // default lease TTL in seconds
	defaultRetryInterval = 5 * time.Second // default retry interval
	defaultOpTimeout     = 3 * time.Second // default operation timeout
	maxBackoff           = 30 * time.Second
)

type RegistrarConfig struct {
	LeaseTTL      int64
	RetryInterval time.Duration
	OpTimeout     time.Duration
	MaxBackoff    time.Duration
	Logger        contract.Logger
	// IfaceName specifies the network interface to take the registered IP from.
	// If set but the interface does not exist or has no usable IP, registration
	// fails loudly instead of silently falling back to another address.
	IfaceName string
	// ServiceIP explicitly overrides the registered IP. It must be a valid IPv4
	// address; an invalid value is rejected at registration time.
	ServiceIP string
}

type Registrar struct {
	etcd    *clientv3.Client
	config  RegistrarConfig
	mu      sync.Mutex
	cancels map[string]context.CancelFunc
}

func NewRegister(etcd *clientv3.Client, cfg RegistrarConfig) *Registrar {
	if cfg.LeaseTTL <= 0 {
		cfg.LeaseTTL = defaultLeaseTTL
	}

	if cfg.RetryInterval <= 0 {
		cfg.RetryInterval = defaultRetryInterval
	}

	if cfg.OpTimeout <= 0 {
		cfg.OpTimeout = defaultOpTimeout
	}

	if cfg.MaxBackoff <= 0 {
		cfg.MaxBackoff = maxBackoff
	}

	if cfg.Logger == nil {
		cfg.Logger = olog.DynamicLogger{}
	}

	return &Registrar{
		etcd:    etcd,
		config:  cfg,
		cancels: make(map[string]context.CancelFunc),
	}
}

// RegisterService registers the service with etcd using a lease for TTL.
// ctx cancel or DeregisterService will stop the automatic refresh of the registration.
func (r *Registrar) RegisterService(ctx context.Context, serviceName string, port int) (string, error) {
	ip, err := r.getServiceIP()
	if err != nil {
		return "", fmt.Errorf("[RegisterService] register %s failed on get ip: %w", serviceName, err)
	}

	host := fmt.Sprintf("%s:%d", ip, port)
	randId := randz.Id().Base36()
	key := fmt.Sprintf("%s/%s", serviceName, randId)

	opCtx, opCancel := context.WithTimeout(ctx, r.config.OpTimeout)
	leaseID, err := r.register(opCtx, key, host)
	opCancel()
	if err != nil {
		return "", fmt.Errorf("[RegisterService] register %s failed on etcd operate: %w", serviceName, err)
	}

	keepCtx, keepCancel := context.WithCancel(ctx)
	r.mu.Lock()
	if oldCancel, exists := r.cancels[key]; exists {
		oldCancel()
	}
	r.cancels[key] = keepCancel
	r.mu.Unlock()

	go r.keepAlive(keepCtx, key, host, leaseID)

	return key, nil
}

// DeregisterService cancels the registration, stops the background keep-alive task,
// and removes the key from etcd.
func (r *Registrar) DeregisterService(registerKey string) error {
	r.mu.Lock()
	if cancel, ok := r.cancels[registerKey]; ok {
		cancel()
		delete(r.cancels, registerKey)
	}
	r.mu.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), r.config.OpTimeout)
	defer cancel()

	_, err := r.etcd.Delete(ctx, registerKey)
	if err != nil {
		return fmt.Errorf("[DeregisterService] etcd delete %s: %w", registerKey, err)
	}
	return nil
}

func (r *Registrar) keepAlive(ctx context.Context, key string, host string, initialLeaseID clientv3.LeaseID) {
	defer func() {
		r.mu.Lock()
		delete(r.cancels, key)
		r.mu.Unlock()
	}()

	leaseID := initialLeaseID
	backoff := r.config.RetryInterval

	for {
		ch, err := r.etcd.KeepAlive(ctx, leaseID)
		if err != nil {
			r.config.Logger.Errorf("[RegisterService] %s etcd keep alive failed: %v", key, err)
		} else {
			for range ch {
				// eat messages until keep alive channel closes
			}
			r.config.Logger.Infof("[RegisterService] %s etcd keep alive closed", key)
		}

		timer := time.NewTimer(backoff)
	registerLoop:
		for {
			select {
			case <-ctx.Done():
				timer.Stop()
				r.config.Logger.Infof("[RegisterService] %s context done, stopping keep alive", key)
				return
			case <-timer.C:
				opCtx, opCancel := context.WithTimeout(ctx, r.config.OpTimeout)
				newLeaseID, err := r.register(opCtx, key, host)
				opCancel()
				if err != nil {
					r.config.Logger.Errorf("[RegisterService] %s etcd re-register failed: %v", key, err)
					backoff *= 2
					if backoff > r.config.MaxBackoff {
						backoff = r.config.MaxBackoff
					}
					timer.Reset(backoff)
					continue
				}
				r.config.Logger.Infof("[RegisterService] %s etcd re-register success", key)
				leaseID = newLeaseID
				backoff = r.config.RetryInterval
				break registerLoop
			}
		}
	}
}

func (r *Registrar) register(ctx context.Context, key, host string) (clientv3.LeaseID, error) {
	leaseRsp, err := r.etcd.Grant(ctx, r.config.LeaseTTL)
	if err != nil {
		return 0, err
	}

	_, err = r.etcd.Put(ctx, key, host, clientv3.WithLease(leaseRsp.ID))
	if err != nil {
		_, _ = r.etcd.Revoke(ctx, leaseRsp.ID)
		return 0, err
	}

	return leaseRsp.ID, nil
}

func (r *Registrar) getServiceIP() (string, error) {
	if r.config.ServiceIP != "" {
		if ip := net.ParseIP(r.config.ServiceIP); ip == nil || ip.To4() == nil {
			return "", fmt.Errorf("invalid ServiceIP %q: must be an IPv4 address", r.config.ServiceIP)
		}
		return r.config.ServiceIP, nil
	}

	if r.config.IfaceName != "" {
		return GetLocalIPByName(r.config.IfaceName)
	}

	// Probe the route to each etcd endpoint (1s timeout each, serial) so the
	// registered IP is the one that can actually reach etcd.
	for _, ep := range safeEndpoints(r.etcd) {
		if ip, err := GetOutboundIP(ep); err == nil {
			parsedIP := net.ParseIP(ip)
			if parsedIP != nil && !parsedIP.IsLoopback() {
				return ip, nil
			}
		}
	}

	if ip, err := GetOutboundIP(); err == nil {
		parsedIP := net.ParseIP(ip)
		if parsedIP != nil && !parsedIP.IsLoopback() {
			return ip, nil
		}
	}

	return GetLocalIP()
}

func safeEndpoints(c *clientv3.Client) []string {
	if c == nil {
		return nil
	}
	return c.Endpoints()
}

// GetOutboundIP determines the outbound IPv4 address by probing the route to a target address.
// If target is omitted, it defaults to "8.8.8.8:80".
func GetOutboundIP(target ...string) (string, error) {
	dst := "8.8.8.8:80"
	if len(target) > 0 && target[0] != "" {
		dst = target[0]
	}
	return getOutboundIP(dst)
}

func getOutboundIP(target string) (string, error) {
	target = strings.TrimPrefix(target, "http://")
	target = strings.TrimPrefix(target, "https://")
	if idx := strings.IndexByte(target, '/'); idx != -1 {
		target = target[:idx]
	}

	host, _, err := net.SplitHostPort(target)
	if err != nil {
		target = net.JoinHostPort(target, "80")
	} else if host == "" {
		return "", errors.New("invalid target: empty host")
	}

	conn, err := net.DialTimeout("udp", target, time.Second)
	if err != nil {
		return "", err
	}
	defer conn.Close()

	localAddr, ok := conn.LocalAddr().(*net.UDPAddr)
	if !ok || localAddr.IP == nil {
		return "", errors.New("cannot determine outbound ip")
	}

	ipv4 := localAddr.IP.To4()
	if ipv4 == nil {
		return "", errors.New("outbound ip is not ipv4")
	}

	return ipv4.String(), nil
}

var virtualInterfacePrefixes = []string{
	"docker", "veth", "br-", "cni", "flannel", "calico", "tun", "utun", "tap", "wg",
}

func isVirtualInterface(name string) bool {
	for _, prefix := range virtualInterfacePrefixes {
		if strings.HasPrefix(name, prefix) {
			return true
		}
	}
	return false
}

func GetLocalIP() (string, error) {
	interfaces, err := net.Interfaces()
	if err != nil {
		return "", err
	}

	var fallbackIP string

	// Pass 1: Look for private IPv4 on active, non-virtual, non-loopback interfaces
	for _, iface := range interfaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		if isVirtualInterface(iface.Name) {
			continue
		}

		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}

		for _, addr := range addrs {
			ipnet, ok := addr.(*net.IPNet)
			if !ok || ipnet.IP.IsLoopback() {
				continue
			}

			ipv4 := ipnet.IP.To4()
			if ipv4 == nil {
				continue
			}

			if ipv4.IsPrivate() {
				return ipv4.String(), nil
			}

			if fallbackIP == "" {
				fallbackIP = ipv4.String()
			}
		}
	}

	if fallbackIP != "" {
		return fallbackIP, nil
	}

	// Pass 2: Fallback to any non-loopback IPv4, including virtual interfaces
	for _, iface := range interfaces {
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}

		for _, addr := range addrs {
			ipnet, ok := addr.(*net.IPNet)
			if !ok || ipnet.IP.IsLoopback() {
				continue
			}
			if ipv4 := ipnet.IP.To4(); ipv4 != nil {
				return ipv4.String(), nil
			}
		}
	}

	return "", errors.New("no local ip found")
}

func GetLocalIPByName(ifaceName string) (string, error) {
	interfaces, err := net.Interfaces()
	if err != nil {
		return "", err
	}
	for _, iface := range interfaces {
		if iface.Name == ifaceName && iface.Flags&net.FlagUp != 0 {
			addrs, err := iface.Addrs()
			if err != nil {
				return "", err
			}
			for _, addr := range addrs {
				if ipnet, ok := addr.(*net.IPNet); ok && !ipnet.IP.IsLoopback() {
					if ipnet.IP.To4() != nil {
						return ipnet.IP.String(), nil
					}
				}
			}
		}
	}
	return "", errors.New("no ip found for interface " + ifaceName)
}
