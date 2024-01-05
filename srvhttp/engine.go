package srvhttp

import (
	"net/http"
	"strings"
	"time"

	"github.com/gorilla/mux"
	"github.com/welllog/golt/contract"
	"github.com/welllog/olog"
)

type Engine struct {
	Router
	logger  contract.Logger
	rspFunc ResponseFunc
	debug   bool
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
		if request.Method == http.MethodOptions {
			cc.apply(request, writer)
			writer.WriteHeader(http.StatusNoContent)
			return
		}

		writer.WriteHeader(http.StatusMethodNotAllowed)
	}))

	e.r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			cc.apply(request, writer)

			if request.Method == http.MethodOptions {
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
				handler.ServeHTTP(ctx, request)
				e.requestDebug(ctx.status, time.Since(start), request, ctx.err)
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
	if e.debug {
		return func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				start := time.Now()
				ctx := NewContext(writer, request)
				next.ServeHTTP(ctx, ctx.Request)

				if !ctx.Written() {
					e.rspFunc(ctx.rsp, ctx.err, ctx)
				}
				e.requestDebug(ctx.status, time.Since(start), request, ctx.err)
			})
		}
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			ctx := NewContext(writer, request)
			next.ServeHTTP(ctx, ctx.Request)

			if !ctx.Written() {
				e.rspFunc(ctx.rsp, ctx.err, ctx)
			}
		})
	}
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
