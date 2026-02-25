package entity

import "time"

// TaskStatus 任务执行状态
type TaskStatus struct {
	ID            uint      `gorm:"primaryKey;autoIncrement"`
	TaskName      string    `gorm:"column:task_name;type:varchar(64);uniqueIndex"`
	LastSuccessAt time.Time `gorm:"column:last_success_at"`
	UpdatedAt     time.Time `gorm:"column:updated_at;autoUpdateTime"`
}

func (TaskStatus) TableName() string {
	return "task_status"
}
