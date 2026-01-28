// Package entity contains GORM entity models representing database tables.
// These are the domain entities used for data persistence.
package entity

import "time"

// Player 球员实体模型
type Player struct {
	ID                 uint      `gorm:"primaryKey;column:id"`
	PlayerID           uint      `gorm:"column:player_id;uniqueIndex"`
	NBAPlayerID        uint      `gorm:"column:nba_player_id"`
	ShowName           string    `gorm:"column:p_name_show"`
	EnName             string    `gorm:"column:p_name_en"`
	TxPlayerID         uint      `gorm:"column:tx_player_id"`
	TeamAbbr           string    `gorm:"column:team_abbr"`
	Version            uint      `gorm:"column:version"`
	CardType           uint      `gorm:"column:card_type"`
	PlayerImg          string    `gorm:"column:player_img"`
	PriceStandard      uint      `gorm:"column:price_standard"`
	PriceCurrentLowest uint      `gorm:"column:price_current_lowest"`
	PriceSaleLower     uint      `gorm:"column:price_sale_lower"`
	PriceSaleUpper     uint      `gorm:"column:price_sale_upper"`
	OverAll            uint      `gorm:"column:over_all"`
	PowerPer5          float64   `gorm:"column:power_per5"`
	PowerPer10         float64   `gorm:"column:power_per10"`
	PriceChange1d      float64   `gorm:"column:price_change_1d"`
	PriceChange7d      float64   `gorm:"column:price_change_7d"`
	UpdatedAt          time.Time `gorm:"column:update_at;autoUpdateTime"`
}

// TableName returns the table name for GORM.
func (Player) TableName() string {
	return "players"
}
