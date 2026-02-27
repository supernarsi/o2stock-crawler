package service

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"o2stock-crawler/internal/db"
	"o2stock-crawler/internal/db/repositories"
	"o2stock-crawler/internal/entity"

	"github.com/golang-jwt/jwt/v5"
	jsoniter "github.com/json-iterator/go"
	"gorm.io/gorm"
)

const (
	TokenExpiration       = 7 * 24 * time.Hour  // Token 有效期
	TokenRefreshThreshold = 2 * 24 * time.Hour  // 剩余多少时间触发刷新
	TokenGracePeriod      = 40 * 24 * time.Hour // 过期后多久内仍允许刷新（宽限期）
)

type AuthService struct {
	db       *db.DB
	dbConfig *db.Config
	userRepo *repositories.UserRepository
}

// WechatLoginUserInfo 微信登录用户信息
type WechatLoginUserInfo struct {
	Code   string `json:"code"`
	Nick   string `json:"nick"`
	Avatar string `json:"avatar"`
	RegOS  int    `json:"reg_os"` // 注册时的系统：1.iOS；2.安卓；3.鸿蒙；0.未知
	RegIP  []byte `json:"-"`      // 注册时的 IP（由服务端从请求解析，varbinary(16)）
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

func NewAuthService(database *db.DB, dbConfig *db.Config) *AuthService {
	return &AuthService{
		db:       database,
		dbConfig: dbConfig,
		userRepo: repositories.NewUserRepository(database.DB),
	}
}

// LoginWithWechat 使用微信 Code 登录
func (s *AuthService) LoginWithWechat(ctx context.Context, info WechatLoginUserInfo) (*entity.User, string, error) {
	wxResp, err := s.code2Session(ctx, info.Code)
	if err != nil {
		return nil, "", fmt.Errorf("wechat login failed: %w", err)
	}

	user, err := s.userRepo.GetByOpenID(ctx, wxResp.OpenID)
	if err != nil && err != gorm.ErrRecordNotFound {
		return nil, "", err
	}

	if user == nil {
		user = &entity.User{
			WxOpenID:     wxResp.OpenID,
			WxUnionID:    wxResp.UnionID,
			WxSessionKey: wxResp.SessionKey,
			Nick:         info.Nick,
			Avatar:       info.Avatar,
			Sta:          1,
			CTime:        time.Now(),
			RegOS:        info.RegOS,
			RegIP:        info.RegIP,
		}
		if err := s.userRepo.Create(ctx, user); err != nil {
			return nil, "", fmt.Errorf("create user failed: %w", err)
		}
	} else if user.Sta == 2 {
		return nil, "", fmt.Errorf("user is banned")
	}

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
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(TokenExpiration)),
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

// RefreshToken 尝试刷新 Token；即使 Token 已过期，只要在宽限期内且签名正确，就允许刷新
func (s *AuthService) RefreshToken(tokenString string) (string, uint, error) {
	_, err := jwt.ParseWithClaims(tokenString, &UserClaims{}, func(token *jwt.Token) (any, error) {
		return []byte(s.dbConfig.JWTSecret), nil
	}, jwt.WithExpirationRequired())
	if err != nil && !strings.Contains(err.Error(), "expired") {
		return "", 0, err
	}

	token, _, err := new(jwt.Parser).ParseUnverified(tokenString, &UserClaims{})
	if err != nil {
		return "", 0, err
	}
	claims, ok := token.Claims.(*UserClaims)
	if !ok {
		return "", 0, fmt.Errorf("invalid claims")
	}
	if time.Now().After(claims.ExpiresAt.Time.Add(TokenGracePeriod)) {
		return "", 0, fmt.Errorf("token expired and beyond grace period")
	}
	_, err = jwt.ParseWithClaims(tokenString, &UserClaims{}, func(token *jwt.Token) (any, error) {
		return []byte(s.dbConfig.JWTSecret), nil
	})
	if err != nil && err.Error() != "token has invalid claims: token is expired" && !strings.Contains(err.Error(), "expired") {
		return "", 0, err
	}
	newToken, err := s.GenerateToken(claims.UserID)
	if err != nil {
		return "", 0, err
	}
	return newToken, claims.UserID, nil
}

// IsTokenNearExpiry 检查 Token 是否接近过期
func (s *AuthService) IsTokenNearExpiry(tokenString string) bool {
	token, _, err := new(jwt.Parser).ParseUnverified(tokenString, &UserClaims{})
	if err != nil {
		return false
	}
	if claims, ok := token.Claims.(*UserClaims); ok {
		return time.Until(claims.ExpiresAt.Time) < TokenRefreshThreshold
	}
	return false
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
	log.Printf("wechat login resp: openid=%s, unionid=%s", result.OpenID, result.UnionID)
	if result.ErrCode != 0 {
		return nil, fmt.Errorf("wechat api error: %d %s", result.ErrCode, result.ErrMsg)
	}
	return &result, nil
}
