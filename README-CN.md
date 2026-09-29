<p align="center">
    <br> <a href="README.md">English</a> | 中文
</p>

# golt
一个简单的http api开发工具库，尝试做到区别于Go标准http库的开发方式，在api开发上更简洁、易用

### srvhttp库
#### http handler
Go http标准库及其它常用库
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
    return nil, unierr.New(1000, "test error").SetData(map[string]int{"reason": 20})
})
// Output:
// {"code":1000,"msg":"test error","data":{"reason":20}}
```
#### http middleware
常见http路由中间件
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

### srvhttp 使用概览
```
// 初始化一个http engine
engine := srvhttp.New()

// 添加路由
user := engine.Group("/user")
user.POST("/login", loginHandler)
user.PATCH("/info", infoHandler)
user.Use(authMiddleware)

engine.GET("/index", indexHandler)
engine.Sub().GET("/index/menu", menuhandler)
engine.Sub().GET("/index/articles/{category}/{id:[0-9]+}", articleHandler)
engine.Static("/static", "./static", false)

// 跨域
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

自定义响应处理函数
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

如果在http handler中需要返回特定数据而不使用通用响应函数，可以像使用http.ResponseWriter一样直接使用srvhttp.Context
```
engine.Any("/hello", func(ctx *srvhttp.Context) (any, error) {
    ctx.WriteHeader(http.StatusOK)
    ctx.Write([]byte("hello world"))
    // 返回值会被中间件捕捉，如果此处返回nil,则后续中间件将拿不到结果
    return nil, nil
})

engine.Any("/page", func(ctx *srvhttp.Context) (any, error) {
    ctx.WriteHeader(http.StatusOK)
    ctx.Write([]byte("<html><body><h1>hello world</h1></body></html>"))
    // 此处返回的值将不再出现在响应结果中，返回的错误同理,但仍会被中间获取到
    return "ok", nil
})
```

### config 库
golt的config库提供了统一的配置管理，支持从文件、etcd加载配置，支持动态加载配置，支持配置更新通知。
其读取源需要一个额外的文件配置，config.FromFile("config.yaml"),其中config.yaml中指定了读取配置的源以及映射方式

#### 从文件加载配置
从文件读取配置的
```yaml
  # 加载配置的源为文件系统
  - source: file://
    configs:
      # 命名空间，决定了该配置从哪个文件中读取
      - namespace: test/demo1 | test/demo2
        # 命名空间指向的文件
        path: test1.yaml
        # 是否监听该文件变动来动态加载配置
        watch: true
```
#### 从etcd加载配置
```yaml
  # 加载配置的源为etcd以及地址
  - source: etcd://127.0.0.1:2379
    configs:
      # 命名空间，决定了该配置从哪个etcd及其key path中读取
      - namespace: test/demo4
        # 当前etcd下的key path
        path: /v1/test/demo4/
        # 是否监听该key path变动来动态加载配置
        watch: true
```
#### 从自定义etcd客户端加载配置
使用自定义 etcd 客户端时，source 仍然写 `etcd://`，客户端通过 `WithCustomEtcdClient` 传入，
所有 `etcd://` 源都会复用该客户端，不再单独建连：
```go
c, err := config.FromFile("./config.yaml", config.WithCustomEtcdClient(cli))
```
#### config使用概览
```go
c, err := FromFile("./config.yaml")
if err != nil {
    panic(err)
}
defer c.Close()

ctx := context.Background()
c.String(ctx, "test/demo1", "app_name")
c.Int64(ctx, "test/demo2", "retry")
c.Int(ctx, "test/demo2", "retry")
c.Float64(ctx, "test/demo4", "rate")
c.Bool(ctx, "test/demo4", "enable")

// 默认值 Fallback
timeout := c.IntOr(ctx, "test/demo2", "timeout", 3000)
host := c.StringOr(ctx, "test/demo1", "host", "127.0.0.1")

// 命名空间作用域 (ScopedConfigure)
demo1 := c.Namespace("test/demo1")
appName, _ := demo1.String(ctx, "app_name")
appPort := demo1.IntOr(ctx, "port", 8080)

c.YamlDecode(ctx, "test/demo1", "log", &logConf)
c.JsonDecode(ctx, "test/demo4", "data", &data)
c.TomlDecode(ctx, "test/demo5", "data", &data)
c.Decode(ctx, "test/demo4", "data", &data, json.Unmarshal)

c.GetRaw(ctx, "test/demo1", "app_name")
c.UnsafeGetRaw(ctx, "test/demo1", "app_name")
c.GetRawString(ctx, "test/demo1", "app_name")

// 监听变更
err = c.OnKeyChange("test/demo1", "app_name", func([]byte) error {
    // do something
    return nil
})
// err 非 nil 表示 namespace 不存在，或该 namespace 未在配置中开启 watch
```
