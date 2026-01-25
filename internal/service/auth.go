package service

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"time"

	"o2stock-crawler/internal/db"
	"o2stock-crawler/internal/db/models"

	"github.com/golang-jwt/jwt/v5"
	jsoniter "github.com/json-iterator/go"
)

type AuthService struct {
	db        *db.DB
	dbConfig  *db.Config
	userQuery *db.UserQuery
	userCmd   *db.UserCommand
}

func NewAuthService(database *db.DB, dbConfig *db.Config) *AuthService {
	return &AuthService{
		db:        database,
		dbConfig:  dbConfig,
		userQuery: db.NewUserQuery(database),
		userCmd:   db.NewUserCommand(database),
	}
}

// WechatLoginUserInfo 微信登录用户信息
type WechatLoginUserInfo struct {
	Code   string `json:"code"`
	Nick   string `json:"nick"`
	Avatar string `json:"avatar"`
}

// WechatLoginResponse 微信登录接口响应
type WechatLoginResponse struct {
	OpenID     string `json:"openid"`
	SessionKey string `json:"session_key"`
	UnionID    string `json:"unionid"`
	ErrCode    int    `json:"errcode"`
	ErrMsg     string `json:"errmsg"`
}

// UserClaims 自定义 JWT Claims
type UserClaims struct {
	UserID uint `json:"user_id"`
	jwt.RegisteredClaims
}

// LoginWithWechat 使用微信 Code 登录
// 1. 调用微信接口获取 OpenID
// 2. 查询用户是否存在，不存在则注册
// 3. 生成 Token
func (s *AuthService) LoginWithWechat(ctx context.Context, info WechatLoginUserInfo) (*models.User, string, error) {
	// 1. 获取微信 OpenID
	wxResp, err := s.code2Session(ctx, info.Code)
	if err != nil {
		return nil, "", fmt.Errorf("wechat login failed: %w", err)
	}

	// 2. 查询或注册用户
	user, err := s.userQuery.GetUserByOpenID(ctx, s.db, wxResp.OpenID)
	if err != nil {
		return nil, "", err
	}

	if user == nil {
		// 注册新用户
		user = &models.User{
			WxOpenID:     wxResp.OpenID,
			WxUnionID:    wxResp.UnionID,
			WxSessionKey: wxResp.SessionKey,
			Nick:         info.Nick,
			Avatar:       info.Avatar,
		}
		if err := s.userCmd.CreateUser(ctx, s.db, user); err != nil {
			return nil, "", fmt.Errorf("create user failed: %w", err)
		}
	} else if user.Sta == 2 {
		return nil, "", fmt.Errorf("user is banned")
	}

	// 3. 生成 Token
	token, err := s.GenerateToken(user.ID)
	if err != nil {
		return nil, "", fmt.Errorf("generate token failed: %w", err)
	}

	return user, token, nil
}

// GenerateToken 生成 JWT Token
func (s *AuthService) GenerateToken(userID uint) (string, error) {
	claims := UserClaims{
		UserID: userID,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(7 * 24 * time.Hour)), // 7天过期
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			NotBefore: jwt.NewNumericDate(time.Now()),
			Issuer:    "o2stock-api",
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(s.dbConfig.JWTSecret))
}

// VerifyToken 验证 JWT Token 并返回 UserID
func (s *AuthService) VerifyToken(tokenString string) (uint, error) {
	token, err := jwt.ParseWithClaims(tokenString, &UserClaims{}, func(token *jwt.Token) (any, error) {
		// 验证签名算法
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return []byte(s.dbConfig.JWTSecret), nil
	})

	if err != nil {
		return 0, err
	}

	if claims, ok := token.Claims.(*UserClaims); ok && token.Valid {
		return claims.UserID, nil
	}

	return 0, fmt.Errorf("invalid token claims")
}

func (s *AuthService) code2Session(ctx context.Context, code string) (*WechatLoginResponse, error) {
	if s.dbConfig.WxAppID == "" || s.dbConfig.WxAppSecret == "" {
		return nil, fmt.Errorf("missing wechat config")
	}

	url := fmt.Sprintf("https://api.weixin.qq.com/sns/jscode2session?appid=%s&secret=%s&js_code=%s&grant_type=authorization_code",
		s.dbConfig.WxAppID, s.dbConfig.WxAppSecret, code)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var result WechatLoginResponse
	if err := jsoniter.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	log.Printf("wechat login resp: %+v", result)

	if result.ErrCode != 0 {
		return nil, fmt.Errorf("wechat api error: %d %s", result.ErrCode, result.ErrMsg)
	}

	return &result, nil
}
