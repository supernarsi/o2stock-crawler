package repositories

import (
	"context"
	"o2stock-crawler/internal/entity"
	"time"
)

// PlayerRepositoryInterface defines the contract for player data access
type PlayerRepositoryInterface interface {
	List(ctx context.Context, filter PlayerFilter) ([]entity.Player, error)
	GetByID(ctx context.Context, playerID uint) (*entity.Player, error)
	BatchGetByIDs(ctx context.Context, playerIDs []uint) ([]entity.Player, error)
	GetAllTxPlayers(ctx context.Context) ([]entity.Player, error)
	UpdatePower(ctx context.Context, playerID uint, power5, power10 float64) error
	UpdatePriceChanges(ctx context.Context, playerID uint, pc1d, pc7d float64) error
	AvgPriceByOVRSegment(ctx context.Context, ovr uint, radius int) (avgPrice float64, count int64, err error)
	AvgPriceGlobal(ctx context.Context) (float64, error)
}

// HistoryRepositoryInterface defines the contract for price history data access
type HistoryRepositoryInterface interface {
	GetByPlayerID(ctx context.Context, playerID uint, startTime time.Time, limit int) ([]entity.PlayerPriceHistory, error)
	GetPriceHistoryMap(ctx context.Context, startTime time.Time) (map[uint]entity.PlayerPriceHistory, error)
	GetRealtime(ctx context.Context, playerID uint) ([]entity.PlayerPriceHistory, error)
	Get5Days(ctx context.Context, playerID uint) ([]entity.PlayerPriceHistory, error)
	GetDailyK(ctx context.Context, playerID uint) ([]entity.PlayerPriceHistory, error)
	GetDays(ctx context.Context, playerID uint, days int) ([]entity.PlayerPriceHistory, error)
	GetMultiPlayersHistory(ctx context.Context, playerIDs []uint, limit int) (map[uint][]entity.PlayerPriceHistory, error)
	Create(ctx context.Context, history *entity.PlayerPriceHistory) error
}

// UserRepositoryInterface defines the contract for user data access
type UserRepositoryInterface interface {
	GetByOpenID(ctx context.Context, openID string) (*entity.User, error)
	GetByID(ctx context.Context, id uint) (*entity.User, error)
	Create(ctx context.Context, user *entity.User) error
	Update(ctx context.Context, user *entity.User) error
}

// FavRepositoryInterface defines the contract for user favorites data access
type FavRepositoryInterface interface {
	Count(ctx context.Context, userID, playerID uint) (int64, error)
	CountUserTotal(ctx context.Context, userID uint) (int64, error)
	Add(ctx context.Context, userID, playerID uint) error
	Remove(ctx context.Context, userID, playerID uint) error
	GetPlayerIDs(ctx context.Context, userID uint) ([]uint, error)
	GetFavMap(ctx context.Context, userID uint, playerIDs []uint) (map[uint]bool, error)
}

// OwnRepositoryInterface defines the contract for player ownership data access
type OwnRepositoryInterface interface {
	CountOwned(ctx context.Context, userID, playerID uint) (int64, error)
	GetByUserID(ctx context.Context, userID uint) ([]entity.UserPlayerOwn, error)
	GetByPlayerIDs(ctx context.Context, userID uint, playerIDs []uint) ([]entity.UserPlayerOwn, error)
	GetByRecordID(ctx context.Context, recordID, userID uint) (*entity.UserPlayerOwn, error)
	Create(ctx context.Context, userID, playerID, num, cost uint, dt time.Time) error
	MarkAsSold(ctx context.Context, userID, playerID, cost uint, dt time.Time) error
	Update(ctx context.Context, userID, recordID uint, updates map[string]interface{}) error
	Delete(ctx context.Context, userID, recordID uint) error
}

// StatsRepositoryInterface defines the contract for player statistics data access
type StatsRepositoryInterface interface {
	GetSeasonStats(ctx context.Context, txPlayerID uint) (*entity.PlayerSeasonStats, error)
	GetRecentGameStats(ctx context.Context, txPlayerID uint, limit int) ([]entity.PlayerGameStats, error)
}

// Compile-time interface compliance checks
var _ PlayerRepositoryInterface = (*PlayerRepository)(nil)
var _ UserRepositoryInterface = (*UserRepository)(nil)
var _ FavRepositoryInterface = (*FavRepository)(nil)
var _ OwnRepositoryInterface = (*OwnRepository)(nil)
var _ StatsRepositoryInterface = (*StatsRepository)(nil)

// Note: HistoryRepository interface check is implicit as the interface is implemented
