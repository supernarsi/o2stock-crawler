package middleware

import (
	"net/http"
	"o2stock-crawler/api"
)

// Route 定义路由和对应的处理器
type Route struct {
	Path        string
	Method      string // HTTP 方法，空字符串表示允许所有方法
	Handler     http.HandlerFunc
	Middlewares []Middleware
}

// Router 管理路由和中间件
type Router struct {
	globalMiddlewares []Middleware
	routes            []Route
}

// NewRouter 创建新的路由器
func NewRouter(globalMiddlewares ...Middleware) *Router {
	return &Router{
		globalMiddlewares: globalMiddlewares,
		routes:            make([]Route, 0),
	}
}

// Register 注册路由，可以指定额外的中间件
func (r *Router) Register(path string, handler http.HandlerFunc, middlewares ...Middleware) {
	r.routes = append(r.routes, Route{
		Path:        path,
		Method:      "", // 默认允许所有方法
		Handler:     handler,
		Middlewares: middlewares,
	})
}

// RegisterAPI 注册 API 路由（自动应用全局中间件）
// method 参数指定允许的 HTTP 方法，空字符串表示允许所有方法
func (r *Router) RegisterAPI(path string, handler http.HandlerFunc, method string, middlewares ...Middleware) {
	// 合并全局中间件和路由特定中间件
	allMiddlewares := append(r.globalMiddlewares, middlewares...)
	r.routes = append(r.routes, Route{
		Path:        path,
		Method:      method,
		Handler:     handler,
		Middlewares: allMiddlewares,
	})
}

// MethodCheck 中间件：检查 HTTP 方法
func MethodCheck(allowedMethod string) Middleware {
	return func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			if allowedMethod != "" && r.Method != allowedMethod {
				w.WriteHeader(http.StatusMethodNotAllowed)
				writeJSON(w, api.Error(http.StatusMethodNotAllowed, "method not allowed"))
				return
			}
			next(w, r)
		}
	}
}

// Apply 将路由应用到 mux
func (r *Router) Apply(mux *http.ServeMux) {
	for _, route := range r.routes {
		handler := route.Handler

		// 应用其他中间件（从后往前）
		for i := len(route.Middlewares) - 1; i >= 0; i-- {
			handler = route.Middlewares[i](handler)
		}

		// 如果指定了方法，添加方法检查中间件（最后应用，最先执行）
		if route.Method != "" {
			handler = MethodCheck(route.Method)(handler)
		}

		mux.HandleFunc(route.Path, handler)
	}
}
