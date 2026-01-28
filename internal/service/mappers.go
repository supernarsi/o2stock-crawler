package service

import (
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
