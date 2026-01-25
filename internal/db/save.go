package db

import (
	"context"
	"o2stock-crawler/internal/consts"
	"o2stock-crawler/internal/crawler"
	"o2stock-crawler/internal/db/models"
	"strconv"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type SaveSnapshotDb struct {
	db *DB
}

func NewSaveSnapshotDb(database *DB) *SaveSnapshotDb {
	return &SaveSnapshotDb{
		db: database,
	}
}

// SaveSnapshot saves current roster snapshot into players and p_p_history tables.
func (s *SaveSnapshotDb) SaveSnapshot(ctx context.Context, database *DB, rosterList []crawler.RosterItemModel, now time.Time) error {
	// Use transaction
	return database.Transaction(func(tx *gorm.DB) error {
		// 去重：按 playerId + version + cardType 作为唯一键
		unique := make(map[string]*crawler.RosterItemModel)
		for i := range rosterList {
			item := &rosterList[i]
			key := strconv.Itoa(int(item.PlayerID)) + "|" + item.VersionStr + "|" + item.CardTypeStr
			if _, ok := unique[key]; ok {
				continue
			}
			unique[key] = item
		}

		for _, item := range unique {
			if err := s.upsertPlayer(ctx, tx, item); err != nil {
				return err
			}
			if err := s.insertHistory(ctx, tx, item, now); err != nil {
				return err
			}
		}
		return nil
	})
}

func (s *SaveSnapshotDb) upsertPlayer(ctx context.Context, tx *gorm.DB, item *crawler.RosterItemModel) error {
	cardType, _ := strconv.Atoi(item.CardTypeStr)
	version, _ := strconv.Atoi(item.VersionStr)
	factor := s.gradeFactor(item.Grade)

	currentLowest := 0
	if item.Price.CurrentLowestPrice != "" {
		if v, err := strconv.Atoi(item.Price.CurrentLowestPrice); err == nil {
			currentLowest = v / factor
		}
	}

	priceStandard := item.Price.StandardPrice / factor
	priceSaleLower := item.Price.LowerPriceForSale / factor
	priceSaleUpper := item.Price.UpperPriceForSale / factor

	if priceStandard < consts.LowestPrice {
		return nil
	}

	player := models.Player{
		PlayerID:           item.PlayerID,
		ShowName:           item.ShowName,
		EnName:             item.PlayerEn,
		TeamAbbr:           item.TeamAbbr,
		Version:            uint(version),
		CardType:           uint(cardType),
		PlayerImg:          item.PlayerImg,
		PriceStandard:      uint(priceStandard),
		PriceCurrentLowest: uint(currentLowest),
		PriceSaleLower:     uint(priceSaleLower),
		PriceSaleUpper:     uint(priceSaleUpper),
		OverAll:            uint(item.OverAll),
	}

	// Use GORM Clauses for ON DUPLICATE KEY UPDATE
	return tx.WithContext(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "player_id"}},
		DoUpdates: clause.AssignmentColumns([]string{
			"p_name_show", "p_name_en", "team_abbr", "version", "card_type",
			"player_img", "price_standard", "price_current_lowest",
			"price_sale_lower", "price_sale_upper", "over_all",
		}),
	}).Create(&player).Error
}

func (s *SaveSnapshotDb) insertHistory(ctx context.Context, tx *gorm.DB, item *crawler.RosterItemModel, now time.Time) error {
	factor := s.gradeFactor(item.Grade)
	priceStandard := item.Price.StandardPrice / factor

	if priceStandard < consts.LowestPrice {
		return nil
	}

	priceLower := item.Price.LowerPriceForSale / factor
	priceUpper := item.Price.UpperPriceForSale / factor

	currentLowest := -1
	if item.Price.CurrentLowestPrice != "" && item.Price.CurrentLowestPrice != "- -" {
		if v, err := strconv.Atoi(item.Price.CurrentLowestPrice); err == nil {
			currentLowest = v / factor
		}
	}

	history := models.PlayerPriceHistory{
		PlayerID:         item.PlayerID,
		AtDate:           now,
		AtDateHour:       now.Format("200601021504"),
		AtYear:           now.Format("2006"),
		AtMonth:          now.Format("01"),
		AtDay:            now.Format("02"),
		AtHour:           now.Format("15"),
		AtMinute:         now.Format("04"),
		PriceStandard:    uint(priceStandard),
		PriceCurrentSale: currentLowest,
		PriceLower:       uint(priceLower),
		PriceUpper:       uint(priceUpper),
		CTime:            now,
	}

	return tx.WithContext(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "at_date_hour"}, {Name: "player_id"}},
		DoUpdates: clause.AssignmentColumns([]string{
			"price_standard", "price_lower", "price_upper", "price_current_sale",
		}),
	}).Create(&history).Error
}

// gradeFactor 将 grade 转换为基础卡张数：n 级需要 2^(n-1) 张卡
func (s *SaveSnapshotDb) gradeFactor(grade uint8) int {
	return 1 << (grade - 1)
}
