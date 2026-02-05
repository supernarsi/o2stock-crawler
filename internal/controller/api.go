package controller

import (
	"o2stock-crawler/internal/db"
	"o2stock-crawler/internal/db/repositories"
	"o2stock-crawler/internal/service"
)

// API 是控制器的主要结构体
type API struct {
	db               *db.DB
	playersService   *service.PlayersService
	userPlayerService *service.UserPlayerService
	itemsService     *service.ItemsService
	ipiRepo          *repositories.IPIRepository
	playerRepo       *repositories.PlayerRepository
}

// NewAPI 创建新的 API 控制器实例
func NewAPI(database *db.DB) *API {
	return &API{
		db:                database,
		playersService:    service.NewPlayersService(database),
		userPlayerService: service.NewUserPlayerService(database),
		itemsService:      service.NewItemsService(database),
		ipiRepo:           repositories.NewIPIRepository(database.DB),
		playerRepo:        repositories.NewPlayerRepository(database.DB),
	}
}
