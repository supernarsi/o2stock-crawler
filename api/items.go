package api

import "o2stock-crawler/internal/dto"

// ItemsRes 道具列表响应
type ItemsRes struct {
	Items []dto.Item `json:"items"`
}

// ItemHistoryRes 单个道具历史价格响应
type ItemHistoryRes struct {
	ItemInfo dto.Item                 `json:"item_info"`
	History  []*dto.ItemPriceHistoryRow `json:"history"`
}
