package service

import (
	"context"
	"o2stock-crawler/internal/consts"
	"o2stock-crawler/internal/crawler"
	"o2stock-crawler/internal/db"
	"o2stock-crawler/internal/entity"
	"strconv"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// SnapshotService 处理爬虫快照数据的保存
type SnapshotService struct {
	db *db.DB
}

// NewSnapshotService 创建 SnapshotService 实例
func NewSnapshotService(database *db.DB) *SnapshotService {
	return &SnapshotService{
		db: database,
	}
}

// SaveSnapshot 将当前的球员快照保存到 players 和 p_p_history 表中
func (s *SnapshotService) SaveSnapshot(ctx context.Context, rosterList []crawler.RosterItemModel, now time.Time) error {
	// 使用事务
	return s.db.Transaction(func(tx *gorm.DB) error {
		// 1. 去重：按 playerID + versionStr + cardTypeStr 作为唯一键
		unique := make(map[uint]*crawler.RosterItemModel)
		playerIDs := make([]uint, 0, len(rosterList))
		for i := range rosterList {
			item := &rosterList[i]
			// 假设 player_id 是全局唯一的复合主键标识 (根据业务逻辑)
			if _, ok := unique[item.PlayerID]; !ok {
				unique[item.PlayerID] = item
				playerIDs = append(playerIDs, item.PlayerID)
			}
		}

		// 2. 批量查询已存在的球员，避免 INSERT ... ON DUPLICATE KEY UPDATE 导致 ID 跳号
		var existingPlayers []entity.Player
		if err := tx.WithContext(ctx).Where("player_id IN ?", playerIDs).Find(&existingPlayers).Error; err != nil {
			return err
		}

		existingMap := make(map[uint]entity.Player)
		for _, p := range existingPlayers {
			existingMap[p.PlayerID] = p
		}

		// 3. 循环处理
		for pid, item := range unique {
			existing, exists := existingMap[pid]
			if err := s.upsertPlayer(ctx, tx, item, now, exists, &existing); err != nil {
				return err
			}
			if err := s.insertHistory(ctx, tx, item, now); err != nil {
				return err
			}
		}
		return nil
	})
}

// upsertPlayer 优化后的更新逻辑，显式处理 Update 与 Insert
func (s *SnapshotService) upsertPlayer(ctx context.Context, tx *gorm.DB, item *crawler.RosterItemModel, now time.Time, exists bool, existing *entity.Player) error {
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

	if exists {
		// 执行更新，通过主键 ID 锁定记录，不会触发 auto_increment 自增
		return tx.WithContext(ctx).Model(existing).Updates(map[string]any{
			"p_name_show":          item.ShowName,
			"p_name_en":            item.PlayerEn,
			"team_abbr":            item.TeamAbbr,
			"version":              uint(version),
			"card_type":            uint(cardType),
			"player_img":           item.PlayerImg,
			"price_standard":       uint(priceStandard),
			"price_current_lowest": uint(currentLowest),
			"price_sale_lower":     uint(priceSaleLower),
			"price_sale_upper":     uint(priceSaleUpper),
			"over_all":             uint(item.OverAll),
			"update_at":            now,
		}).Error
	}

	// 执行插入
	player := entity.Player{
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
		UpdatedAt:          now,
	}
	return tx.WithContext(ctx).Create(&player).Error
}

func (s *SnapshotService) insertHistory(ctx context.Context, tx *gorm.DB, item *crawler.RosterItemModel, now time.Time) error {
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

	history := entity.PlayerPriceHistory{
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

	// 历史表允许按小时覆盖，ID 跳号在流水表中通常不是主要矛盾，但为了严谨这里也使用 ON CONFLICT
	return tx.WithContext(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "at_date_hour"}, {Name: "player_id"}},
		DoUpdates: clause.AssignmentColumns([]string{
			"price_standard", "price_lower", "price_upper", "price_current_sale",
		}),
	}).Create(&history).Error
}

// gradeFactor 将 grade 转换为基础卡张数
func (s *SnapshotService) gradeFactor(grade uint8) int {
	return 1 << (grade - 1)
}
