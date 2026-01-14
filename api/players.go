package api

import "o2stock-crawler/internal/model"

// PlayerHistoryRes 球员历史价格响应
type PlayerHistoryRes struct {
	PlayerInfo    *model.PlayerWithPriceChange `json:"player_info"`
	PlayerHistory []*model.PriceHistoryRow     `json:"history"`
}

// MultiPlayersHistoryRes 批量球员历史价格响应
type MultiPlayersHistoryRes struct {
	History []PlayerHistoryItem `json:"history"`
}

// PlayerHistoryItem 单个球员的历史价格项
type PlayerHistoryItem struct {
	PlayerID uint32                   `json:"player_id"`
	History  []*model.PriceHistoryRow `json:"history"`
}
