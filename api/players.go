package api

import "o2stock-crawler/internal/model"

// PlayerHistoryRes 球员历史价格响应
type PlayerHistoryRes struct {
	PlayerHistory []*model.PriceHistoryRow `json:"history"`
}
