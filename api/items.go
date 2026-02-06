package api

import "o2stock-crawler/internal/dto"

// ItemsRes 道具列表响应
type ItemsRes struct {
	Items []dto.Item `json:"items"`
}

// ItemHistoryRes 单个道具历史价格响应
type ItemHistoryRes struct {
	ItemInfo dto.Item                   `json:"item_info"`
	History  []*dto.ItemPriceHistoryRow `json:"history"`
}

// ItemInReq 标记购买道具请求
type ItemInReq struct {
	ItemID     uint   `json:"item_id"`
	Num        uint   `json:"num"`
	Cost       uint   `json:"cost"`
	Dt         string `json:"dt"`          // 格式: 2006-01-02
	NotifyType uint8  `json:"notify_type"` // 0:不订阅 1:回本 2:盈利15%，可选，默认 0
}

// ItemOutReq 标记出售道具请求（指定持仓记录）
type ItemOutReq struct {
	OwnID  uint   `json:"own_id"`
	ItemID uint   `json:"item_id"`
	Cost   uint   `json:"cost"`
	Dt     string `json:"dt"` // 格式: 2006-01-02
}

// OwnedItem 用户拥有的道具（包含道具信息）
type OwnedItem struct {
	Id         uint     `json:"id" dc:"持仓记录 id"`
	ItemID     uint     `json:"item_id"`
	PriceIn    uint     `json:"price_in"`
	PriceOut   uint     `json:"price_out"`
	OwnSta     uint8    `json:"own_sta"`
	OwnNum     uint     `json:"own_num"`
	DtIn       string   `json:"dt_in"`
	DtOut      string   `json:"dt_out"`
	NotifyType uint8    `json:"notify_type"`
	Item       dto.Item `json:"item"`
}

// UserItemsRes 用户拥有道具列表响应
type UserItemsRes struct {
	Rosters []OwnedItem `json:"rosters"`
}
