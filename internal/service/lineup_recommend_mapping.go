// lineup_recommend_mapping.go 负责 NBA 球员与腾讯体育（TX）球员之间的 ID 映射，包括：
// - 推荐流程专用的三级映射构建（薪资表→手工兜底→腾讯阵容 API 回退）
// - 球员名称匹配（英文精确/中文精确/模糊匹配）
// - DVP（Defensive Value Per position）因子映射构建
package service

import (
	"context"
	"log"
	"strconv"
	"strings"

	"o2stock-crawler/internal/crawler"
	"o2stock-crawler/internal/entity"
)

// recommendTxMapSummary 汇总推荐流程中 NBA→TX 映射的来源统计。
type recommendTxMapSummary struct {
	SalaryCount         int // 通过薪资表匹配的数量
	ManualCount         int // 通过手工兜底列表匹配的数量
	LineupFallbackCount int // 通过腾讯阵容 API 回退匹配的数量
}

// buildRecommendTxPlayerIDMap 构建推荐流程专用的 NBAPlayerID → TxPlayerID 映射。
// 优先级：nba_player_salary 表 > 手工兜底列表 > 腾讯阵容 API 回退。
func (s *LineupRecommendService) buildRecommendTxPlayerIDMap(
	ctx context.Context,
	gamePlayers []entity.NBAGamePlayer,
	salaryTxMap map[uint]uint,
) (map[uint]uint, recommendTxMapSummary) {
	result := make(map[uint]uint, len(gamePlayers))
	summary := recommendTxMapSummary{}

	for _, player := range gamePlayers {
		txPlayerID := salaryTxMap[player.NBAPlayerID]
		if txPlayerID == 0 {
			continue
		}
		if _, exists := result[player.NBAPlayerID]; exists {
			continue
		}
		result[player.NBAPlayerID] = txPlayerID
		summary.SalaryCount++
	}

	missingSet := make(map[uint]struct{})
	for _, player := range gamePlayers {
		if player.NBAPlayerID == 0 {
			continue
		}
		if result[player.NBAPlayerID] > 0 {
			continue
		}
		missingSet[player.NBAPlayerID] = struct{}{}
	}
	summary.ManualCount = applyManualNBATxPlayerIDOverrides(result, missingSet)

	missingByTeam := make(map[string][]entity.NBAGamePlayer)
	for _, player := range gamePlayers {
		if player.NBAPlayerID == 0 || result[player.NBAPlayerID] > 0 || strings.TrimSpace(player.NBATeamID) == "" {
			continue
		}
		missingByTeam[player.NBATeamID] = append(missingByTeam[player.NBATeamID], player)
	}
	if len(missingByTeam) == 0 || s.txNBAClient == nil {
		return result, summary
	}

	for teamID, teamPlayers := range missingByTeam {
		resp, err := s.txNBAClient.GetTeamLineup(ctx, teamID)
		if err != nil {
			log.Printf("获取腾讯球队阵容失败: team_id=%s err=%v", teamID, err)
			continue
		}
		if resp == nil || len(resp.Data.LineUp.Players) == 0 {
			continue
		}

		usedTxIDs := make(map[uint]struct{})
		for _, player := range teamPlayers {
			txPlayerID, ok := matchNBAGamePlayerToTxLineupPlayer(player, resp.Data.LineUp.Players, usedTxIDs)
			if !ok {
				continue
			}
			result[player.NBAPlayerID] = txPlayerID
			usedTxIDs[txPlayerID] = struct{}{}
			summary.LineupFallbackCount++
		}
	}

	return result, summary
}

// matchNBAGamePlayerToTxLineupPlayer 将 NBA 球员与腾讯阵容中的球员匹配。
// 优先英文名精确匹配，其次中文名精确匹配，最后模糊匹配。
func matchNBAGamePlayerToTxLineupPlayer(
	player entity.NBAGamePlayer,
	lineupPlayers []struct {
		ID       string `json:"id"`
		CnName   string `json:"cnName"`
		EnName   string `json:"enName"`
		Logo     string `json:"logo"`
		Position string `json:"position"`
	},
	usedTxIDs map[uint]struct{},
) (uint, bool) {
	normalizedEn := normalizePlayerName(player.PlayerEnName)
	normalizedCN := normalizeLocalizedPlayerName(player.PlayerName)

	exactCandidates := make([]txLineupPlayer, 0)
	fuzzyCandidates := make([]txLineupPlayer, 0)
	for _, raw := range lineupPlayers {
		txPlayerID := parseUintOrZero(raw.ID)
		if txPlayerID == 0 {
			continue
		}
		if _, used := usedTxIDs[txPlayerID]; used {
			continue
		}

		lineupPlayer := txLineupPlayer{
			ID:     txPlayerID,
			CnName: raw.CnName,
			EnName: raw.EnName,
		}
		if normalizedEn != "" && normalizePlayerName(raw.EnName) == normalizedEn {
			exactCandidates = append(exactCandidates, lineupPlayer)
			continue
		}
		if normalizedCN != "" && normalizeLocalizedPlayerName(raw.CnName) == normalizedCN {
			exactCandidates = append(exactCandidates, lineupPlayer)
			continue
		}
		if normalizedEn != "" && crawler.MatchInjuryToPlayer(raw.EnName, player.PlayerEnName) {
			fuzzyCandidates = append(fuzzyCandidates, lineupPlayer)
		}
	}

	if len(exactCandidates) == 1 {
		return exactCandidates[0].ID, true
	}
	if len(fuzzyCandidates) == 1 {
		return fuzzyCandidates[0].ID, true
	}
	return 0, false
}

