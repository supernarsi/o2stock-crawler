package db

import (
	"context"
	"o2stock-crawler/internal/model"
)

// PlayerStatsQuery 球员统计数据查询
type PlayerStatsQuery struct {
	nbaPlayerID uint
}

// NewPlayerStatsQuery 创建一个 PlayerStatsQuery
func NewPlayerStatsQuery(nbaPlayerID uint) *PlayerStatsQuery {
	return &PlayerStatsQuery{
		nbaPlayerID: nbaPlayerID,
	}
}

// GetSeasonStats 查询球员最近赛季平均数据
func (q *PlayerStatsQuery) GetSeasonStats(ctx context.Context, database *DB) (*model.PlayerSeasonStats, error) {
	query := `SELECT id, player_id, player_name, season, season_type, games_played, minutes, points, rebounds, rebounds_offensive, rebounds_defensive, assists, turnovers, steals, blocks, fouls, field_goal_percentage, three_point_percentage, free_throw_percentage, updated_at, created_at 
	FROM player_season_stats 
	WHERE player_id = ? 
	ORDER BY season DESC, season_type DESC 
	LIMIT 1`

	row := database.QueryRowContext(ctx, query, q.nbaPlayerID)
	var s model.PlayerSeasonStats
	err := row.Scan(
		&s.ID,
		&s.PlayerID,
		&s.PlayerName,
		&s.Season,
		&s.SeasonType,
		&s.GamesPlayed,
		&s.Minutes,
		&s.Points,
		&s.Rebounds,
		&s.ReboundsOffensive,
		&s.ReboundsDefensive,
		&s.Assists,
		&s.Turnovers,
		&s.Steals,
		&s.Blocks,
		&s.Fouls,
		&s.FieldGoalPercentage,
		&s.ThreePointPercentage,
		&s.FreeThrowPercentage,
		&s.UpdatedAt,
		&s.CreatedAt,
	)
	if err != nil {
		return nil, err
	}
	return &s, nil
}

// GetRecentGameStats 查询球员最近几场比赛数据
func (q *PlayerStatsQuery) GetRecentGameStats(ctx context.Context, database *DB, limit int) ([]*model.PlayerGameStats, error) {
	query := `SELECT id, player_id, game_id, game_date, player_team_name, vs_team_name, is_home, points, rebounds, assists, steals, blocks, turnovers, minutes, field_goals_made, field_goals_attempted, three_pointers_made, three_pointers_attempted, free_throws_made, free_throws_attempted, created_at 
	FROM player_game_stats 
	WHERE player_id = ? 
	ORDER BY game_date DESC 
	LIMIT ?`

	rows, err := database.QueryContext(ctx, query, q.nbaPlayerID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make([]*model.PlayerGameStats, 0, limit)
	for rows.Next() {
		var s model.PlayerGameStats
		var isHomeBytes []byte
		err := rows.Scan(
			&s.ID,
			&s.PlayerID,
			&s.GameID,
			&s.GameDate,
			&s.PlayerTeamName,
			&s.VsTeamName,
			&isHomeBytes,
			&s.Points,
			&s.Rebounds,
			&s.Assists,
			&s.Steals,
			&s.Blocks,
			&s.Turnovers,
			&s.Minutes,
			&s.FieldGoalsMade,
			&s.FieldGoalsAttempted,
			&s.ThreePointersMade,
			&s.ThreePointersAttempted,
			&s.FreeThrowsMade,
			&s.FreeThrowsAttempted,
			&s.CreatedAt,
		)
		if err != nil {
			return nil, err
		}
		// BIT(1) handling for MySQL in Go
		if len(isHomeBytes) > 0 {
			s.IsHome = isHomeBytes[0] != 0
		}
		result = append(result, &s)
	}
	return result, nil
}
