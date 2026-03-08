package service

import (
	"context"
	"fmt"
	"log"
	"sort"
	"strings"

	"o2stock-crawler/internal/crawler"
	"o2stock-crawler/internal/db/repositories"
	"o2stock-crawler/internal/entity"
)

func (s *LineupRecommendService) ensureGamePlayersForDate(ctx context.Context, gameDate string) error {
	gamePlayerRepo := repositories.NewNBAGamePlayerRepository(s.db.DB)
	existingPlayers, err := gamePlayerRepo.GetByGameDate(ctx, gameDate)
	if err != nil {
		return fmt.Errorf("查询候选球员失败: %w", err)
	}
	if len(existingPlayers) > 0 {
		return nil
	}

	salaryCount, err := s.syncPlayerSalaryLibrary(ctx)
	if err != nil {
		return err
	}
	log.Printf("球员薪资库: %d 名球员", salaryCount)

	games, err := s.scheduleClient.GetGamesByDate(ctx, gameDate)
	if err != nil {
		return fmt.Errorf("获取官方赛程失败: %w", err)
	}
	if len(games) == 0 {
		log.Printf("官方赛程显示该日期无比赛: %s", gameDate)
		return nil
	}

	salaryRepo := repositories.NewNBAPlayerSalaryRepository(s.db.DB)
	teamIDs := collectScheduleTeamIDs(games)
	salaryRows, err := salaryRepo.GetByTeamIDs(ctx, teamIDs)
	if err != nil {
		return fmt.Errorf("查询球员薪资库失败: %w", err)
	}
	if len(salaryRows) == 0 {
		return fmt.Errorf("已获取到 %s 的官方赛程，但 nba_player_salary 中没有对应球队球员，请先准备历史 nba_game_player 数据", gameDate)
	}

	players, missingTeamIDs := buildGamePlayersFromSchedule(gameDate, games, salaryRows)
	if len(players) == 0 {
		return fmt.Errorf("已获取到 %s 的官方赛程，但无法构建候选球员", gameDate)
	}
	if err := gamePlayerRepo.ReplaceByGameDate(ctx, gameDate, players); err != nil {
		return fmt.Errorf("自动生成候选球员失败: %w", err)
	}

	log.Printf("已根据官方赛程自动生成候选池: 比赛=%d, 球员=%d", len(games), len(players))
	if len(missingTeamIDs) > 0 {
		log.Printf("警告: 以下球队在 nba_player_salary 中无球员数据: %s", strings.Join(missingTeamIDs, ", "))
	}
	return nil
}

func (s *LineupRecommendService) syncPlayerSalaryLibrary(ctx context.Context) (int, error) {
	gamePlayerRepo := repositories.NewNBAGamePlayerRepository(s.db.DB)
	salaryRepo := repositories.NewNBAPlayerSalaryRepository(s.db.DB)

	rows, err := gamePlayerRepo.ListLatestSalaryProfiles(ctx)
	if err != nil {
		return 0, fmt.Errorf("从 nba_game_player 构建薪资库失败: %w", err)
	}
	if len(rows) > 0 {
		if err := salaryRepo.BatchUpsert(ctx, rows); err != nil {
			return 0, fmt.Errorf("同步 nba_player_salary 失败: %w", err)
		}
		return len(rows), nil
	}

	count, err := salaryRepo.Count(ctx)
	if err != nil {
		return 0, fmt.Errorf("查询 nba_player_salary 数量失败: %w", err)
	}
	if count == 0 {
		return 0, fmt.Errorf("nba_player_salary 为空，且 nba_game_player 无历史数据，无法自动生成候选池")
	}

	return int(count), nil
}

func collectScheduleTeamIDs(games []crawler.NBAScheduleGame) []string {
	seen := make(map[string]struct{})
	var teamIDs []string

	for _, game := range games {
		for _, teamID := range []string{
			fmt.Sprintf("%d", game.HomeTeam.TeamID),
			fmt.Sprintf("%d", game.AwayTeam.TeamID),
		} {
			if teamID == "0" {
				continue
			}
			if _, ok := seen[teamID]; ok {
				continue
			}
			seen[teamID] = struct{}{}
			teamIDs = append(teamIDs, teamID)
		}
	}

	sort.Strings(teamIDs)
	return teamIDs
}

func buildGamePlayersFromSchedule(
	gameDate string,
	games []crawler.NBAScheduleGame,
	salaryRows []entity.NBAPlayerSalary,
) ([]entity.NBAGamePlayer, []string) {
	teamToGame := make(map[string]crawler.NBAScheduleGame)
	expectedTeams := make(map[string]struct{})
	for _, game := range games {
		homeTeamID := fmt.Sprintf("%d", game.HomeTeam.TeamID)
		awayTeamID := fmt.Sprintf("%d", game.AwayTeam.TeamID)
		teamToGame[homeTeamID] = game
		teamToGame[awayTeamID] = game
		expectedTeams[homeTeamID] = struct{}{}
		expectedTeams[awayTeamID] = struct{}{}
	}

	playerMap := make(map[uint]entity.NBAGamePlayer)
	seenTeams := make(map[string]struct{})
	for _, row := range salaryRows {
		match, ok := teamToGame[row.NBATeamID]
		if !ok {
			continue
		}

		seenTeams[row.NBATeamID] = struct{}{}
		playerMap[row.NBAPlayerID] = entity.NBAGamePlayer{
			GameDate:     gameDate,
			MatchID:      match.GameID,
			NBAPlayerID:  row.NBAPlayerID,
			NBATeamID:    row.NBATeamID,
			PlayerName:   row.PlayerName,
			PlayerEnName: row.PlayerEnName,
			TeamName:     fallbackTeamName(row.TeamName, row.NBATeamID),
			IsHome:       row.NBATeamID == fmt.Sprintf("%d", match.HomeTeam.TeamID),
			Salary:       row.Salary,
			CombatPower:  row.CombatPower,
			Position:     row.Position,
		}
	}

	players := make([]entity.NBAGamePlayer, 0, len(playerMap))
	for _, player := range playerMap {
		players = append(players, player)
	}
	sort.Slice(players, func(i, j int) bool {
		return players[i].NBAPlayerID < players[j].NBAPlayerID
	})

	var missingTeamIDs []string
	for teamID := range expectedTeams {
		if _, ok := seenTeams[teamID]; ok {
			continue
		}
		missingTeamIDs = append(missingTeamIDs, teamID)
	}
	sort.Strings(missingTeamIDs)

	return players, missingTeamIDs
}

func fallbackTeamName(teamName string, teamID string) string {
	if strings.TrimSpace(teamName) != "" {
		return teamName
	}
	return teamNameByID(teamID)
}
