package srvhttp

import (
	"net/http"
	"strings"

	"github.com/gorilla/mux"
	"github.com/welllog/golt/contract"
	"github.com/welllog/olog"
)

type Engine struct {
	Router
	logger  contract.Logger
	rspFunc ResponseFunc
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

func New(opts ...Option) *Engine {
	e := Engine{Router: Router{r: mux.NewRouter()}, rspFunc: defResponseFunc, logger: olog.GetLogger()}
	for _, opt := range opts {
		opt(&e)
	}

	e.r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			ctx := NewContext(writer, request)
			next.ServeHTTP(ctx, ctx.Request)

			if !ctx.Written() {
				e.rspFunc(ctx.rsp, ctx.err, ctx)
			}
		})
	})
	return &e
}

func (e *Engine) UseCors(c CorsConfig) {
	cc := &c
	cc.init()

	e.r.MethodNotAllowedHandler = http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method == http.MethodOptions {
			cc.apply(request, writer)
			return
		}

		writer.WriteHeader(http.StatusMethodNotAllowed)
	})

	e.r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			cc.apply(request, writer)

			if request.Method != http.MethodOptions {
				next.ServeHTTP(writer, request)
			}
		})
	})
}

func (e *Engine) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	e.r.ServeHTTP(w, req)
}

func (e *Engine) PrintRoutes() {
	_ = e.r.Walk(func(route *mux.Route, router *mux.Router, ancestors []*mux.Route) error {
		pathTemplate, _ := route.GetPathTemplate()
		queriesTemplates, _ := route.GetQueriesTemplates()
		methods, _ := route.GetMethods()

		e.logger.Debugf("ROUTE: %s%s; Methods: %s",
			pathTemplate, strings.Join(queriesTemplates, ","), strings.Join(methods, ","))

		return nil
	})
}
