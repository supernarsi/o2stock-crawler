package controller

import (
	"o2stock-crawler/internal/db"
)

// API 是控制器的主要结构体
type API struct {
	db *db.DB
}

// NewAPI 创建新的 API 控制器实例
func NewAPI(database *db.DB) *API {
	return &API{db: database}
}
