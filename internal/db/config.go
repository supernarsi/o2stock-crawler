package db

import (
	"errors"
	"fmt"
	"os"
)

// Config holds DB configuration.
type Config struct {
	Host     string
	Port     string
	User     string
	Password string
	DBName   string

	// Wechat config
	WxAppID     string
	WxAppSecret string
	JWTSecret   string
}

// LoadConfigFromEnv loads MySQL configuration from env vars.
//
// Defaults (same as design doc):
//   - DB_HOST: 127.0.0.1
//   - DB_PORT: 3306
//   - DB_USER: root
//   - DB_PASS:
//   - DB_NAME: ol2
//   - WX_APP_ID:
//   - WX_APP_SECRET:
//   - JWT_SECRET: default_secret
func LoadConfigFromEnv() (*Config, error) {
	host := getenvDefault("DB_HOST", "127.0.0.1")
	port := getenvDefault("DB_PORT", "3306")
	user := getenvDefault("DB_USER", "root")
	pass := getenvDefault("DB_PASS", "")
	dbname := getenvDefault("DB_NAME", "")

	wxAppID := getenvDefault("WX_APP_ID", "")
	wxAppSecret := getenvDefault("WX_APP_SECRET", "")
	jwtSecret := getenvDefault("JWT_SECRET", "default_secret")

	if host == "" || port == "" || user == "" || dbname == "" {
		return nil, errors.New("invalid db config")
	}

	return &Config{
		Host:        host,
		Port:        port,
		User:        user,
		Password:    pass,
		DBName:      dbname,
		WxAppID:     wxAppID,
		WxAppSecret: wxAppSecret,
		JWTSecret:   jwtSecret,
	}, nil
}

func getenvDefault(key, def string) string {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	return v
}

// DSN returns MySQL DSN string.
func (c *Config) DSN() string {
	// parseTime=true so we can use time.Time with datetime/date
	params := "charset=utf8mb4&parseTime=true&loc=Local"
	return fmt.Sprintf("%s:%s@tcp(%s:%s)/%s?%s",
		c.User, c.Password, c.Host, c.Port, c.DBName, params,
	)
}
