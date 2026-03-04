package service

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"o2stock-crawler/internal/db/repositories"
	"o2stock-crawler/internal/entity"
)

const todayNBATotalPrepareFileName = "today_nba_total_prepare.json"

// nbaTeamIDToName 内置 NBA 官方 team_id 到中文队名映射。
// 数据源固定后不再依赖 team_id.json 文件。
var nbaTeamIDToName = map[string]string{
	"1610612737": "老鹰",
	"1610612738": "凯尔特人",
	"1610612739": "骑士",
	"1610612740": "鹈鹕",
	"1610612741": "公牛",
	"1610612742": "独行侠",
	"1610612743": "掘金",
	"1610612744": "勇士",
	"1610612745": "火箭",
	"1610612746": "快船",
	"1610612747": "湖人",
	"1610612748": "热火",
	"1610612749": "雄鹿",
	"1610612750": "森林狼",
	"1610612751": "篮网",
	"1610612752": "尼克斯",
	"1610612753": "魔术",
	"1610612754": "步行者",
	"1610612755": "76人",
	"1610612756": "太阳",
	"1610612757": "开拓者",
	"1610612758": "国王",
	"1610612759": "马刺",
	"1610612760": "雷霆",
	"1610612761": "猛龙",
	"1610612762": "爵士",
	"1610612763": "灰熊",
	"1610612764": "奇才",
	"1610612765": "活塞",
	"1610612766": "黄蜂",
}

type todayNBATotalPreparePayload struct {
	JData struct {
		PlayerData struct {
			MatchData     []MatchData    `json:"sMatchData"`
			ContestPlayer []PlayerSalary `json:"sContestPlayer"`
		} `json:"playerData"`
	} `json:"jData"`
}

// ImportGameData 从 today_nba_total_prepare.json 导入候选球员数据。
func (s *LineupRecommendService) ImportGameData(ctx context.Context, dataDir string) error {
	preparePath := filepath.Join(strings.TrimSpace(dataDir), todayNBATotalPrepareFileName)
	raw, err := os.ReadFile(preparePath)
	if err != nil {
		return fmt.Errorf("读取 %s 失败: %w", todayNBATotalPrepareFileName, err)
	}

	matches, playerSalaries, err := parseTodayNBATotalPrepare(raw)
	if err != nil {
		return fmt.Errorf("解析 %s 失败: %w", todayNBATotalPrepareFileName, err)
	}
	log.Printf("加载比赛数据: %d 场比赛", len(matches))
	log.Printf("加载球员工资: %d 名球员", len(playerSalaries))

	// 构建 teamId -> match 映射，并校验仅包含单日数据
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
			return fmt.Errorf("today_nba_total_prepare.json 包含多个比赛日期: %s / %s", gameDate, match.DtDate)
		}
	}
	if gameDate == "" {
		return fmt.Errorf("无法确定比赛日期")
	}
	log.Printf("比赛日期: %s", gameDate)

	// 构建候选球员，按 nba_player_id 去重
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
		teamName := teamNameByID(ps.ITeamId)

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

	repo := repositories.NewNBAGamePlayerRepository(s.db.DB)
	if err := repo.ReplaceByGameDate(ctx, gameDate, players); err != nil {
		return fmt.Errorf("导入球员数据失败: %w", err)
	}

	log.Printf("成功导入 %d 名球员到 nba_game_player 表 (日期: %s, 非法记录: %d)", len(players), gameDate, invalidCount)
	return nil
}

func parseTodayNBATotalPrepare(raw []byte) ([]MatchData, []PlayerSalary, error) {
	var payload todayNBATotalPreparePayload
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil, nil, err
	}

	matches := payload.JData.PlayerData.MatchData
	players := payload.JData.PlayerData.ContestPlayer
	if len(matches) == 0 {
		return nil, nil, fmt.Errorf("sMatchData 为空")
	}
	if len(players) == 0 {
		return nil, nil, fmt.Errorf("sContestPlayer 为空")
	}
	return matches, players, nil
}

func teamNameByID(teamID string) string {
	if name, ok := nbaTeamIDToName[strings.TrimSpace(teamID)]; ok && name != "" {
		return name
	}
	return teamID
}
