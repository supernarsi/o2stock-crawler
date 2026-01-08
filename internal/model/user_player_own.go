package model

import "time"

// UserPlayerOwn 表示 u_p_own 表的一行
type UserPlayerOwn struct {
	ID       uint       `json:"id"`
	UserID   uint       `json:"user_id"`
	PlayerID uint       `json:"player_id"`
	OwnSta   uint8      `json:"own_sta"` // 0.未拥有；1.已购买；2.已出售
	PriceIn  uint       `json:"price_in"`
	PriceOut uint       `json:"price_out"`
	NumIn    uint       `json:"own_num"` // 购买的卡数
	DtIn     time.Time  `json:"dt_in"`
	DtOut    *time.Time `json:"dt_out"`
}

// OwnInfo 用于 API 响应，简化版本（不包含 id）
type OwnInfo struct {
	PlayerID uint   `json:"player_id"`
	PriceIn  uint   `json:"price_in"`
	PriceOut uint   `json:"price_out"`
	OwnSta   uint8  `json:"own_sta"`
	OwnNum   uint   `json:"own_num"`
	DtIn     string `json:"dt_in"` // 格式: 2006-01-02 15:04:05
	DtOut    string `json:"dt_out"`
}

// ToOwnInfo 将 UserPlayerOwn 转换为 OwnInfo
func (u *UserPlayerOwn) ToOwnInfo() OwnInfo {
	info := OwnInfo{
		PlayerID: u.PlayerID,
		PriceIn:  u.PriceIn,
		PriceOut: u.PriceOut,
		OwnSta:   u.OwnSta,
		OwnNum:   u.NumIn,
		DtIn:     u.DtIn.Format("2006-01-02 15:04:05"),
	}
	if u.DtOut != nil {
		info.DtOut = u.DtOut.Format("2006-01-02 15:04:05")
	}
	return info
}
