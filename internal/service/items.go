package service

import (
	"context"
	"o2stock-crawler/internal/db"
	"o2stock-crawler/internal/db/repositories"
	"o2stock-crawler/internal/dto"
	"o2stock-crawler/internal/entity"
	"time"
)

// ItemListOptions 道具列表查询参数
type ItemListOptions struct {
	Page     int
	Limit    int
	OrderBy  string
	OrderAsc bool
	ItemName string
}

// ItemsService 道具相关服务
type ItemsService struct {
	db *db.DB
}

// NewItemsService 创建 ItemsService
func NewItemsService(database *db.DB) *ItemsService {
	return &ItemsService{db: database}
}

// ListItems 查询道具列表（不分页，最多 Limit 条，默认 100）
func (s *ItemsService) ListItems(ctx context.Context, opts ItemListOptions) ([]dto.Item, error) {
	itemRepo := repositories.NewItemRepository(s.db.DB)
	limit := opts.Limit
	if limit <= 0 {
		limit = 100
	}
	filter := repositories.ItemFilter{
		Page:     1, // 不分页，只取前 limit 条
		Limit:    limit,
		OrderBy:  opts.OrderBy,
		OrderAsc: opts.OrderAsc,
		ItemName: opts.ItemName,
	}
	items, err := itemRepo.List(ctx, filter)
	if err != nil {
		return nil, err
	}
	return s.mapItemsToDTO(items), nil
}

// GetItemHistory 获取单个道具信息及其价格历史（默认最近 24 小时，最多 limit 条）
func (s *ItemsService) GetItemHistory(ctx context.Context, itemID uint, limit int) (*dto.Item, []*dto.ItemPriceHistoryRow, error) {
	itemRepo := repositories.NewItemRepository(s.db.DB)
	historyRepo := repositories.NewItemHistoryRepository(s.db.DB)

	item, err := itemRepo.GetByItemID(ctx, itemID)
	if err != nil {
		return nil, nil, err
	}
	if limit <= 0 {
		limit = 500
	}
	startTime := time.Now().Add(-24 * time.Hour)
	rows, err := historyRepo.GetByItemID(ctx, itemID, startTime, limit)
	if err != nil {
		return nil, nil, err
	}
	itemDTO := s.itemToDTO(item)
	historyDTO := s.mapItemHistoryToDTO(rows)
	return &itemDTO, historyDTO, nil
}

func (s *ItemsService) mapItemsToDTO(items []entity.Item) []dto.Item {
	out := make([]dto.Item, len(items))
	for i := range items {
		out[i] = s.itemToDTO(&items[i])
	}
	return out
}

func (s *ItemsService) itemToDTO(e *entity.Item) dto.Item {
	return dto.Item{
		ItemID:             e.ItemID,
		Name:               e.Name,
		Desc:               e.Desc,
		Icon:               e.Icon,
		PriceStandard:      e.PriceStandard,
		PriceCurrentLowest: e.PriceCurrentLowest,
		PriceChange1d:      e.PriceChange1d,
		PriceChange7d:      e.PriceChange7d,
	}
}

func (s *ItemsService) mapItemHistoryToDTO(rows []entity.ItemPriceHistory) []*dto.ItemPriceHistoryRow {
	out := make([]*dto.ItemPriceHistoryRow, len(rows))
	for i := range rows {
		r := &rows[i]
		out[i] = &dto.ItemPriceHistoryRow{
			ItemID:           r.ItemID,
			AtDate:           r.AtDate,
			AtDateHourStr:    r.AtDateHour,
			PriceStandard:    uint32(r.PriceStandard),
			PriceCurrentSale: uint32(r.PriceCurrentSale),
		}
	}
	return out
}