// normalizeLocalizedPlayerName 标准化中文版球员姓名（去除标点和空格）。
func normalizeLocalizedPlayerName(name string) string {
	replacer := strings.NewReplacer(
		".", "",
		"-", "",
		"·", "",
		" ", "",
	)
	return strings.ToLower(strings.TrimSpace(replacer.Replace(name)))
}

// parseUintOrZero 将字符串解析为 uint，解析失败返回 0。
func parseUintOrZero(value string) uint {
	n, err := strconv.ParseUint(strings.TrimSpace(value), 10, 64)
	if err != nil {
		return 0
	}
	return uint(n)
}

// buildDVPFactorMap 构建 DVP（Defensive Value Per position）因子映射。
// 统计每支球队面对各位置球员时的历史战力均值，与联盟均值对比得出 DVP 因子。
func (s *LineupRecommendService) buildDVPFactorMap(
	allPlayers []entity.NBAGamePlayer,
	txPlayerIDMap map[uint]uint,
	gameStatsMap map[uint][]entity.PlayerGameStats,
) map[string]map[uint]positionDVPMetric {
	opponentPositionPowers := make(map[string]map[uint][]float64)
	leaguePositionPowers := make(map[uint][]float64)

	for _, player := range allPlayers {
		txPlayerID := txPlayerIDMap[player.NBAPlayerID]
		if txPlayerID == 0 {
			continue
		}
		stats := gameStatsMap[txPlayerID]
		if len(stats) == 0 {
			continue
		}
		position := normalizePositionGroup(player.Position)
		for _, g := range stats {
			opponent := normalizeTeamCode(g.VsTeamName)
			if opponent == "" {
				continue
			}
			power := calcPowerFromStats(g)
			if power <= 0 {
				continue
			}
			if _, ok := opponentPositionPowers[opponent]; !ok {
				opponentPositionPowers[opponent] = make(map[uint][]float64)
			}
			opponentPositionPowers[opponent][position] = append(opponentPositionPowers[opponent][position], power)
			leaguePositionPowers[position] = append(leaguePositionPowers[position], power)
		}
	}

	leagueAvgByPosition := make(map[uint]float64)
	for position, powers := range leaguePositionPowers {
		if len(powers) == 0 {
			continue
		}
		total := 0.0
		for _, power := range powers {
			total += power
		}
		leagueAvgByPosition[position] = total / float64(len(powers))
	}

	result := make(map[string]map[uint]positionDVPMetric)
	for opponent, byPosition := range opponentPositionPowers {
		result[opponent] = make(map[uint]positionDVPMetric)
		for position, powers := range byPosition {
			metric := positionDVPMetric{
				Factor:      1.0,
				SampleCount: len(powers),
			}
			leagueAvg := leagueAvgByPosition[position]
			if len(powers) < dvpMetricMinSampleSize || leagueAvg <= 0 {
				result[opponent][position] = metric
				continue
			}
			total := 0.0
			for _, power := range powers {
				total += power
			}
			avgPower := total / float64(len(powers))
			metric.Factor = clamp(avgPower/leagueAvg, 0.92, 1.10)
			result[opponent][position] = metric
		}
	}
	return result
}

// getOpponentTeamCode 获取球员在同一场比赛中的对手球队代码。
func (s *LineupRecommendService) getOpponentTeamCode(player entity.NBAGamePlayer, allPlayers []entity.NBAGamePlayer) string {
	for _, p := range allPlayers {
		if p.MatchID == player.MatchID && p.NBATeamID != player.NBATeamID {
			return normalizeTeamCode(p.TeamName)
		}
	}
	return ""
}
