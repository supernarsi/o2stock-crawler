package middleware

import (
	"net/http"
	"o2stock-crawler/api"
	"strings"
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
	// 1. 按路径分组路由
	routesByPath := make(map[string][]Route)
	for _, route := range r.routes {
		routesByPath[route.Path] = append(routesByPath[route.Path], route)
	}

	// 2. 为每个路径创建一个分发处理器
	for path, routes := range routesByPath {
		// 预处理：为每个路由构建完整的中间件链
		// 使用结构体存储构建好的处理链，避免在请求时重复构建
		type builtRoute struct {
			Method  string
			Handler http.HandlerFunc
		}

		// 这里的 routes 是循环变量，在闭包中使用需要注意，但在下面的循环中我们是构建新的切片，所以还好。
		// 但是为了安全起见，我们先构建好 builtRoutes 切片
		var currentBuiltRoutes []builtRoute

		for _, route := range routes {
			handler := route.Handler

			// 应用其他中间件（从后往前）
			for i := len(route.Middlewares) - 1; i >= 0; i-- {
				handler = route.Middlewares[i](handler)
			}

			// 注意：这里不再需要显式添加 MethodCheck 中间件，
			// 因为分发逻辑本身就会检查方法。

			currentBuiltRoutes = append(currentBuiltRoutes, builtRoute{
				Method:  route.Method,
				Handler: handler,
			})
		}

		// 3. 注册分发处理器到 mux
		// 需要将 currentBuiltRoutes 捕获到闭包中
		finalRoutes := currentBuiltRoutes

		mux.HandleFunc(path, func(w http.ResponseWriter, r *http.Request) {
			allowedMethods := make([]string, 0)

			for _, br := range finalRoutes {
				// 如果路由未指定方法，或方法匹配
				if br.Method == "" || br.Method == r.Method {
					br.Handler(w, r)
					return
				}
				if br.Method != "" {
					allowedMethods = append(allowedMethods, br.Method)
				}
			}

			// 如果没有匹配的方法，但有其他方法的路由存在
			if len(allowedMethods) > 0 {
				w.Header().Set("Allow", strings.Join(allowedMethods, ", "))
				w.WriteHeader(http.StatusMethodNotAllowed)
				writeJSON(w, api.Error(http.StatusMethodNotAllowed, "method not allowed"))
				return
			}

			// 如果没有任何路由匹配（理论上不应该发生，因为 path 匹配了）
			http.NotFound(w, r)
		})
	}
}
