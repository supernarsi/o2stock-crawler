package repositories

import (
	"context"

	"gorm.io/gorm"
)

// baseRepository provides common GORM query initialization helpers
type baseRepository[T any] struct {
	db *gorm.DB
}

// db returns a GORM DB session with context
func (r *baseRepository[T]) ctx(ctx context.Context) *gorm.DB {
	return r.db.WithContext(ctx)
}

// model returns a GORM DB session with context and the target model set
func (r *baseRepository[T]) model(ctx context.Context) *gorm.DB {
	var m T
	return r.db.WithContext(ctx).Model(&m)
}
