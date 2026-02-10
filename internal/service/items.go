package service

import (
	"context"
	"math"
	"o2stock-crawler/internal/consts"
	"o2stock-crawler/internal/db"
	"o2stock-crawler/internal/db/repositories"
	"o2stock-crawler/internal/dto"
	"o2stock-crawler/internal/entity"
)

// ItemListOptions 道具列表查询参数
type ItemListOptions struct {
	Page     int
	Limit    int
	OrderBy  string
	OrderAsc bool
	ItemName string
	SoldOut  bool // true 时仅返回 price_current_lowest = 0 的道具
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
	return s.ListItemsWithOwned(ctx, opts, nil)
}

// ListItemsWithOwned 查询道具列表，并可选地包含用户的拥有信息
func (s *ItemsService) ListItemsWithOwned(ctx context.Context, opts ItemListOptions, userID *uint) ([]dto.Item, error) {
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
		SoldOut:  opts.SoldOut,
	}
	items, err := itemRepo.List(ctx, filter)
	if err != nil {
		return nil, err
	}

	out := s.mapItemsToDTO(items)
	// ensure owned is not null
	for i := range out {
		out[i].Owned = []*dto.OwnInfo{}
	}

	if userID != nil && len(items) > 0 {
		itemIDs := make([]uint, len(items))
		for i, it := range items {
			itemIDs[i] = it.ItemID
		}
		ownRepo := repositories.NewOwnRepository(s.db.DB)
		ownRecords, err := ownRepo.GetByGoodsIDs(ctx, *userID, itemIDs, consts.OwnGoodsItem)
		if err != nil {
			return nil, err
		}
		ownedMap := ToOwnInfoDTOPointerMap(ownRecords)

		favRepo := repositories.NewItemFavRepository(s.db.DB)
		favMap, _ := favRepo.GetFavMap(ctx, *userID, itemIDs)

		for i := range out {
			if owned, ok := ownedMap[out[i].ItemID]; ok {
				out[i].Owned = owned
			}
			if favMap != nil {
				out[i].IsFav = favMap[out[i].ItemID]
			}
		}
	}

	return out, nil
}

// GetItemHistory 获取单个道具信息及其价格历史（默认 realtime 模式）
func (s *ItemsService) GetItemHistory(ctx context.Context, itemID uint, limit int) (*dto.Item, []*dto.ItemPriceHistoryRow, error) {
	return s.GetItemHistoryWithOwned(ctx, itemID, "realtime", limit, nil)
}

// GetItemHistoryWithOwned 获取单个道具信息及其价格历史，并可选包含用户拥有信息
// mode: realtime | 5d | 10d | 30d | dailyk，与 GET /player-history 一致；limit 仅 realtime 时生效（默认 500）
func (s *ItemsService) GetItemHistoryWithOwned(ctx context.Context, itemID uint, mode string, limit int, userID *uint) (*dto.Item, []*dto.ItemPriceHistoryRow, error) {
	itemRepo := repositories.NewItemRepository(s.db.DB)
	historyRepo := repositories.NewItemHistoryRepository(s.db.DB)

	item, err := itemRepo.GetByItemID(ctx, itemID)
	if err != nil {
		return nil, nil, err
	}
	if mode == "" {
		mode = "realtime"
	}
	if limit <= 0 {
		limit = 500
	}

	var rows []entity.ItemPriceHistory
	switch mode {
	case "realtime":
		rows, err = historyRepo.GetRealtime(ctx, itemID)
		if err != nil {
			return nil, nil, err
		}
		rows = s.sampleItemRealtime(rows)
	case "5d":
		rows, err = historyRepo.Get5Days(ctx, itemID)
		if err != nil {
			return nil, nil, err
		}
		rows = s.sampleItem5Days(rows)
	case "10d":
		rows, err = historyRepo.GetDays(ctx, itemID, 10)
		if err != nil {
			return nil, nil, err
		}
	case "30d":
		rows, err = historyRepo.GetDays(ctx, itemID, 30)
		if err != nil {
			return nil, nil, err
		}
	case "dailyk":
		rows, err = historyRepo.GetDailyK(ctx, itemID)
		if err != nil {
			return nil, nil, err
		}
	default:
		// 由 controller 校验 mode，此处不返回 error 避免重复
		rows = nil
	}

	itemDTO := s.itemToDTO(item)
	itemDTO.Owned = []*dto.OwnInfo{}
	if userID != nil {
		ownRepo := repositories.NewOwnRepository(s.db.DB)
		ownRecords, err := ownRepo.GetByGoodsIDs(ctx, *userID, []uint{itemID}, consts.OwnGoodsItem)
		if err != nil {
			return nil, nil, err
		}
		ownedMap := ToOwnInfoDTOPointerMap(ownRecords)
		if owned, ok := ownedMap[itemID]; ok {
			itemDTO.Owned = owned
		}

		favRepo := repositories.NewItemFavRepository(s.db.DB)
		isFav, _ := favRepo.Count(ctx, *userID, itemID)
		itemDTO.IsFav = isFav > 0
	}
	historyDTO := s.mapItemHistoryToDTO(rows)
	return &itemDTO, historyDTO, nil
}

// sampleItemRealtime 分时采样：每小时保留该小时第一个点，并保留最后一个点（与球员一致）
func (s *ItemsService) sampleItemRealtime(rows []entity.ItemPriceHistory) []entity.ItemPriceHistory {
	if len(rows) == 0 {
		return rows
	}
	var sampled []entity.ItemPriceHistory
	seenHour := make(map[string]bool)
	for i := 0; i < len(rows)-1; i++ {
		row := rows[i]
		key := row.AtDate.Format("2006010215") + row.AtHour
		if !seenHour[key] {
			sampled = append(sampled, row)
			seenHour[key] = true
		}
	}
	sampled = append(sampled, rows[len(rows)-1])
	return sampled
}

// sampleItem5Days 5 日采样：每天最多 4 个点（0, 1/3, 2/3, last 索引）+ 最后一个点（与球员一致）
func (s *ItemsService) sampleItem5Days(rows []entity.ItemPriceHistory) []entity.ItemPriceHistory {
	if len(rows) == 0 {
		return rows
	}
	dayMap := make(map[string][]entity.ItemPriceHistory)
	var days []string
	for i := 0; i < len(rows)-1; i++ {
		row := rows[i]
		dayKey := row.AtDate.Format("2006-01-02")
		if _, exists := dayMap[dayKey]; !exists {
			days = append(days, dayKey)
		}
		dayMap[dayKey] = append(dayMap[dayKey], row)
	}
	var sampled []entity.ItemPriceHistory
	for _, day := range days {
		dayRows := dayMap[day]
		count := len(dayRows)
		if count <= 4 {
			sampled = append(sampled, dayRows...)
		} else {
			step := float64(count-1) / 3.0
			for k := 0; k < 4; k++ {
				idx := int(math.Round(float64(k) * step))
				if idx >= count {
					idx = count - 1
				}
				sampled = append(sampled, dayRows[idx])
			}
		}
	}
	sampled = append(sampled, rows[len(rows)-1])
	return sampled
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
		Owned:              []*dto.OwnInfo{},
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
