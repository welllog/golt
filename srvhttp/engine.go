package srvhttp

import (
	"fmt"
	"net/http"
	"runtime/debug"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/mux"
	"github.com/welllog/golt/contract"
	"github.com/welllog/golt/unierr"
	"github.com/welllog/olog"
)

type Engine struct {
	Router
	logger  contract.Logger
	rspFunc ResponseFunc
	debug   bool
	pool    sync.Pool
}

type Option func(*Engine)

func WithResponseFunc(rspFunc ResponseFunc) Option {
	return func(e *Engine) {
		e.rspFunc = rspFunc
	}
}

func WithLogger(logger contract.Logger) Option {
	return func(e *Engine) {
		e.logger = logger
	}
}

func WithDebug(open bool) Option {
	return func(e *Engine) {
		e.debug = open
	}
}

func New(opts ...Option) *Engine {
	e := Engine{Router: Router{r: mux.NewRouter()}, rspFunc: defResponseFunc, logger: olog.GetLogger()}
	e.pool.New = func() any {
		return &Context{}
	}
	for _, opt := range opts {
		opt(&e)
	}

	e.initNotFoundHandler()
	e.initMethodNotAllowedHandler(nil)
	e.loadMustMiddlewares()

	return &e
}

func (e *Engine) UseCors(c CorsConfig) {
	cc := &c
	cc.init()

	e.initMethodNotAllowedHandler(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if isPreflight(request) {
			cc.apply(request, writer)
			writer.WriteHeader(http.StatusNoContent)
			return
		}

		writer.WriteHeader(http.StatusMethodNotAllowed)
	}))

	e.r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			cc.apply(request, writer)

			// only cross-origin preflights are answered here; plain OPTIONS
			// requests are routed to the handler
			if isPreflight(request) {
				writer.WriteHeader(http.StatusNoContent)
				return
			}

			next.ServeHTTP(writer, request)
		})
	})
}

func (e *Engine) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	e.r.ServeHTTP(w, req)
}

// PrintRoutes logs the registered routes at debug level.
func (e *Engine) PrintRoutes() {
	_ = e.r.Walk(func(route *mux.Route, router *mux.Router, ancestors []*mux.Route) error {
		pathTemplate, _ := route.GetPathTemplate()
		methods, _ := route.GetMethods()
		method := "Any"
		if len(methods) > 0 {
			method = strings.Join(methods, ",")
		}
		name := route.GetName()
		if name != "" {
			name = "Name=" + name
		}

		e.logger.Debugf("ROUTE=%s; Methods=%s; %s",
			pathTemplate, method, name)

		return nil
	})
}

func (e *Engine) initNotFoundHandler() {
	if e.debug {
		e.r.NotFoundHandler = http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			e.requestDebug(http.StatusNotFound, 0, request, nil)
			writer.WriteHeader(http.StatusNotFound)
		})
	}
}

func (e *Engine) initMethodNotAllowedHandler(handler http.Handler) {
	if e.debug {
		if handler != nil {
			e.r.MethodNotAllowedHandler = http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				start := time.Now()
				ctx := NewContext(writer, request)
				handler.ServeHTTP(ctx, ctx.Request)
				e.requestDebug(ctx.status, time.Since(start), ctx.Request, ctx.err)
			})

			return
		}

		e.r.MethodNotAllowedHandler = http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			e.requestDebug(http.StatusMethodNotAllowed, 0, request, nil)
			writer.WriteHeader(http.StatusMethodNotAllowed)
		})

		return
	}

	if handler != nil {
		e.r.MethodNotAllowedHandler = handler
	}
}

func (e *Engine) loadMustMiddlewares() {
	e.r.Use(e.middlewareOne())
}

func (e *Engine) middlewareOne() mux.MiddlewareFunc {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			start := time.Now()
			ctx := e.acquireContext(writer, request)
			defer e.releaseContext(ctx)

			// registered first, so it also guards the response writing below
			defer func() {
				if r := recover(); r != nil {
					e.logger.Errorf("panic: %v\n%s", r, debug.Stack())

					// the connection may already be written or hijacked
					if !ctx.Written() {
						msg := "internal server error"
						if e.debug {
							msg = fmt.Sprint(r)
						}
						ue := unierr.New(unierr.Internal, msg).SetHttpCode(http.StatusInternalServerError)
						e.rspFunc(nil, ue, ctx)
					}
				}
			}()

			next.ServeHTTP(ctx, ctx.Request)

			if !ctx.Written() {
				e.rspFunc(ctx.rsp, ctx.err, ctx)
			}
			if e.debug {
				e.requestDebug(ctx.status, time.Since(start), request, ctx.err)
			}
		})
	}
}

func (e *Engine) acquireContext(w http.ResponseWriter, req *http.Request) *Context {
	c := e.pool.Get().(*Context)
	c.reset(w, req)
	return c
}

func (e *Engine) releaseContext(c *Context) {
	if c.status == http.StatusSwitchingProtocols {
		return
	}
	c.clean()
	e.pool.Put(c)
}

func (e *Engine) requestDebug(httpCode int, cost time.Duration, request *http.Request, err error) {
	if err != nil {
		e.logger.Warnf(
			"|%d| %fms | %s |%s|%s|%s",
			httpCode, float64(cost.Microseconds())/1000, ClientIP(request), request.Method, request.RequestURI, err.Error())
		return
	}

	e.logger.Debugf(
		"|%d| %fms | %s |%s|%s|",
		httpCode, float64(cost.Microseconds())/1000, ClientIP(request), request.Method, request.RequestURI)
}
