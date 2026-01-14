package api

import "o2stock-crawler/internal/model"

// PlayerInReq 标记购买请求
type PlayerInReq struct {
	PlayerID uint   `json:"player_id"`
	Num      uint   `json:"num"`
	Cost     uint   `json:"cost"`
	Dt       string `json:"dt"` // 格式: 2006-01-02 15:04:05
}

// PlayerOutReq 标记出售请求
type PlayerOutReq struct {
	PlayerID uint   `json:"player_id"`
	Cost     uint   `json:"cost"`
	Dt       string `json:"dt"` // 格式: 2006-01-02 15:04:05
}

// OwnedPlayer 用户拥有的球员（包含球员信息）
type OwnedPlayer struct {
	PlayerID uint          `json:"player_id"`
	PriceIn  uint          `json:"price_in"`
	PriceOut uint          `json:"price_out"`
	OwnSta   uint8         `json:"own_sta"`
	OwnNum   uint          `json:"own_num"`
	DtIn     string        `json:"dt_in"`
	DtOut    string        `json:"dt_out"`
	PP       model.Players `json:"p_p"`
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
	model.PlayerWithPriceChange
	Owned []*model.OwnInfo `json:"owned"`
}

// UserFavPlayerReq 用户收藏球员请求
type UserFavPlayerReq struct {
	PlayerID uint `json:"player_id"`
}
