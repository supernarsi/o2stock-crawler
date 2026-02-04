package api

import "o2stock-crawler/internal/dto"

// PlayerInReq 标记购买请求
type PlayerInReq struct {
	PlayerID   uint   `json:"player_id"`
	Num        uint   `json:"num"`
	Cost       uint   `json:"cost"`
	Dt         string `json:"dt"`          // 格式: 2006-01-02
	NotifyType uint8  `json:"notify_type"` // 0:不订阅 1:回本 2:盈利15%，可选，默认 0
}

// PlayerPriceNotifyReq 修改订阅请求
type PlayerPriceNotifyReq struct {
	PlayerID   uint  `json:"player_id"`
	NotifyType uint8 `json:"notify_type"` // 0:不订阅 1:回本 2:盈利15%
}

// PlayerOutReq 标记出售请求
type PlayerOutReq struct {
	PlayerID uint   `json:"player_id"`
	Cost     uint   `json:"cost"`
	Dt       string `json:"dt"` // 格式: 2006-01-02
}

// PlayerOwnReq 修改持仓记录请求
type PlayerOwnEditReq struct {
	RecordId uint   `json:"r_id" dc:"持仓记录 id"`
	PriceIn  uint   `json:"price_in" dc:"购买价格"`
	PriceOut uint   `json:"price_out" dc:"出售价格"`
	Num      uint   `json:"num" dc:"数量"`
	SoldOut  bool   `json:"sold_out" dc:"是否已出售"`
	DtIn     string `json:"dt_in" dc:"购买时间"`  // 格式: 2006-01-02
	DtOut    string `json:"dt_out" dc:"出售时间"` // 格式: 2006-01-02
}

// PlayerOwnDeleteReq 删除持仓记录请求
type PlayerOwnDeleteReq struct {
	RecordId uint `json:"r_id" dc:"持仓记录 id"`
}

// OwnedPlayer 用户拥有的球员（包含球员信息）
type OwnedPlayer struct {
	Id         uint        `json:"id" dc:"持仓记录 id"`
	PlayerID   uint        `json:"player_id"`
	PriceIn    uint        `json:"price_in"`
	PriceOut   uint        `json:"price_out"`
	OwnSta     uint8       `json:"own_sta"`
	OwnNum     uint        `json:"own_num"`
	DtIn       string      `json:"dt_in"`
	DtOut      string      `json:"dt_out"`
	NotifyType uint8       `json:"notify_type"`
	PP         dto.Players `json:"p_p"`
}

// UserPlayersRes 用户拥有球员列表响应
type UserPlayersRes struct {
	Rosters []OwnedPlayer `json:"rosters"`
}

// PlayersWithOwnedRes 球员列表响应（包含拥有信息）
type PlayersWithOwnedRes struct {
	Players []PlayerWithOwned `json:"players"`
}

// PlayerWithOwned 球员信息（包含拥有信息）
type PlayerWithOwned struct {
	dto.PlayerWithPriceChange
	Owned []*dto.OwnInfo `json:"owned"`
	IsFav bool           `json:"is_fav"`
}

// UserFavPlayerReq 用户收藏球员请求
type UserFavPlayerReq struct {
	PlayerID uint `json:"player_id"`
}
