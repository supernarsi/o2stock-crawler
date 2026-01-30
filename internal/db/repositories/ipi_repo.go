package repositories

import (
	"context"
	"time"

	"o2stock-crawler/internal/entity"
	"o2stock-crawler/internal/model"

	"gorm.io/gorm"
)

type IPIRepository struct {
	baseRepository[entity.PlayerIPI]
}

func NewIPIRepository(db *gorm.DB) *IPIRepository {
	return &IPIRepository{baseRepository: baseRepository[entity.PlayerIPI]{db: db}}
}

// SaveBatch 将一批 IPI 结果写入 player_ipi，calculated_at 统一为给定时间
func (r *IPIRepository) SaveBatch(ctx context.Context, results []model.IPIResult, calculatedAt time.Time) error {
	if len(results) == 0 {
		return nil
	}
	rows := make([]entity.PlayerIPI, len(results))
	for i := range results {
		rows[i] = entity.PlayerIPI{
			PlayerID:           results[i].PlayerID,
			IPI:                results[i].IPI,
			SPerf:              results[i].SPerf,
			VGap:               results[i].VGap,
			MGrowth:            results[i].MGrowth,
			RRisk:              results[i].RRisk,
			MeetsTaxSafeMargin: results[i].MeetsTaxSafeMargin,
			RankInversionIndex: results[i].RankInversionIndex,
			CalculatedAt:       calculatedAt,
		}
	}
	return r.ctx(ctx).CreateInBatches(rows, 200).Error
}

// GetLatestCalculatedAt 取最新一批的计算时间；无数据返回零值与 false
func (r *IPIRepository) GetLatestCalculatedAt(ctx context.Context) (time.Time, bool, error) {
	var t time.Time
	err := r.model(ctx).Select("MAX(calculated_at)").Scan(&t).Error
	if err != nil {
		return time.Time{}, false, err
	}
	if t.IsZero() {
		return time.Time{}, false, nil
	}
	return t, true, nil
}

// ListLatestFilter 最新一批列表查询条件
type ListLatestFilter struct {
	Page         int
	Limit        int
	TaxSafeOnly  bool
	MinPrice     uint      // 最低价格（price_sale_lower），0 表示不限制
	MaxPrice     uint      // 最高价格（price_sale_upper），0 表示不限制
	CalculatedAt time.Time // 必须为已存在的批次时间
}

// ListLatest 取指定批次（通常为最新一批）的 IPI 记录，按 ipi 降序分页；可选税后安全边际、价格区间
// 价格区间通过子查询 players 表按 price_sale_lower、 price_sale_upper筛选，避免 JOIN 导致的列名问题
func (r *IPIRepository) ListLatest(ctx context.Context, filter ListLatestFilter) (list []entity.PlayerIPI, total int64, err error) {
	query := r.model(ctx).Where("calculated_at = ?", filter.CalculatedAt)
	if filter.TaxSafeOnly {
		query = query.Where("meets_tax_safe_margin = ?", true)
	}
	if filter.MinPrice > 0 || filter.MaxPrice > 0 {
		subQuery := r.ctx(ctx).Table("players").Select("player_id")
		if filter.MinPrice > 0 {
			subQuery = subQuery.Where("price_sale_lower >= ?", filter.MinPrice)
		}
		if filter.MaxPrice > 0 {
			subQuery = subQuery.Where("price_sale_upper <= ?", filter.MaxPrice)
		}
		query = query.Where("player_id IN (?)", subQuery)
	}
	if err = query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	list = make([]entity.PlayerIPI, 0)
	offset := (filter.Page - 1) * filter.Limit
	if offset < 0 {
		offset = 0
	}
	err = query.Order("ipi DESC").Offset(offset).Limit(filter.Limit).Find(&list).Error
	return list, total, err
}

// GetByPlayerIDLatest 取指定球员在最新一批中的 IPI 记录
func (r *IPIRepository) GetByPlayerIDLatest(ctx context.Context, playerID uint) (*entity.PlayerIPI, error) {
	t, ok, err := r.GetLatestCalculatedAt(ctx)
	if err != nil || !ok {
		return nil, err
	}
	var row entity.PlayerIPI
	err = r.model(ctx).Where("player_id = ? AND calculated_at = ?", playerID, t).First(&row).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, err
	}
	return &row, nil
}
