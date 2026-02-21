package repositories

import (
	"context"
	"fmt"
	"o2stock-crawler/internal/consts"
	"o2stock-crawler/internal/entity"

	"gorm.io/gorm"
)

type PlayerRepository struct {
	baseRepository[entity.Player]
}

func NewPlayerRepository(db *gorm.DB) *PlayerRepository {
	return &PlayerRepository{
		baseRepository: baseRepository[entity.Player]{db: db},
	}
}

type PlayerFilter struct {
	Page          int
	Limit         int
	OrderBy       string
	OrderAsc      bool
	Period        uint8
	SoldOut       bool
	PlayerName    string
	PlayerIDs     []uint
	MinPrice      uint
	MaxPrice      uint
	ExFree        bool
	TeamAbbr      string
	OnlyFreeAgent bool
}

func (r *PlayerRepository) List(ctx context.Context, filter PlayerFilter) ([]entity.Player, error) {
	var players []entity.Player
	query := r.model(ctx)

	// Apply filters
	if filter.SoldOut {
		query = query.Where("price_current_lowest = 0")
	}
	if filter.PlayerName != "" {
		query = query.Where("p_name_show LIKE ?", "%"+filter.PlayerName+"%")
	}
	if filter.MinPrice > 0 {
		query = query.Where("price_sale_lower >= ?", filter.MinPrice)
	}
	if filter.MaxPrice > 0 {
		query = query.Where("price_sale_upper <= ?", filter.MaxPrice)
	}
	if filter.ExFree {
		query = query.Where("team_abbr != ?", "自由球员")
	}
	if filter.OnlyFreeAgent {
		query = query.Where("team_abbr = ?", "自由球员")
	}
	if filter.TeamAbbr != "" {
		query = query.Where("team_abbr = ?", filter.TeamAbbr)
	}
	if len(filter.PlayerIDs) > 0 {
		query = query.Where("player_id IN ?", filter.PlayerIDs)
	}

	// Default filter for list
	if consts.LowestPrice > 0 {
		query = query.Where("price_standard >= ?", consts.LowestPrice)
	}

	// Sort
	if filter.OrderBy != "" {
		direction := "DESC"
		if filter.OrderAsc {
			direction = "ASC"
		}
		query = query.Order(fmt.Sprintf("%s %s", filter.OrderBy, direction))
	}

	// Pagination
	if filter.Limit > 0 {
		offset := max((filter.Page-1)*filter.Limit, 0)
		query = query.Offset(offset).Limit(filter.Limit)
	}

	err := query.Find(&players).Error
	return players, err
}

func (r *PlayerRepository) GetByID(ctx context.Context, playerID uint) (*entity.Player, error) {
	var player entity.Player
	err := r.ctx(ctx).Where("player_id = ?", playerID).First(&player).Error
	if err != nil {
		return nil, err
	}
	return &player, nil
}

func (r *PlayerRepository) BatchGetByIDs(ctx context.Context, playerIDs []uint) ([]entity.Player, error) {
	var players []entity.Player
	err := r.ctx(ctx).Where("player_id IN ?", playerIDs).Find(&players).Error
	return players, err
}

func (r *PlayerRepository) GetExtraByPlayerIDs(ctx context.Context, playerIDs []uint) ([]entity.PlayerExtra, error) {
	var extras []entity.PlayerExtra
	err := r.ctx(ctx).Model(&entity.PlayerExtra{}).Where("player_id IN ?", playerIDs).Find(&extras).Error
	return extras, err
}

// GetPlayersForAgeSync 获取待补充年龄的球员：若 playerIDs 为空则返回所有 tx_player_id > 0；否则按 player_id 过滤
func (r *PlayerRepository) GetPlayersForAgeSync(ctx context.Context, playerIDs []uint) ([]entity.Player, error) {
	query := r.ctx(ctx).Where("tx_player_id > 0")
	if len(playerIDs) > 0 {
		query = query.Where("tx_player_id IN ?", playerIDs)
	}
	var players []entity.Player
	err := query.Find(&players).Error
	return players, err
}

// UpdateAge 更新球员年龄
func (r *PlayerRepository) UpdateAge(ctx context.Context, playerID uint, age uint) error {
	return r.model(ctx).
		Where("player_id = ?", playerID).
		Update("age", age).Error
}

func (r *PlayerRepository) UpdatePower(ctx context.Context, playerID uint, power5, power10 float64) error {
	return r.model(ctx).
		Where("player_id = ?", playerID).
		Updates(map[string]any{
			"power_per5":  power5,
			"power_per10": power10,
		}).Error
}

