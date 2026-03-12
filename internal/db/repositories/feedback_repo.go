package repositories

import (
	"context"
	"fmt"
	"o2stock-crawler/internal/entity"

	"gorm.io/gorm"
)

type FeedbackRepository struct {
	db *gorm.DB
}

func NewFeedbackRepository(db *gorm.DB) *FeedbackRepository {
	return &FeedbackRepository{db: db}
}

func (r *FeedbackRepository) Create(ctx context.Context, feedback *entity.Feedback) error {
	if err := r.db.WithContext(ctx).Create(feedback).Error; err != nil {
		return fmt.Errorf("repo feedback create: %w", err)
	}
	return nil
}
