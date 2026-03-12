package controller

import (
	"o2stock-crawler/internal/db"
	"o2stock-crawler/internal/db/repositories"
	"o2stock-crawler/internal/service"
)

// API 是控制器的主要结构体
type API struct {
	db                *db.DB
	cfg               *db.Config
	playersService    *service.PlayersService
	userPlayerService *service.UserPlayerService
	userItemService   *service.UserItemService
	itemsService      *service.ItemsService
	lineupService     *service.LineupAPIService
	feedbackService   *service.FeedbackService
	ipiRepo           *repositories.IPIRepository
	playerRepo        *repositories.PlayerRepository
	taskStatusRepo    *repositories.TaskStatusRepository
}

// NewAPI 创建新的 API 控制器实例
func NewAPI(database *db.DB, cfg *db.Config) *API {
	return &API{
		db:             database,
		cfg:            cfg,
		playersService: service.NewPlayersService(database),
		lineupService:  service.NewLineupAPIService(database),
		userPlayerService: service.NewUserPlayerService(
			database,
			repositories.NewOwnRepository(database.DB),
			repositories.NewPlayerRepository(database.DB),
			repositories.NewItemRepository(database.DB),
			repositories.NewFavRepository(database.DB),
		),
		userItemService: service.NewUserItemService(
			database,
			repositories.NewOwnRepository(database.DB),
			repositories.NewItemRepository(database.DB),
			repositories.NewItemFavRepository(database.DB),
		),
		itemsService:    service.NewItemsService(database),
		feedbackService: service.NewFeedbackService(database, cfg, repositories.NewFeedbackRepository(database.DB), repositories.NewUserRepository(database.DB)),
		ipiRepo:         repositories.NewIPIRepository(database.DB),
		playerRepo:     repositories.NewPlayerRepository(database.DB),
		taskStatusRepo: repositories.NewTaskStatusRepository(database.DB),
	}
}
