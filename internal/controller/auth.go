package controller

import (
	"context"
	"log"
	"net/http"
	"strings"

	"o2stock-crawler/api"
	"o2stock-crawler/internal/db"
	"o2stock-crawler/internal/middleware"
	"o2stock-crawler/internal/service"
)

type AuthController struct {
	authService *service.AuthService
}

func NewAuthController(database *db.DB, cfg *db.Config) *AuthController {
	return &AuthController{
		authService: service.NewAuthService(database, cfg),
	}
}

// Login 登录接口（新用户注册时记录 reg_os、reg_ip）
func (c *AuthController) Login() http.HandlerFunc {
	return middleware.API(func(r *http.Request) (any, *middleware.APIError) {
		var req struct {
			Code   string `json:"code"`
			Nick   string `json:"nickname"`
			Avatar string `json:"avatar"`
		}
		if err := middleware.DecodeJSONBody(r, &req); err != nil {
			return nil, &middleware.APIError{Status: http.StatusBadRequest, Code: http.StatusBadRequest, Msg: "invalid request body"}
		}

		if req.Code == "" {
			return nil, &middleware.APIError{Status: http.StatusBadRequest, Code: http.StatusBadRequest, Msg: "missing code"}
		}

		client := middleware.MustGetClient(r.Context())
		user, token, err := c.authService.LoginWithWechat(r.Context(), service.WechatLoginUserInfo{
			Code:   req.Code,
			Nick:   req.Nick,
			Avatar: "",
			RegOS:  client.OS,
			RegIP:  client.IP,
		})
		if err != nil {
			log.Printf("Login failed: [internal error]")
			return nil, &middleware.APIError{Status: http.StatusInternalServerError, Code: http.StatusInternalServerError, Msg: "登录失败，请稍后重试"}
		}

		return map[string]any{
			"user":  service.ToUserDTO(user),
			"token": token,
		}, nil
	})
}

// Middleware 鉴权中间件（强制登录）
func (c *AuthController) Middleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		authHeader := r.Header.Get("Authorization")
		if authHeader == "" {
			middleware.WriteJSON(w, api.Error(http.StatusUnauthorized, "missing authorization header"))
			return
		}

		parts := strings.Split(authHeader, " ")
		if len(parts) != 2 || parts[0] != "Bearer" {
			middleware.WriteJSON(w, api.Error(http.StatusUnauthorized, "invalid authorization format"))
			return
		}

		token := parts[1]
		userID, err := c.authService.VerifyToken(token)
		if err != nil {
			// 尝试刷新 Token (处理过期但仍在宽限期内的情况)
			if strings.Contains(err.Error(), "expired") {
				newToken, newUserID, refreshErr := c.authService.RefreshToken(token)
				if refreshErr == nil {
					// 刷新成功，下发新 Token 并继续请求
					w.Header().Set("X-New-Token", newToken)
					w.Header().Set("Access-Control-Expose-Headers", "X-New-Token") // 确保前端可以读取
					userID = newUserID
					goto SUCCESS
				} else {
					log.Printf("Refresh token failed: %v", refreshErr)
				}
			}
			if strings.Contains(err.Error(), "expired") {
				newToken, newUserID, refreshErr := c.authService.RefreshToken(token)
				log.Printf("Refresh token failed: [internal error] %s", refreshErr.Error())
				if refreshErr == nil {
					// 刷新成功，下发新 Token 并继续请求
					w.Header().Set("X-New-Token", newToken)
					w.Header().Set("Access-Control-Expose-Headers", "X-New-Token") // 确保前端可以读取
					userID = newUserID
					goto SUCCESS
				}
			}
			middleware.WriteJSON(w, api.Error(http.StatusUnauthorized, "invalid token: "+err.Error()))
			return
		}

		// 主动刷新：如果 Token 没过期但快过期了，也下发一个新 Token
		if c.authService.IsTokenNearExpiry(token) {
			if newToken, _, refreshErr := c.authService.RefreshToken(token); refreshErr == nil {
				w.Header().Set("X-New-Token", newToken)
				w.Header().Set("Access-Control-Expose-Headers", "X-New-Token")
			}
		}

	SUCCESS:
		// 将 UserID 注入 Context
		ctx := context.WithValue(r.Context(), "user_id", userID)
		next(w, r.WithContext(ctx))
	}
}

// OptionalMiddleware 可选鉴权中间件
// 如果 Header 中包含 Token 且验证通过，则注入 UserID
// 否则不注入（视为匿名用户），但不报错
func (c *AuthController) OptionalMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		authHeader := r.Header.Get("Authorization")
		if authHeader != "" {
			parts := strings.Split(authHeader, " ")
			if len(parts) == 2 && parts[0] == "Bearer" {
				token := parts[1]
				userID, err := c.authService.VerifyToken(token)
				if err == nil {
					// 主动刷新：快过期时下发新 Token
					if c.authService.IsTokenNearExpiry(token) {
						if newToken, _, refreshErr := c.authService.RefreshToken(token); refreshErr == nil {
							w.Header().Set("X-New-Token", newToken)
							w.Header().Set("Access-Control-Expose-Headers", "X-New-Token")
						}
					}
					// 验证通过，注入 Context
					ctx := context.WithValue(r.Context(), "user_id", userID)
					next(w, r.WithContext(ctx))
					return
				} else if strings.Contains(err.Error(), "expired") {
					// 尝试静默刷新
					if newToken, newUserID, refreshErr := c.authService.RefreshToken(token); refreshErr == nil {
						w.Header().Set("X-New-Token", newToken)
						w.Header().Set("Access-Control-Expose-Headers", "X-New-Token")
						ctx := context.WithValue(r.Context(), "user_id", newUserID)
						next(w, r.WithContext(ctx))
						return
					}
				}
			}
		}
		// 验证失败或无 Token，直接放行（不注入 user_id）
		next(w, r)
	}
}

// GetUserIDFromContext 从 Context 获取 UserID
func GetUserIDFromContext(ctx context.Context) (uint, bool) {
	id, ok := ctx.Value("user_id").(uint)
	return id, ok
}
