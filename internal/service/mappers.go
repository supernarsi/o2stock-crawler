package service

import (
	"o2stock-crawler/internal/consts"
	"o2stock-crawler/internal/dto"
	"o2stock-crawler/internal/entity"
)

// ToUserDTO converts a User entity to a User DTO
func ToUserDTO(u *entity.User) *dto.User {
	if u == nil {
		return nil
	}
	return &dto.User{
		ID:           u.ID,
		Nick:         u.Nick,
		Avatar:       u.Avatar,
		WxOpenID:     u.WxOpenID,
		WxUnionID:    u.WxUnionID,
		WxSessionKey: u.WxSessionKey,
		Sta:          uint8(u.Sta),
		CTime:        u.CTime,
	}
}

// ToPlayerDTO converts a Player entity to a Players DTO
func ToPlayerDTO(p entity.Player) dto.Players {
	return dto.Players{
		PlayerID:          p.PlayerID,
		TxPlayerID:        p.TxPlayerID,
		ShowName:          p.ShowName,
		EnName:            p.EnName,
		TeamAbbr:          p.TeamAbbr,
		Age:               p.Age,
		Version:           p.Version,
		CardType:          p.CardType,
		PlayerImg:         p.PlayerImg,
		PriceStandard:     p.PriceStandard,
		PriceCurrentLower: p.PriceCurrentLowest,
		PriceSaleLower:    p.PriceSaleLower,
		PriceSaleUpper:    p.PriceSaleUpper,
		OverAll:           p.OverAll,
		PowerPer5:         p.PowerPer5,
		PowerPer10:        p.PowerPer10,
		PriceChange1d:     p.PriceChange1d,
		PriceChange7d:     p.PriceChange7d,
		UpdatedAt:         p.UpdatedAt.Format("2006-01-02 15:04:05"),
	}
}

// ToPlayerWithPriceChangeDTO converts a Player entity to a PlayerWithPriceChange DTO
func ToPlayerWithPriceChangeDTO(p entity.Player) dto.PlayerWithPriceChange {
	return dto.PlayerWithPriceChange{
		Players:     ToPlayerDTO(p),
		PriceChange: p.PriceChange1d,
	}
}

// ToOwnInfoDTO converts a UserPOwn entity to an OwnInfo DTO
func ToOwnInfoDTO(o entity.UserPOwn) dto.OwnInfo {
	dtOut := ""
	if o.SellTime != nil {
		dtOut = o.SellTime.Format("2006-01-02 15:04:05")
	}

	notifyType := o.NotifyType
	// 仅对“持有/已购买(own_sta=1)”的记录返回实际订阅类型；其他状态返回 0
	if uint8(o.Sta) != consts.OwnStaPurchased {
		notifyType = consts.NotifyTypeNone
	}

	return dto.OwnInfo{
		OwnID:      o.ID,
		OwnGoods:   o.OwnGoods,
		GoodsID:    o.PID,
		PriceIn:    o.BuyPrice,
		PriceOut:   o.SellPrice,
		OwnSta:     uint8(o.Sta),
		OwnNum:     o.BuyCount,
		DtIn:       o.BuyTime.Format("2006-01-02 15:04:05"),
		DtOut:      dtOut,
		NotifyType: notifyType,
	}
}

// ToOwnInfoDTOList converts a list of UserPOwn entities to a list of OwnInfo DTOs
func ToOwnInfoDTOList(records []entity.UserPOwn) []dto.OwnInfo {
	if records == nil {
		return nil
	}
	result := make([]dto.OwnInfo, len(records))
	for i, o := range records {
		result[i] = ToOwnInfoDTO(o)
	}
	return result
}

// ToOwnInfoDTOMap converts a list of UserPOwn entities to a map of OwnInfo DTOs grouped by GoodsID (PID)
func ToOwnInfoDTOMap(records []entity.UserPOwn) map[uint][]dto.OwnInfo {
	result := make(map[uint][]dto.OwnInfo)
	for _, o := range records {
		result[o.PID] = append(result[o.PID], ToOwnInfoDTO(o))
	}
	return result
}

// ToOwnInfoDTOPointerMap converts a list of UserPOwn entities to a map of []*OwnInfo DTOs grouped by GoodsID (PID)
func ToOwnInfoDTOPointerMap(records []entity.UserPOwn) map[uint][]*dto.OwnInfo {
	result := make(map[uint][]*dto.OwnInfo)
	for _, o := range records {
		info := ToOwnInfoDTO(o)
		result[o.PID] = append(result[o.PID], &info)
	}
	return result
}
