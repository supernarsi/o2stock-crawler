package service

import (
	"context"
	"fmt"
	"log"
	"sync"
	"sync/atomic"
	"time"

	"o2stock-crawler/internal/db"
	"o2stock-crawler/internal/db/repositories"
	"o2stock-crawler/internal/entity"
	"o2stock-crawler/internal/wechat"
)

const lineupNotifyWorkers = 5

// LineupNotifyService 阵容推荐推送服务
type LineupNotifyService struct {
	db            *db.DB
	wechat        *wechat.Client
	subscribeRepo *repositories.LineupSubscribeRepository
	recommendRepo *repositories.LineupRecommendationRepository
	userRepo      *repositories.UserRepository
}

func NewLineupNotifyService(database *db.DB, wc *wechat.Client) *LineupNotifyService {
	return &LineupNotifyService{
		db:            database,
		wechat:        wc,
		subscribeRepo: repositories.NewLineupSubscribeRepository(database.DB),
		recommendRepo: repositories.NewLineupRecommendationRepository(database.DB),
		userRepo:      repositories.NewUserRepository(database.DB),
	}
}

// RunDailyNotify 执行每日阵容推荐推送
func (s *LineupNotifyService) RunDailyNotify(ctx context.Context) error {
	log.Printf("[lineup-notify] 开始执行每日阵容订阅推送")

	// 1. 获取当日 Rank=1 的 AI 推荐阵容
	todayStr := time.Now().Format("2006-01-02")
	recs, err := s.recommendRepo.GetByDatesAndType(ctx, []string{todayStr}, entity.LineupRecommendationTypeAIRecommended)
	if err != nil {
		return fmt.Errorf("get today's recommendation: %w", err)
	}

	var targetRec *entity.LineupRecommendation
	for i := range recs {
		if recs[i].Rank == 1 {
			targetRec = &recs[i]
			break
		}
	}

	if targetRec == nil {
		log.Printf("[lineup-notify] 当日 (%s) 无 Rank=1 的 AI 推荐阵容，跳过推送", todayStr)
		return nil
	}

	// 2. 获取所有活跃订阅用户
	subs, err := s.subscribeRepo.GetActiveSubscribers(ctx)
	if err != nil {
		return fmt.Errorf("get active subscribers: %w", err)
	}
	if len(subs) == 0 {
		log.Printf("[lineup-notify] 暂无活跃订阅用户，跳过推送")
		return nil
	}

	uids := make([]uint, 0, len(subs))
	for _, sub := range subs {
		uids = append(uids, sub.UserID)
	}

	// 3. 获取用户 OpenID
	userMap, err := s.userRepo.GetByIDs(ctx, uids)
	if err != nil {
		return fmt.Errorf("get users: %w", err)
	}

	// 4. 并发推送
	cfg := s.wechat.GetConfig()
	templateID := cfg.LineupSubscribeTemplateID
	page := cfg.LineupSubscribePage

	var successCount atomic.Int32
	sem := make(chan struct{}, lineupNotifyWorkers)
	var wg sync.WaitGroup

	for _, sub := range subs {
		user, ok := userMap[sub.UserID]
		if !ok || user.WxOpenID == "" {
			continue
		}

		wg.Add(1)
		go func(openid string, uid uint) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			if err := s.wechat.SendLineupNotify(openid, templateID, page, targetRec.TotalPredictedPower); err != nil {
				log.Printf("[lineup-notify] 推送失败 uid=%d: %v", uid, err)
				return
			}

			// 推送成功，更新统计信息
			if err := s.subscribeRepo.UpdatePushStats(ctx, uid); err != nil {
				log.Printf("[lineup-notify] 更新推送统计失败 uid=%d: %v", uid, err)
			} else {
				successCount.Add(1)
			}
		}(user.WxOpenID, sub.UserID)
	}

	wg.Wait()
	log.Printf("[lineup-notify] 每日阵容推送完成，成功推送人数: %d/%d", successCount.Load(), len(subs))

	return nil
}
