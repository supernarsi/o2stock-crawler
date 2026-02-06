package dto

import "time"

// Item 道具 API 响应 DTO
type Item struct {
	ItemID             uint           `json:"item_id"`
	Name               string         `json:"name"`
	Desc               string         `json:"desc"`
	Icon               string         `json:"icon"`
	PriceStandard      uint           `json:"price_standard"`
	PriceCurrentLowest uint           `json:"price_current_lowest"`
	PriceChange1d      float64        `json:"price_change_1d"`
	PriceChange7d      float64        `json:"price_change_7d"`
	Owned              []*ItemOwnInfo `json:"owned"`
}

// ItemOwnInfo 简化版道具持仓信息 DTO（用于 items/item-history 的 owned 字段）
type ItemOwnInfo struct {
	ItemID     uint   `json:"item_id"`
	PriceIn    uint   `json:"price_in"`
	PriceOut   uint   `json:"price_out"`
	OwnSta     uint8  `json:"own_sta"`
	OwnNum     uint   `json:"own_num"`
	DtIn       string `json:"dt_in"`
	DtOut      string `json:"dt_out"`
	NotifyType uint8  `json:"notify_type"` // 0:不订阅 1:回本 2:盈利15%；非“已购买”时返回 0
}

// ItemPriceHistoryRow 道具价格历史行 DTO
type ItemPriceHistoryRow struct {
	ItemID           uint      `json:"item_id"`
	AtDate           time.Time `json:"at_date"`
	AtDateHourStr    string    `json:"at_date_hour"`
	PriceStandard    uint32    `json:"price_standard"`
	PriceCurrentSale uint32    `json:"price_current_sale"`
}
