package service

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"sort"
	"strconv"
	"strings"

	"o2stock-crawler/internal/db/repositories"
	"o2stock-crawler/internal/entity"
)

// ImportGameData 导入游戏数据 JSON 到数据库。
func (s *LineupRecommendService) ImportGameData(ctx context.Context, dataDir string) error {
	// 1. 读取 team_id.json → 构建 teamId → teamName 映射
	teamIDMap, err := s.loadTeamIDMap(dataDir + "/team_id.json")
	if err != nil {
		return fmt.Errorf("读取 team_id.json 失败: %w", err)
	}
	log.Printf("加载球队映射: %d 支球队", len(teamIDMap))

	// 2. 读取 match_data.json → 获取比赛列表
	matches, err := s.loadMatchData(dataDir + "/match_data.json")
	if err != nil {
		return fmt.Errorf("读取 match_data.json 失败: %w", err)
	}
	log.Printf("加载比赛数据: %d 场比赛", len(matches))

	// 3. 读取 player_salary.json → 获取球员列表
	playerSalaries, err := s.loadPlayerSalary(dataDir + "/player_salary.json")
	if err != nil {
		return fmt.Errorf("读取 player_salary.json 失败: %w", err)
	}
	log.Printf("加载球员工资: %d 名球员", len(playerSalaries))

	// 4. 构建 teamId → matchId 映射 和 match 信息映射
	teamToMatch := make(map[string]*MatchData)
	gameDate := ""
	for i := range matches {
		match := &matches[i]
		teamToMatch[match.IHomeTeamId] = match
		teamToMatch[match.IAwayTeamId] = match

		if gameDate == "" {
			gameDate = match.DtDate
			continue
		}
		if match.DtDate != gameDate {
			return fmt.Errorf("match_data.json 包含多个比赛日期: %s / %s", gameDate, match.DtDate)
		}
	}
	if gameDate == "" {
		return fmt.Errorf("无法确定比赛日期")
	}
	log.Printf("比赛日期: %s", gameDate)

	// 5. 遍历球员列表，构建 NBAGamePlayer 对象
	playerMap := make(map[uint]entity.NBAGamePlayer)
	invalidCount := 0

	for _, ps := range playerSalaries {
		match, ok := teamToMatch[ps.ITeamId]
		if !ok {
			log.Printf("警告: 球员 %s (team %s) 未找到对应比赛", ps.SPlayerName, ps.ITeamId)
			continue
		}

		nbaPlayerID, err := strconv.ParseUint(strings.TrimSpace(ps.IPlayerId), 10, 32)
		if err != nil || nbaPlayerID == 0 {
			invalidCount++
			log.Printf("警告: 跳过非法 iPlayerId=%q (name=%s)", ps.IPlayerId, ps.SPlayerName)
			continue
		}

		salary, err := strconv.ParseUint(strings.TrimSpace(ps.ISalary), 10, 32)
		if err != nil {
			invalidCount++
			log.Printf("警告: 跳过非法 iSalary=%q (name=%s)", ps.ISalary, ps.SPlayerName)
			continue
		}

		combatPower, err := strconv.ParseFloat(strings.TrimSpace(ps.FCombatPower), 64)
		if err != nil {
			invalidCount++
			log.Printf("警告: 跳过非法 fCombatPower=%q (name=%s)", ps.FCombatPower, ps.SPlayerName)
			continue
		}

		position, err := strconv.ParseUint(strings.TrimSpace(ps.IPosition), 10, 8)
		if err != nil {
			invalidCount++
			log.Printf("警告: 跳过非法 iPosition=%q (name=%s)", ps.IPosition, ps.SPlayerName)
			continue
		}

		isHome := ps.ITeamId == match.IHomeTeamId
		teamName := teamIDMap[ps.ITeamId]
		if teamName == "" {
			teamName = ps.ITeamId
		}

		playerMap[uint(nbaPlayerID)] = entity.NBAGamePlayer{
			GameDate:     gameDate,
			MatchID:      match.IMatchId,
			NBAPlayerID:  uint(nbaPlayerID),
			NBATeamID:    ps.ITeamId,
			PlayerName:   ps.SPlayerName,
			PlayerEnName: ps.SPlayerEnName,
			TeamName:     teamName,
			IsHome:       isHome,
			Salary:       uint(salary),
			CombatPower:  combatPower,
			Position:     uint(position),
		}
	}

	players := make([]entity.NBAGamePlayer, 0, len(playerMap))
	for _, p := range playerMap {
		players = append(players, p)
	}
	sort.Slice(players, func(i, j int) bool {
		return players[i].NBAPlayerID < players[j].NBAPlayerID
	})
	if len(players) == 0 {
		return fmt.Errorf("没有可导入的候选球员")
	}

	// 6. 按日期全量替换，避免旧候选残留
	repo := repositories.NewNBAGamePlayerRepository(s.db.DB)
	if err := repo.ReplaceByGameDate(ctx, gameDate, players); err != nil {
		return fmt.Errorf("导入球员数据失败: %w", err)
	}

	log.Printf("成功导入 %d 名球员到 nba_game_player 表 (日期: %s, 非法记录: %d)", len(players), gameDate, invalidCount)
	return nil
}

// loadTeamIDMap 读取 team_id.json 并构建 teamID -> teamName 映射。
func (s *LineupRecommendService) loadTeamIDMap(path string) (map[string]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var raw map[string]string
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, err
	}
	// 反转：teamName → teamId 变成 teamId → teamName
	result := make(map[string]string)
	for name, id := range raw {
		result[id] = name
	}
	return result, nil
}

func (s *LineupRecommendService) loadMatchData(path string) ([]MatchData, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var matches []MatchData
	if err := json.Unmarshal(data, &matches); err != nil {
		return nil, err
	}
	return matches, nil
}

func (s *LineupRecommendService) loadPlayerSalary(path string) ([]PlayerSalary, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var players []PlayerSalary
	if err := json.Unmarshal(data, &players); err != nil {
		return nil, err
	}
	return players, nil
}

// --- 通用工具函数 ---
