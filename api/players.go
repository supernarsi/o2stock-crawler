package api

import (
	"o2stock-crawler/internal/dto"
)

// PlayerHistoryRes 球员历史价格响应
type PlayerHistoryRes struct {
	PlayerInfo    *PlayerWithOwned       `json:"player_info"`
	PlayerHistory []*dto.PriceHistoryRow `json:"history"`
	GameData      *GameData              `json:"game_data"`
}

// MultiPlayersHistoryRes 批量球员历史价格响应
type MultiPlayersHistoryRes struct {
	History []PlayerHistoryItem `json:"history"`
}

// PlayerInvestmentStatsRes 球员投资盈亏统计列表响应
type PlayerInvestmentStatsRes struct {
	List []dto.PlayerInvestmentStats `json:"list"`
}

// PlayerHistoryItem 单个球员的历史价格项
type PlayerHistoryItem struct {
	PlayerID uint32                 `json:"player_id"`
	History  []*dto.PriceHistoryRow `json:"history"`
}

type PlayerGameData struct {
	PlayerID uint32                 `json:"player_id"`
	GameData []*dto.PriceHistoryRow `json:"game_data"`
}

type GameData struct {
	PlayerID       uint32              `json:"player_id"`
	PlayerNameShow string              `json:"p_name_show"`
	Standard       *GameDataStandard   `json:"standard"`
	NbaToday       []*GameDataNbaToday `json:"nba_today"`
}

type GameDataNbaToday struct {
	Date      string `json:"date" dc:"比赛日期"`
	VsHome    string `json:"vs_home" dc:"对阵主队"`
	VsAway    string `json:"vs_away" dc:"对阵客队"`
	IsHome    bool   `json:"is_home" dc:"是否主场"`
	Points    uint   `json:"points" dc:"得分"`
	Rebound   uint   `json:"rb" dc:"篮板"`
	Assists   uint   `json:"ast" dc:"助攻"`
	Blocks    uint   `json:"blk" dc:"盖帽"`
	Steals    uint   `json:"stl" dc:"抢断"`
	Turnovers uint   `json:"tov" dc:"失误"`
	Minutes   uint   `json:"min" dc:"上场时间（分钟）"`
}

type GameDataStandard struct {
	Time                 float64 `json:"time" dc:"出场时间"`
	Points               float64 `json:"points" dc:"得分"`
	Rebound              float64 `json:"rb" dc:"篮板"`
	ReboundOffense       float64 `json:"rb_o" dc:"进攻篮板"`
	ReboundDefense       float64 `json:"rb_d" dc:"防守篮板"`
	Assists              float64 `json:"ast" dc:"助攻"`
	Blocks               float64 `json:"blk" dc:"盖帽"`
	Steals               float64 `json:"stl" dc:"抢断"`
	Turnovers            float64 `json:"tov" dc:"失误"`
	Fouls                float64 `json:"pf" dc:"犯规"`
	PercentOfThrees      float64 `json:"pct_3" dc:"三分命中率"`
	PercentOfTwoPointers float64 `json:"pct_2" dc:"两分命中率"`
	PercentOfFreeThrows  float64 `json:"pct_ft" dc:"罚球命中率"`
}
