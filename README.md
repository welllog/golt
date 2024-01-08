<p align="center">
    <br> English | <a href="README.md">中文</a>
</p>

# golt
a simple http api development tool library, trying to be different from the development method of Go standard http
library, more concise and easy to use in api development

### srvhttp library
#### http handler
Go http standard library and other commonly used libraries
```
http.HandleFunc("/hello", func(w http.ResponseWriter, r *http.Request) {
    json.NewEncoder(w).Encode(map[string]string{"hello": "world"})
})
```
golt
```
engine := srvhttp.New()
engine.Any("/hello", func(ctx *srvhttp.Context) (any, error) {
    return map[string]string{"hello": "world"}, nil
})
// Output:
// {"data":{"hello":"world"}}

engine.POST("/error", func(ctx *Context) (any, error) {
    return nil, unierr.New(1000, "test error").WithData(map[string]int{"reason": 20})
})
// Output:
// {"code":1000,"msg":"test error","data":{"reason":20}}
```
#### http middleware
Common http routing middleware
```
middleware.Use(func(next http.Handler) http.Handler {
    return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
        next.ServeHTTP(writer, request)
    })
})
```
golt
```
engine.Use(func(ctx *srvhttp.Context, next srvhttp.Handler) (any, error) {
    ret, err := next(ctx)
    if err != nil {
        // todo
    }
    return ret, nil
})
```

### srvhttp overview
```
// init a http engine
engine := srvhttp.New()

// add router
user := engine.Group("/user")
user.POST("/login", loginHandler)
user.PATCH("/info", infoHandler)
user.Use(authMiddleware)

engine.GET("/index", indexHandler)
engine.Sub().GET("/index/menu", menuhandler)
engine.Sub().GET("/index/articles/{category}/{id:[0-9]+}", articleHandler)
engine.Static("/static", "./static", false)

// CORS middleware
engine.UseCors(srvhttp.CorsConfig{
    AllowPrivateNetwork: true,
    AllowCredentials:    true,
    AllowOrigins:        []string{"*127.0.0.1", "https://172.10.0.4"},
    AllowMethods:        []string{"*"},
    AllowHeaders:        []string{"*"},
    MaxAge:              12 * time.Hour,
})

srv := http.Server{
    Addr:    "0.0.0.0:8080",
    Handler: engine,
}

srv.ListenAndServe()
```

customer response handler
```
engine := srvhttp.New(
    srvhttp.WithResponseFunc(func(response any, err error, c *srvhttp.Context) {
        if err != nil {
            c.WriteHeader(http.StatusBadRequest)
            c.Write([]byte(err.Error()))
            return
        }
        json.NewEncoder(c).Encode(response)
    }),
)
```

If you need to return specific data in the http handler without using the general response function, you can use the srvhttp.Context directly like using http.ResponseWriter
```
engine.Any("/hello", func(ctx *srvhttp.Context) (any, error) {
    ctx.WriteHeader(http.StatusOK)
    ctx.Write([]byte("hello world"))
    // The return value will be captured by the middleware, if nil is returned here, the subsequent middleware will not get the result
    return nil, nil
})

engine.Any("page", func(ctx *srvhttp.Context) (any, error) {
    ctx.WriteHeader(http.StatusOK)
    ctx.Write([]byte("<html><body><h1>hello world</h1></body></html>"))
    // The value returned here will no longer appear in the response result, and the returned error is the same, but it will still be obtained by the middleware
    return "ok", nil
})
```