package service

import (
	"context"
	"fmt"
	"log"
	"sync"
	"sync/atomic"
	"time"

	"o2stock-crawler/internal/consts"
	"o2stock-crawler/internal/db"
	"o2stock-crawler/internal/db/repositories"
	"o2stock-crawler/internal/entity"
	"o2stock-crawler/internal/wechat"
)

// 发送订阅消息的并发协程数
const priceNotifyWorkers = 5

// PriceNotifyService 盈利/回本订阅通知服务
type PriceNotifyService struct {
	db     *db.DB
	wechat *wechat.Client
}

func NewPriceNotifyService(database *db.DB, wc *wechat.Client) *PriceNotifyService {
	return &PriceNotifyService{db: database, wechat: wc}
}

// RunForPlayerIDs 仅针对给定球员 ID 检查订阅并发送通知（未发送过且达到条件才发，发送后更新 notify_time）
func (s *PriceNotifyService) RunForPlayerIDs(ctx context.Context, playerIDs []uint) error {
	// 打印开始发送订阅消息日志
	log.Printf("[price-notify] 开始发送订阅消息 playerIDs=%v", playerIDs)

	if len(playerIDs) == 0 {
		return nil
	}

	ownRepo := repositories.NewOwnRepository(s.db.DB)
	playerRepo := repositories.NewPlayerRepository(s.db.DB)
	userRepo := repositories.NewUserRepository(s.db.DB)

	owns, err := ownRepo.GetActiveNotifyOwnsByGoodsIDs(ctx, playerIDs, consts.OwnGoodsPlayer)
	if err != nil {
		return fmt.Errorf("get active notify owns: %w", err)
	}
	if len(owns) == 0 {
		return nil
	}

	var toCheck []entity.UserPOwn
	for i := range owns {
		// 设计：若已存在 notify_time，则不再重复发送
		if owns[i].NotifyTime == nil {
			toCheck = append(toCheck, owns[i])
		}
	}
	if len(toCheck) == 0 {
		return nil
	}

	pids := make([]uint, 0, len(toCheck))
	uids := make([]uint, 0, len(toCheck))
	for _, o := range toCheck {
		pids = append(pids, o.PID)
		uids = append(uids, o.UserID)
	}

	players, err := playerRepo.BatchGetByIDs(ctx, pids)
	if err != nil {
		return fmt.Errorf("batch get players: %w", err)
	}
	playerMap := make(map[uint]entity.Player, len(players))
	for _, p := range players {
		playerMap[p.PlayerID] = p
	}

	userMap, err := userRepo.GetByIDs(ctx, uids)
	if err != nil {
		return fmt.Errorf("get users: %w", err)
	}

	// 组装待发送任务（仅达到条件的记录）
	type sendTask struct {
		own        entity.UserPOwn
		openID     string
		currentStr string
		costStr    string
		remark     string
		player     *entity.Player
	}
	var tasks []sendTask
	for _, o := range toCheck {
		player, ok := playerMap[o.PID]
		if !ok {
			continue
		}
		user, ok := userMap[o.UserID]
		if !ok || user.WxOpenID == "" {
			continue
		}
		if o.BuyCount == 0 {
			continue
		}
		costAvg := float64(o.BuyPrice) / float64(o.BuyCount)
		if costAvg <= 0 {
			continue
		}
		currentPrice := float64(player.PriceStandard)
		effectivePrice := currentPrice * 0.75
		var remark string
		switch o.NotifyType {
		case consts.NotifyTypeBreakEven:
			if effectivePrice <= costAvg {
				continue
			}
			remark = "球员已达到回本价格"
		case consts.NotifyTypeProfit15:
			if (effectivePrice-costAvg)/costAvg <= 0.15 {
				continue
			}
			remark = "球员已盈利 15%"
		default:
			continue
		}
		tasks = append(tasks, sendTask{
			own:        o,
			openID:     user.WxOpenID,
			currentStr: fmt.Sprintf("%.0f", currentPrice),
			costStr:    fmt.Sprintf("%.0f", costAvg),
			remark:     remark,
			player:     &player,
		})
	}

	if len(tasks) == 0 {
		log.Printf("[price-notify] 没有需要发送的订阅消息")
		return nil
	}

	now := time.Now()
	var successCount atomic.Int32
	sem := make(chan struct{}, priceNotifyWorkers)
	var wg sync.WaitGroup
	for i := range tasks {
		task := tasks[i]
		wg.Add(1)
		go func(t sendTask) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			if err := s.wechat.SendPriceNotify(t.openID, t.currentStr, t.costStr, t.remark, t.player); err != nil {
				log.Printf("[price-notify] send wechat failed own_id=%d uid=%d pid=%d: %v", t.own.ID, t.own.UserID, t.own.PID, err)
				return
			}
			if err := ownRepo.SetNotifyTime(ctx, t.own.ID, now); err != nil {
				log.Printf("[price-notify] set notify_time failed own_id=%d: %v", t.own.ID, err)
				return
			}
			log.Printf("[price-notify] 发送订阅消息成功 own_id=%d uid=%d pid=%d: %s", t.own.ID, t.own.UserID, t.own.PID, t.remark)
			successCount.Add(1)
		}(task)
	}
	wg.Wait()
	log.Printf("[price-notify] 发送订阅消息完成，成功数量: %d/%d", successCount.Load(), len(tasks))

	return nil
}

