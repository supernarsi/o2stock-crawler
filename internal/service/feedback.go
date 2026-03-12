package service

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"o2stock-crawler/internal/db"
	"o2stock-crawler/internal/db/repositories"
	"o2stock-crawler/internal/entity"

	jsoniter "github.com/json-iterator/go"
)

type FeedbackService struct {
	db       *db.DB
	dbConfig *db.Config
	repo     repositories.FeedbackRepositoryInterface
	userRepo repositories.UserRepositoryInterface
}

func NewFeedbackService(db *db.DB, dbConfig *db.Config, repo repositories.FeedbackRepositoryInterface, userRepo repositories.UserRepositoryInterface) *FeedbackService {
	return &FeedbackService{
		db:       db,
		dbConfig: dbConfig,
		repo:     repo,
		userRepo: userRepo,
	}
}

func (s *FeedbackService) SubmitFeedback(ctx context.Context, uid uint, code, content, appVersion string, ip []byte, os uint8) error {
	if content == "" {
		return fmt.Errorf("feedback content cannot be empty")
	}

	var wxOpenID string

	if uid > 0 {
		user, err := s.userRepo.GetByID(ctx, uid)
		if err == nil && user != nil {
			wxOpenID = user.WxOpenID
		} else {
			log.Printf("submit feedback: failed to get user by id %d: %v", uid, err)
		}
	} else if code != "" {
		wxResp, err := s.code2Session(ctx, code)
		if err == nil && wxResp != nil {
			wxOpenID = wxResp.OpenID
		} else {
			log.Printf("submit feedback: failed to get openid from code: %v", err)
		}
	}

	feedback := &entity.Feedback{
		UID:        uid,
		WxOpenID:   wxOpenID,
		Content:    content,
		AppVersion: appVersion,
		IP:         ip,
		OS:         os,
	}

	if err := s.repo.Create(ctx, feedback); err != nil {
		return fmt.Errorf("service submit feedback: %w", err)
	}

	return nil
}

func (s *FeedbackService) code2Session(ctx context.Context, code string) (*WechatLoginResponse, error) {
	if s.dbConfig.WxAppID == "" || s.dbConfig.WxAppSecret == "" {
		return nil, fmt.Errorf("missing wechat config")
	}
	url := fmt.Sprintf("https://api.weixin.qq.com/sns/jscode2session?appid=%s&secret=%s&js_code=%s&grant_type=authorization_code",
		s.dbConfig.WxAppID, s.dbConfig.WxAppSecret, code)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var result WechatLoginResponse
	if err := jsoniter.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}
	if result.ErrCode != 0 {
		return nil, fmt.Errorf("wechat api error: %d %s", result.ErrCode, result.ErrMsg)
	}
	return &result, nil
}
