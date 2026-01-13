package controller

import (
	"o2stock-crawler/internal/db"
	"o2stock-crawler/internal/service"
)

// API 是控制器的主要结构体
type API struct {
	playersService    *service.PlayersService
	userPlayerService *service.UserPlayerService
}

// NewAPI 创建新的 API 控制器实例
func NewAPI(database *db.DB) *API {
	return &API{
		playersService:    service.NewPlayersService(database),
		userPlayerService: service.NewUserPlayerService(database),
	}
}