func (r *PlayerRepository) UpdatePriceChanges(ctx context.Context, playerID uint, pc1d, pc7d float64) error {
	return r.model(ctx).
		Where("player_id = ?", playerID).
		Updates(map[string]any{
			"price_change_1d": pc1d,
			"price_change_7d": pc7d,
		}).Error
}

// GetAllTxPlayers 返回参与 IPI 计算的球员：tx_player_id > 0 且非自由球员（用于排名、批量 IPI 等）
func (r *PlayerRepository) GetAllTxPlayers(ctx context.Context) ([]entity.Player, error) {
	var players []entity.Player
	err := r.ctx(ctx).Where(ipiEligibleCondition, "自由球员").Find(&players).Error
	return players, err
}

func (r *PlayerRepository) Transaction(fn func(tx *gorm.DB) error) error {
	return r.db.Transaction(fn)
}

// ipiEligibleCondition 参与 IPI 计算的球员条件：排除自由球员与 tx_player_id=0
const ipiEligibleCondition = "tx_player_id > 0 AND team_abbr != ?"

// AvgPriceByOVRSegment 计算同 OVR 段（over_all 在 [ovr-radius, ovr+radius]）球员的 price_standard 均值
// 仅统计参与 IPI 计算的球员（排除自由球员、tx_player_id=0）。用于 IPI 价值洼地分 V_gap。
func (r *PlayerRepository) AvgPriceByOVRSegment(ctx context.Context, ovr uint, radius int) (avgPrice float64, count int64, err error) {
	if radius < 0 {
		radius = 0
	}
	low := int(ovr) - radius
	high := int(ovr) + radius
	if low < 0 {
		low = 0
	}
	var res struct {
		Avg   float64
		Count int64
	}
	err = r.ctx(ctx).Model(&entity.Player{}).
		Select("AVG(price_standard) AS avg, COUNT(*) AS count").
		Where("over_all >= ? AND over_all <= ? AND price_standard > 0", low, high).
		Where(ipiEligibleCondition, "自由球员").
		Scan(&res).Error
	if err != nil {
		return 0, 0, err
	}
	return res.Avg, res.Count, nil
}

// AvgPriceGlobal 全表 price_standard 均值，仅统计参与 IPI 计算的球员（排除自由球员、tx_player_id=0），用于 V_gap 回退
func (r *PlayerRepository) AvgPriceGlobal(ctx context.Context) (float64, error) {
	var avg float64
	err := r.ctx(ctx).Model(&entity.Player{}).
		Select("AVG(price_standard)").
		Where("price_standard > 0").
		Where(ipiEligibleCondition, "自由球员").
		Scan(&avg).Error
	return avg, err
}

// OVRAvgPriceMap 批量获取所有 OVR 段的均价 map，key 为 over_all 值，用于 IPI 批量计算预缓存
// 仅统计参与 IPI 计算的球员（排除自由球员、tx_player_id=0）
func (r *PlayerRepository) OVRAvgPriceMap(ctx context.Context) (map[uint]float64, error) {
	type ovrAvg struct {
		OverAll uint
		Avg     float64
	}
	var results []ovrAvg
	err := r.ctx(ctx).Model(&entity.Player{}).
		Select("over_all, AVG(price_standard) as avg").
		Where("price_standard > 0").
		Where(ipiEligibleCondition, "自由球员").
		Group("over_all").
		Scan(&results).Error
	if err != nil {
		return nil, err
	}
	out := make(map[uint]float64, len(results))
	for _, r := range results {
		out[r.OverAll] = r.Avg
	}
	return out, nil
}

// OVRCountMap 批量获取所有 OVR 段的球员数量 map，key 为 over_all 值
func (r *PlayerRepository) OVRCountMap(ctx context.Context) (map[uint]int64, error) {
	type ovrCount struct {
		OverAll uint
		Count   int64
	}
	var results []ovrCount
	err := r.ctx(ctx).Model(&entity.Player{}).
		Select("over_all, COUNT(*) as count").
		Where("price_standard > 0").
		Where(ipiEligibleCondition, "自由球员").
		Group("over_all").
		Scan(&results).Error
	if err != nil {
		return nil, err
	}
	out := make(map[uint]int64, len(results))
	for _, r := range results {
		out[r.OverAll] = r.Count
	}
	return out, nil
}
