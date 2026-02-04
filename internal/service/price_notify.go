package service

import (
	"context"
	"fmt"
	"log"
	"time"

	"o2stock-crawler/internal/consts"
	"o2stock-crawler/internal/db"
	"o2stock-crawler/internal/db/repositories"
	"o2stock-crawler/internal/entity"
	"o2stock-crawler/internal/wechat"
)

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
	if len(playerIDs) == 0 {
		return nil
	}

	ownRepo := repositories.NewOwnRepository(s.db.DB)
	playerRepo := repositories.NewPlayerRepository(s.db.DB)
	userRepo := repositories.NewUserRepository(s.db.DB)

	owns, err := ownRepo.GetActiveNotifyOwnsByPlayerIDs(ctx, playerIDs)
	if err != nil {
		return fmt.Errorf("get active notify owns: %w", err)
	}
	if len(owns) == 0 {
		return nil
	}

	var toCheck []entity.UserPlayerOwn
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
		pids = append(pids, o.PlayerID)
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

	now := time.Now()
	for _, o := range toCheck {
		player, ok := playerMap[o.PlayerID]
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

		var shouldSend bool
		var remark string
		switch o.NotifyType {
		case consts.NotifyTypeBreakEven:
			if effectivePrice > costAvg {
				shouldSend = true
				remark = "球员已达到回本价格"
			}
		case consts.NotifyTypeProfit15:
			if (effectivePrice-costAvg)/costAvg > 0.15 {
				shouldSend = true
				remark = "球员已达到盈利价格"
			}
		default:
			continue
		}
		if !shouldSend {
			continue
		}

		if err := s.wechat.SendPriceNotify(
			user.WxOpenID,
			player.ShowName,
			fmt.Sprintf("%.0f", currentPrice),
			fmt.Sprintf("%.0f", costAvg),
			remark,
		); err != nil {
			log.Printf("[price-notify] send wechat failed own_id=%d uid=%d pid=%d: %v", o.ID, o.UserID, o.PlayerID, err)
			continue
		}

		if err := ownRepo.SetNotifyTime(ctx, o.ID, now); err != nil {
			log.Printf("[price-notify] set notify_time failed own_id=%d: %v", o.ID, err)
		}
	}

	return nil
}