// RunForItemIDs 仅针对给定道具 ID 检查订阅并发送通知（未发送过且达到条件才发，发送后更新 notify_time；与球员共用同一模板 id）
func (s *PriceNotifyService) RunForItemIDs(ctx context.Context, itemIDs []uint) error {
	if len(itemIDs) == 0 {
		return nil
	}
	log.Printf("[price-notify] 开始发送道具订阅消息 itemIDs 数量: %d", len(itemIDs))

	ownRepo := repositories.NewOwnRepository(s.db.DB)
	itemRepo := repositories.NewItemRepository(s.db.DB)
	userRepo := repositories.NewUserRepository(s.db.DB)

	owns, err := ownRepo.GetActiveNotifyOwnsByGoodsIDs(ctx, itemIDs, consts.OwnGoodsItem)
	if err != nil {
		return fmt.Errorf("get active notify owns by item ids: %w", err)
	}
	if len(owns) == 0 {
		log.Printf("[price-notify] 暂无用户订阅道具消息")
		return nil
	}

	var toCheck []entity.UserPOwn
	for i := range owns {
		if owns[i].NotifyTime == nil {
			toCheck = append(toCheck, owns[i])
		}
	}
	if len(toCheck) == 0 {
		return nil
	}

	ids := make([]uint, 0, len(toCheck))
	uids := make([]uint, 0, len(toCheck))
	for _, o := range toCheck {
		ids = append(ids, o.PID)
		uids = append(uids, o.UserID)
	}

	items, err := itemRepo.BatchGetByItemIDs(ctx, ids)
	if err != nil {
		return fmt.Errorf("batch get items: %w", err)
	}
	itemMap := make(map[uint]entity.Item, len(items))
	for _, it := range items {
		itemMap[it.ItemID] = it
	}

	userMap, err := userRepo.GetByIDs(ctx, uids)
	if err != nil {
		return fmt.Errorf("get users: %w", err)
	}

	type itemSendTask struct {
		own        entity.UserPOwn
		openID     string
		currentStr string
		costStr    string
		remark     string
		item       *entity.Item
	}
	var tasks []itemSendTask
	for _, o := range toCheck {
		item, ok := itemMap[o.PID]
		if !ok {
			continue
		}
		user, ok := userMap[o.UserID]
		if !ok || user.WxOpenID == "" {
			continue
		}
		if o.BuyCount == 0 {
			continue
		}
		costAvg := float64(o.BuyPrice) / float64(o.BuyCount)
		if costAvg <= 0 {
			continue
		}
		currentPrice := float64(item.PriceStandard)
		effectivePrice := currentPrice * 0.75
		var remark string
		switch o.NotifyType {
		case consts.NotifyTypeBreakEven:
			if effectivePrice <= costAvg {
				continue
			}
			remark = "道具已达到回本价格"
		case consts.NotifyTypeProfit15:
			if (effectivePrice-costAvg)/costAvg <= 0.15 {
				continue
			}
			remark = "道具已盈利 15%"
		default:
			continue
		}
		tasks = append(tasks, itemSendTask{
			own:        o,
			openID:     user.WxOpenID,
			currentStr: fmt.Sprintf("%.0f", currentPrice),
			costStr:    fmt.Sprintf("%.0f", costAvg),
			remark:     remark,
			item:       &item,
		})
	}

	if len(tasks) == 0 {
		log.Printf("[price-notify] 没有需要发送的道具订阅消息")
		return nil
	}

	now := time.Now()
	var successCount atomic.Int32
	sem := make(chan struct{}, priceNotifyWorkers)
	var wg sync.WaitGroup
	for i := range tasks {
		task := tasks[i]
		wg.Add(1)
		go func(t itemSendTask) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			if err := s.wechat.SendPriceNotifyForItem(t.openID, t.currentStr, t.costStr, t.remark, t.item); err != nil {
				log.Printf("[price-notify] send item wechat failed own_id=%d uid=%d item_id=%d: %v", t.own.ID, t.own.UserID, t.own.PID, err)
				return
			}
			if err := ownRepo.SetNotifyTime(ctx, t.own.ID, now); err != nil {
				log.Printf("[price-notify] set item notify_time failed own_id=%d: %v", t.own.ID, err)
				return
			}
			log.Printf("[price-notify] 发送道具订阅消息成功 own_id=%d uid=%d item_id=%d: %s", t.own.ID, t.own.UserID, t.own.PID, t.remark)
			successCount.Add(1)
		}(task)
	}
	wg.Wait()
	log.Printf("[price-notify] 道具订阅消息发送完成，成功数量: %d/%d", successCount.Load(), len(tasks))
	return nil
}
