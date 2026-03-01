package repositories

import (
	"context"
	"o2stock-crawler/internal/entity"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// TaskStatusRepository 任务状态仓储
type TaskStatusRepository struct {
	db *gorm.DB
}

// NewTaskStatusRepository 创建实例
func NewTaskStatusRepository(db *gorm.DB) *TaskStatusRepository {
	return &TaskStatusRepository{db: db}
}

// Upsert 更新或插入指定任务的最近成功时间
func (r *TaskStatusRepository) Upsert(ctx context.Context, taskName string, successAt time.Time) error {
	status := entity.TaskStatus{
		TaskName:      taskName,
		LastSuccessAt: successAt,
		UpdatedAt:     time.Now(),
	}
	return r.db.WithContext(ctx).
		Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "task_name"}},
			DoUpdates: clause.AssignmentColumns([]string{"last_success_at", "updated_at"}),
		}).Create(&status).Error
}

// GetAll 查询所有任务状态记录
func (r *TaskStatusRepository) GetAll(ctx context.Context) ([]entity.TaskStatus, error) {
	var statuses []entity.TaskStatus
	err := r.db.WithContext(ctx).Find(&statuses).Error
	return statuses, err
}
