package dto

import (
	"o2stock-crawler/internal/consts"
	"time"
)

// User 用户API响应DTO
type User struct {
	ID           uint      `json:"id"`
	Nick         string    `json:"nick"`
	Avatar       string    `json:"avatar"`
	WxOpenID     string    `json:"wx_openid"`
	WxUnionID    string    `json:"wx_unionid"`
	WxSessionKey string    `json:"wx_session_key"`
	Sta          uint8     `json:"sta"`
	CTime        time.Time `json:"c_time"`
}

// UserPlayerOwn 用户球员持仓DTO
type UserPlayerOwn struct {
	ID         uint       `json:"id"`
	UserID     uint       `json:"uid"`
	OwnGoods   uint8      `json:"own_goods"`
	PlayerID   uint       `json:"pid"`
	OwnSta     uint8      `json:"own_sta"`
	PriceIn    uint       `json:"price_in"`
	PriceOut   uint       `json:"price_out"`
	NumIn      uint       `json:"own_num"`
	DtIn       time.Time  `json:"dt_in"`
	DtOut      *time.Time `json:"dt_out"`
	NotifyType uint8      `json:"notify_type"`
}

// OwnInfo 简化版持仓信息DTO
type OwnInfo struct {
	OwnID      uint   `json:"own_id"`    // 持仓记录 id，用于出售等操作
	OwnGoods   uint8  `json:"own_goods"` // 1:球员; 2:道具
	GoodsID    uint   `json:"goods_id"`  // 球员id 或 道具id
	PriceIn    uint   `json:"price_in"`
	PriceOut   uint   `json:"price_out"`
	OwnSta     uint8  `json:"own_sta"`
	OwnNum     uint   `json:"own_num"`
	DtIn       string `json:"dt_in"`
	DtOut      string `json:"dt_out"`
	NotifyType uint8  `json:"notify_type"` // 0:不订阅 1:回本 2:盈利15%，own_sta=0 时返回 0
}

// ToOwnInfo 转换为OwnInfo
func (u *UserPlayerOwn) ToOwnInfo() OwnInfo {
	info := OwnInfo{
		OwnID:      u.ID,
		GoodsID:    u.PlayerID,
		OwnGoods:   u.OwnGoods,
		PriceIn:    u.PriceIn,
		PriceOut:   u.PriceOut,
		OwnSta:     u.OwnSta,
		OwnNum:     u.NumIn,
		DtIn:       u.DtIn.Format("2006-01-02 15:04:05"),
		NotifyType: u.NotifyType,
	}
	if u.DtOut != nil {
		info.DtOut = u.DtOut.Format("2006-01-02 15:04:05")
	}
	// 仅对“持有/已购买(own_sta=1)”的记录返回实际订阅类型；其他状态返回 0
	if info.OwnSta != consts.OwnStaPurchased {
		info.NotifyType = consts.NotifyTypeNone
	}
	return info
}

// UnifiedOwnGoods 统一持仓 DTO
type UnifiedOwnGoods struct {
	OwnID              uint   `json:"own_id"`
	OwnGoods           uint8  `json:"own_goods"` // 1:球员; 2:道具
	GoodsID            uint   `json:"goods_id"`
	PriceIn            uint   `json:"price_in"`
	PriceOut           uint   `json:"price_out"`
	OwnSta             uint8  `json:"own_sta"`
	OwnNum             uint   `json:"own_num"`
	DtIn               string `json:"dt_in"`
	DtOut              string `json:"dt_out"`
	GoodsName          string `json:"goods_name"`
	GoodsNameEn        string `json:"goods_name_en"`
	GoodsImg           string `json:"goods_img"`
	PriceStandard      uint   `json:"price_standard"`
	PriceCurrentLowest uint   `json:"price_current_lowest"`
}
