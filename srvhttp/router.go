package srvhttp

import (
	"net/http"

	"github.com/gorilla/mux"
)

type Router struct {
	r *mux.Router
}

func (r Router) Use(mds ...Middleware) {
	if len(mds) == 0 {
		return
	}

	r.r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			var (
				chainHandler Handler
				index        int
			)

			ctx := writer.(*Context)
			num := len(mds)

			chainHandler = func(ctx *Context) (any, error) {
				if index < num {
					md := mds[index]
					index++
					return md(ctx, chainHandler)
				}

				next.ServeHTTP(ctx, ctx.Request)
				return ctx.rsp, ctx.err
			}

			ctx.rsp, ctx.err = chainHandler(ctx)
		})
	})
}

func (r Router) Group(pathPrefix string) Router {
	return Router{r: r.r.PathPrefix(pathPrefix).Subrouter()}
}

func (r Router) Sub() Router {
	return Router{r: r.r.NewRoute().Subrouter()}
}

func (r Router) POST(path string, handler Handler) {
	r.r.HandleFunc(path, wrapHandler(handler)).Methods(http.MethodPost)
}

func (r Router) GET(path string, handler Handler) {
	r.r.HandleFunc(path, wrapHandler(handler)).Methods(http.MethodGet)
}

func (r Router) PUT(path string, handler Handler) {
	r.r.HandleFunc(path, wrapHandler(handler)).Methods(http.MethodPut)
}

func (r Router) PATCH(path string, handler Handler) {
	r.r.HandleFunc(path, wrapHandler(handler)).Methods(http.MethodPatch)
}

func (r Router) DELETE(path string, handler Handler) {
	r.r.HandleFunc(path, wrapHandler(handler)).Methods(http.MethodDelete)
}

func (r Router) OPTIONS(path string, handler Handler) {
	r.r.HandleFunc(path, wrapHandler(handler)).Methods(http.MethodOptions)
}

func (r Router) HEAD(path string, handler Handler) {
	r.r.HandleFunc(path, wrapHandler(handler)).Methods(http.MethodHead)
}

func (r Router) Any(path string, handler Handler) {
	r.r.HandleFunc(path, wrapHandler(handler))
}

func wrapHandler(handler Handler) http.HandlerFunc {
	return func(writer http.ResponseWriter, request *http.Request) {
		ctx := writer.(*Context)
		ctx.rsp, ctx.err = handler(ctx)
	}
}