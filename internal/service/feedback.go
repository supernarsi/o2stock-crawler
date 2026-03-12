package service

import (
	"context"
	"fmt"
	"o2stock-crawler/internal/db"
	"o2stock-crawler/internal/db/repositories"
	"o2stock-crawler/internal/entity"
)

type FeedbackService struct {
	db   *db.DB
	repo repositories.FeedbackRepositoryInterface
}

func NewFeedbackService(db *db.DB, repo repositories.FeedbackRepositoryInterface) *FeedbackService {
	return &FeedbackService{
		db:   db,
		repo: repo,
	}
}

func (s *FeedbackService) SubmitFeedback(ctx context.Context, uid uint, openID, content, appVersion string, ip []byte, os uint8) error {
	if content == "" {
		return fmt.Errorf("feedback content cannot be empty")
	}

	feedback := &entity.Feedback{
		UID:        uid,
		WxOpenID:   openID,
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
