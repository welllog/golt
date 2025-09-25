package etcdutil

import (
	"context"
	"errors"
	"fmt"
	"net"
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
	IfaceName     string
}

type Registrar struct {
	etcd   *clientv3.Client
	config RegistrarConfig
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
		etcd:   etcd,
		config: cfg,
	}
}

// RegisterService registers the service with etcd using a lease for TTL
// ctx cancel will stop the automatic refresh of the registration
func (r *Registrar) RegisterService(ctx context.Context, serviceName string, port int) (string, error) {
	var (
		ip  string
		err error
	)
	if r.config.IfaceName == "" {
		ip, err = GetLocalIP()
	} else {
		ip, err = GetLocalIPByName(r.config.IfaceName)
	}
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

	go r.keepAlive(ctx, key, host, leaseID)

	return key, nil
}

// DeregisterService cancels the registration. However, it does not stop the keep-alive;
// the context used in the RegisterService method needs to be canceled.
func (r *Registrar) DeregisterService(registerKey string) error {
	ctx, cancel := context.WithTimeout(context.Background(), r.config.OpTimeout)
	defer cancel()

	_, err := r.etcd.Delete(ctx, registerKey)
	if err != nil {
		return fmt.Errorf("[DeregisterService] etcd delete %s: %w", registerKey, err)
	}
	return nil
}

func (r *Registrar) keepAlive(ctx context.Context, key string, host string, initialLeaseID clientv3.LeaseID) {
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

func GetLocalIP() (string, error) {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return "", err
	}

	for _, address := range addrs {
		// Check the address type and loopback status
		if ipnet, ok := address.(*net.IPNet); ok && !ipnet.IP.IsLoopback() {
			if ipnet.IP.To4() != nil {
				return ipnet.IP.String(), nil
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
