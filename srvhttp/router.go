package srvhttp

import (
	"net/http"
	"os"

	"github.com/gorilla/mux"
)

type Router struct {
	r *mux.Router
}

type Route struct {
	r *mux.Route
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

func (r Router) POST(path string, handler Handler) Route {
	route := r.r.HandleFunc(path, wrapHandler(handler)).Methods(http.MethodPost)
	checkRoute(route)
	return Route{r: route}
}

func (r Router) GET(path string, handler Handler) Route {
	route := r.r.HandleFunc(path, wrapHandler(handler)).Methods(http.MethodGet)
	checkRoute(route)
	return Route{r: route}
}

func (r Router) PUT(path string, handler Handler) Route {
	route := r.r.HandleFunc(path, wrapHandler(handler)).Methods(http.MethodPut)
	checkRoute(route)
	return Route{r: route}
}

func (r Router) PATCH(path string, handler Handler) Route {
	route := r.r.HandleFunc(path, wrapHandler(handler)).Methods(http.MethodPatch)
	checkRoute(route)
	return Route{r: route}
}

func (r Router) DELETE(path string, handler Handler) Route {
	route := r.r.HandleFunc(path, wrapHandler(handler)).Methods(http.MethodDelete)
	checkRoute(route)
	return Route{r: route}
}

func (r Router) OPTIONS(path string, handler Handler) Route {
	route := r.r.HandleFunc(path, wrapHandler(handler)).Methods(http.MethodOptions)
	checkRoute(route)
	return Route{r: route}
}

func (r Router) HEAD(path string, handler Handler) Route {
	route := r.r.HandleFunc(path, wrapHandler(handler)).Methods(http.MethodHead)
	checkRoute(route)
	return Route{r: route}
}

func (r Router) Any(path string, handler Handler) Route {
	route := r.r.HandleFunc(path, wrapHandler(handler))
	checkRoute(route)
	return Route{r: route}
}

func (r Router) Static(relativePath, root string, listFiles bool) {
	r.StaticFS(relativePath, http.Dir(root), listFiles)
}

func (r Router) StaticFS(relativePath string, fs http.FileSystem, listFiles bool) {
	if !listFiles {
		fs = onlyFilesFS{fs}
	}

	checkRoute(r.r.PathPrefix(relativePath).Handler(http.StripPrefix(relativePath, http.FileServer(fs))))
}

func (r Route) Name(name string) {
	checkRoute(r.r.Name(name))
}

func wrapHandler(handler Handler) http.HandlerFunc {
	return func(writer http.ResponseWriter, request *http.Request) {
		ctx := writer.(*Context)
		ctx.rsp, ctx.err = handler(ctx)
	}
}

func checkRoute(route *mux.Route) {
	if err := route.GetError(); err != nil {
		panic(err.Error())
	}
}

type onlyFilesFS struct {
	fs http.FileSystem
}

type neuteredReaddirFile struct {
	http.File
}

// Open conforms to http.Filesystem.
func (fs onlyFilesFS) Open(name string) (http.File, error) {
	f, err := fs.fs.Open(name)
	if err != nil {
		return nil, err
	}
	return neuteredReaddirFile{f}, nil
}

// Readdir overrides the http.File default implementation.
func (f neuteredReaddirFile) Readdir(_ int) ([]os.FileInfo, error) {
	// this disables directory listing
	return nil, nil
}
