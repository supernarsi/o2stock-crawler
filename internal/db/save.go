package db

import (
	"context"
	"database/sql"
	"o2stock-crawler/internal/consts"
	"o2stock-crawler/internal/crawler"
	"strconv"
	"time"
)

type SaveSnapshotDb struct {
	DbBase
}

func NewSaveSnapshotDb() *SaveSnapshotDb {
	return &SaveSnapshotDb{
		DbBase: DbBase{},
	}
}

// SaveSnapshot saves current roster snapshot into players and p_p_history tables.
func (s *SaveSnapshotDb) SaveSnapshot(ctx context.Context, database *DB, rosterList []crawler.RosterItemModel, now time.Time) error {
	tx, err := database.BeginTx(ctx, &sql.TxOptions{})
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	// 去重：按 playerId + version + cardType 作为唯一键，避免重复入库同一球员
	unique := make(map[string]*crawler.RosterItemModel)
	for i := range rosterList {
		item := &rosterList[i]
		key := strconv.Itoa(int(item.PlayerID)) + "|" + item.VersionStr + "|" + item.CardTypeStr
		if _, ok := unique[key]; ok {
			continue
		}
		unique[key] = item
	}

	// todo: 批量查询球员的 24 小时前价格

	for _, item := range unique {
		if err := s.upsertPlayer(ctx, tx, item); err != nil {
			return err
		}
		if err := s.insertHistory(ctx, tx, item, now); err != nil {
			return err
		}
	}

	if err := tx.Commit(); err != nil {
		return err
	}
	return nil
}

func (s *SaveSnapshotDb) upsertPlayer(ctx context.Context, tx *sql.Tx, item *crawler.RosterItemModel) error {
	playerID := item.PlayerID
	cardType, _ := strconv.Atoi(item.CardTypeStr)
	version, _ := strconv.Atoi(item.VersionStr)

	// 根据 grade 计算单张基础卡的价格：grade n 表示 2^(n-1) 张基础卡
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
		// 如果单张基础卡价格低于最低价格，则不保存
		return nil
	}

	// 保存球员数据
	const q = `
INSERT INTO players
	(player_id, p_name_show, p_name_en, team_abbr, version, card_type,
	 player_img, price_standard, price_current_lowest, price_sale_lower, price_sale_upper, over_all)
VALUES (?,?,?,?,?,?,?,?,?,?,?,?)
ON DUPLICATE KEY UPDATE
	p_name_show = VALUES(p_name_show),
	p_name_en = VALUES(p_name_en),
	team_abbr = VALUES(team_abbr),
	version = VALUES(version),
	card_type = VALUES(card_type),
	player_img = VALUES(player_img),
	price_standard = VALUES(price_standard),
	price_current_lowest = VALUES(price_current_lowest),
	price_sale_lower = VALUES(price_sale_lower),
	price_sale_upper = VALUES(price_sale_upper),
	over_all = VALUES(over_all)
`

	_, err := tx.ExecContext(ctx, q,
		playerID,
		item.ShowName,
		item.PlayerEn,
		item.TeamAbbr,
		version,
		cardType,
		item.PlayerImg,
		priceStandard,
		currentLowest,
		priceSaleLower,
		priceSaleUpper,
		item.OverAll,
	)
	if err != nil {
		return err
	}
	return nil
}

func (s *SaveSnapshotDb) insertHistory(ctx context.Context, tx *sql.Tx, item *crawler.RosterItemModel, now time.Time) error {
	playerID := item.PlayerID

	atDate := now.Format("2006-01-02")
	atYear := now.Format("2006")
	atMonth := now.Format("01")
	atDay := now.Format("02")
	atHour := now.Format("15")
	atMinute := now.Format("04")
	atDateHour := now.Format("200601021504")

	// 历史表也保存单张基础卡的价格
	factor := s.gradeFactor(item.Grade)
	priceStandard := item.Price.StandardPrice / factor

	if priceStandard < consts.LowestPrice {
		// 如果单张基础卡价格低于最低价格，则不保存
		return nil
	}

	// 计算市场最低价和最高价（单张基础卡价格），来源为 lowerPriceForSale / upperPriceForSale
	priceLower := item.Price.LowerPriceForSale / factor
	priceUpper := item.Price.UpperPriceForSale / factor

	// 计算当前成交的最低价（单张基础卡价格）
	currentLowest := -1 // 默认-1，表示没有成交（断卡）
	if item.Price.CurrentLowestPrice != "" && item.Price.CurrentLowestPrice != "- -" {
		if v, err := strconv.Atoi(item.Price.CurrentLowestPrice); err == nil {
			currentLowest = v / factor
		}
	}

	// 由于存在唯一键，如果重复数据直接替换，ON DUPLICATE KEY UPDATE。
	const q = `
INSERT INTO p_p_history
	(player_id, at_date, at_date_hour, at_year, at_month, at_day, at_hour, at_minute, price_standard, price_current_sale, price_lower, price_upper, c_time)
VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?)
ON DUPLICATE KEY UPDATE
	price_standard = VALUES(price_standard),
	price_lower = VALUES(price_lower),
	price_upper = VALUES(price_upper),
	price_current_sale = VALUES(price_current_sale)
`
	_, err := tx.ExecContext(ctx, q,
		playerID,
		atDate,
		atDateHour,
		atYear,
		atMonth,
		atDay,
		atHour,
		atMinute,
		priceStandard,
		currentLowest,
		priceLower,
		priceUpper,
		now,
	)
	return err
}

// gradeFactor 将 grade 转换为基础卡张数：n 级需要 2^(n-1) 张卡
func (s *SaveSnapshotDb) gradeFactor(grade uint8) int {
	return 1 << (grade - 1)
}
