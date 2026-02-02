package service

import (
	"context"
	"fmt"
	"log"
	"math/rand"
	"o2stock-crawler/internal/crawler"
	"o2stock-crawler/internal/db"
	"o2stock-crawler/internal/db/repositories"
	"o2stock-crawler/internal/entity"
	"strconv"
	"strings"
	"time"

	jsoniter "github.com/json-iterator/go"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	PlayerStatsType = "19" // 球员统计
	DataFromTxNBA   = 2    // 腾讯体育
	MatchTypeNBA    = "2"  // NBA 比赛
)

var teamNames = map[string]string{
	"老鹰":   "ATL",
	"篮网":   "BKN",
	"凯尔特人": "BOS",
	"黄蜂":   "CHA",
	"公牛":   "CHI",
	"骑士":   "CLE",
	"独行侠":  "DAL",
	"掘金":   "DEN",
	"活塞":   "DET",
	"勇士":   "GSW",
	"火箭":   "HOU",
	"步行者":  "IND",
	"快船":   "LAC",
	"湖人":   "LAL",
	"灰熊":   "MEM",
	"热火":   "MIA",
	"雄鹿":   "MIL",
	"森林狼":  "MIN",
	"鹈鹕":   "NOP",
	"尼克斯":  "NYK",
	"雷霆":   "OKC",
	"魔术":   "ORL",
	"76人":  "PHI",
	"太阳":   "PHX",
	"开拓者":  "POR",
	"国王":   "SAC",
	"马刺":   "SAS",
	"猛龙":   "TOR",
	"爵士":   "UTA",
	"奇才":   "WAS",
}

var teamIdToName = map[string]string{
	"1":  "老鹰",
	"2":  "凯尔特人",
	"3":  "鹈鹕",
	"4":  "公牛",
	"5":  "骑士",
	"6":  "独行侠",
	"7":  "掘金",
	"8":  "活塞",
	"9":  "勇士",
	"10": "火箭",
	"11": "步行者",
	"12": "快船",
	"13": "湖人",
	"14": "热火",
	"15": "雄鹿",
	"16": "森林狼",
	"17": "篮网",
	"18": "尼克斯",
	"19": "魔术",
	"20": "76人",
	"21": "太阳",
	"22": "开拓者",
	"23": "国王",
	"24": "马刺",
	"25": "雷霆",
	"26": "爵士",
	"27": "奇才",
	"28": "猛龙",
	"29": "灰熊",
	"30": "黄蜂",
}

func convertTeamName(name string) string {
	if abbr, ok := teamNames[name]; ok {
		return abbr
	}
	return name
}

type TxNBAService struct {
	db     *db.DB
	client *crawler.TxNBAClient
}

func NewTxNBAService(database *db.DB) *TxNBAService {
	return &TxNBAService{
		db:     database,
		client: crawler.NewTxNBAClient(),
	}
}

// CrawlDailyStats 抓取指定日期的所有 NBA 比赛统计（腾讯 API 会返回前后几天的列表）
func (s *TxNBAService) CrawlDailyStats(ctx context.Context, targetDate string, flag int) ([]uint, error) {
	log.Printf(">>> 开始从腾讯体育抓取比赛列表 (日期: %s, flag: %d) <<<", targetDate, flag)

	resp, err := s.client.GetMatchList(ctx, targetDate, flag)
	if err != nil {
		return nil, fmt.Errorf("获取比赛列表失败: %w", err)
	}

	totalMatches := 0
	txPlayerIDs := make(map[uint]struct{})

	for matchDate, dayMatches := range resp.Data.Matches {
		if matchDate != targetDate && flag == 2 {
			// flag == 2 时，只处理目标日期的数据
			continue
		}
		log.Printf("正在处理日期: %s, 比赛场数: %d", matchDate, len(dayMatches.List))
		for _, m := range dayMatches.List {
			if m.MatchInfo.MatchType != MatchTypeNBA {
				continue // 仅处理 NBA 正赛
			}

			log.Printf(" ==> 处理比赛: %s (%s vs %s)", m.MatchInfo.Mid, m.MatchInfo.LeftName, m.MatchInfo.RightName)

			// 避免抓取过快
			time.Sleep(time.Duration(rand.Intn(2)+1) * time.Second)

			pids, err := s.crawlMatchStat(ctx, m.MatchInfo.Mid, matchDate)
			if err != nil {
				log.Printf("抓取比赛 %s 详情失败: %v", m.MatchInfo.Mid, err)
				continue
			}
			for _, pid := range pids {
				txPlayerIDs[pid] = struct{}{}
			}
			totalMatches++
		}
	}

	resultPIDs := make([]uint, 0, len(txPlayerIDs))
	for pid := range txPlayerIDs {
		resultPIDs = append(resultPIDs, pid)
	}

	log.Printf(">>> 比赛数据抓取完成，共处理比赛: %d, 更新球员数: %d <<<", totalMatches, len(resultPIDs))
	return resultPIDs, nil
}

// SyncPlayerSeasonStats 同步球员赛季场均数据
func (s *TxNBAService) SyncPlayerSeasonStats(ctx context.Context, txPlayerIDs []uint) error {
	log.Printf(">>> 开始更新涉及球员的赛季场均数据 (球员 ids: %v) <<<", txPlayerIDs)

	for _, pid := range txPlayerIDs {
		playerName := ""
		// 获取球员姓名 (从数据库查一下)
		var player entity.Player
		if err := s.db.WithContext(ctx).Where("tx_player_id = ?", pid).First(&player).Error; err != nil {
			log.Printf("球员 ID %d 在数据库中未找到，跳过场均更新", pid)
			continue
		}
		playerName = player.EnName

		// 避免请求过快
		time.Sleep(time.Duration(rand.Intn(3)+2) * time.Second)

		log.Printf("开始获取球员 %s (%d) 赛季统计数据", playerName, pid)
		pidStr := strconv.Itoa(int(pid))
		resp, err := s.client.GetPlayerStats(ctx, pidStr)
		if err != nil {
			log.Printf("获取球员 %s (%d) 赛季统计失败: %v", playerName, pid, err)
			continue
		}

		// 找到基础数据 - 场均 tab
		var stats []struct {
			Key   string `json:"key"`
			Value string `json:"value"`
			Rank  int    `json:"rank"`
		}
		found := false

		for _, mod := range resp.Data.Modules {
			if mod.ID == "statList" {
				for _, tab := range mod.StatList.Tabs {
					if tab.Name == "场均" {
						stats = make([]struct {
							Key   string `json:"key"`
							Value string `json:"value"`
							Rank  int    `json:"rank"`
						}, len(tab.Stats))
						for i, st := range tab.Stats {
							stats[i] = struct {
								Key   string `json:"key"`
								Value string `json:"value"`
								Rank  int    `json:"rank"`
							}{Key: st.Key, Value: st.Value, Rank: st.Rank}
						}
						found = true
						break
					}
				}
			}
		}

		if !found {
			log.Printf("球员 %s 未找到场均统计数据", pidStr)
			continue
		}

		// 组装实体
		seasonStats := entity.PlayerSeasonStats{
			TxPlayerID:           pid,
			PlayerName:           playerName,
			Season:               "2025-26", // 固定为当前赛季
			SeasonType:           1,         // 常规赛
			GamesPlayed:          1,         // API 目前没有直接给场次，这里先固定 1
			Minutes:              s.findStatValue(stats, "minutesPG"),
			RankMin:              s.findStatRank(stats, "minutesPG"),
			Points:               s.findStatValue(stats, "pointsPG"),
			RankPts:              s.findStatRank(stats, "pointsPG"),
			Rebounds:             s.findStatValue(stats, "reboundsPG"),
			RankRb:               s.findStatRank(stats, "reboundsPG"),
			ReboundsOffensive:    s.findStatValue(stats, "offensiveReboundsPG"),
			RankRbo:              s.findStatRank(stats, "offensiveReboundsPG"),
			ReboundsDefensive:    s.findStatValue(stats, "defensiveReboundsPG"),
			RankRbd:              s.findStatRank(stats, "defensiveReboundsPG"),
			Assists:              s.findStatValue(stats, "assistsPG"),
			RankAst:              s.findStatRank(stats, "assistsPG"),
			Turnovers:            s.findStatValue(stats, "turnoversPG"),
			RankTov:              s.findStatRank(stats, "turnoversPG"),
			Steals:               s.findStatValue(stats, "stealsPG"),
			RankStl:              s.findStatRank(stats, "stealsPG"),
			Blocks:               s.findStatValue(stats, "blocksPG"),
			RankBlk:              s.findStatRank(stats, "blocksPG"),
			Fouls:                s.findStatValue(stats, "foulsPG"),
			RankPf:               s.findStatRank(stats, "foulsPG"),
			FieldGoalPercentage:  s.findStatValue(stats, "fgPCT"),
			RankPct2:             s.findStatRank(stats, "fgPCT"),
			ThreePointPercentage: s.findStatValue(stats, "threesPCT"),
			RankPct3:             s.findStatRank(stats, "threesPCT"),
			FreeThrowPercentage:  s.findStatValue(stats, "ftPCT"),
			RankPctFt:            s.findStatRank(stats, "ftPCT"),
		}

		// Upsert
		err = s.db.WithContext(ctx).Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "tx_player_id"}, {Name: "season"}, {Name: "season_type"}},
			UpdateAll: true,
		}).Create(&seasonStats).Error

		if err != nil {
			log.Printf("保存球员 %s 赛季统计失败: %v", playerName, err)
		} else {
			log.Printf("更新球员 %s 赛季统计成功", playerName)
		}
	}

	log.Printf(">>> 赛季场均数据更新完成 <<<")
	return nil
}

// SyncPlayerAge 补充球员年龄：从腾讯球员详情接口拉取并更新 players 表
func (s *TxNBAService) SyncPlayerAge(ctx context.Context, playerIDs []uint) error {
	playerRepo := repositories.NewPlayerRepository(s.db.DB)
	players, err := playerRepo.GetPlayersForAgeSync(ctx, playerIDs)
	if err != nil {
		return fmt.Errorf("查询待补充年龄的球员失败: %w", err)
	}

	log.Printf(">>> 开始补充球员年龄 (共 %d 人) <<<", len(players))

	updated := 0
	for _, p := range players {
		time.Sleep(time.Duration(rand.Intn(2)+1) * time.Second)

		pidStr := strconv.Itoa(int(p.TxPlayerID))
		log.Printf("获取球员 %s (%s) 年龄信息", p.ShowName, pidStr)

		resp, err := s.client.GetPlayerInfo(ctx, pidStr)
		if err != nil {
			log.Printf("获取球员 %s 详情失败: %v", p.ShowName, err)
			continue
		}

		if resp.Code != 0 {
			log.Printf("腾讯 API 返回错误: code=%d msg=%s", resp.Code, resp.Msg)
			continue
		}

		var age uint
		for _, item := range resp.Data.BaseInfo {
			if item.Name == "年龄" && item.Value != "" {
				val := strings.TrimSuffix(strings.TrimSpace(item.Value), "岁")
				a, err := strconv.Atoi(val)
				if err == nil && a > 0 && a <= 100 {
					age = uint(a)
				}
				break
			}
		}
		if age == 0 {
			log.Printf("球员 %s 未解析到年龄", p.ShowName)
			continue
		}

		if err := playerRepo.UpdateAge(ctx, p.PlayerID, age); err != nil {
			log.Printf("更新球员 %s 年龄失败: %v", p.ShowName, err)
			continue
		}
		updated++
		log.Printf(">> 更新球员 %s 年龄为 %d", p.ShowName, age)
	}

	log.Printf(">>> 球员年龄补充完成，成功更新 %d 人 <<<", updated)
	return nil
}

// SyncPlayers 同步腾讯球员 ID 和英文名
func (s *TxNBAService) SyncPlayers(ctx context.Context, teamID string) error {
	if teamID != "" {
		return s.syncTeamPlayers(ctx, teamID)
	}

	// 同步所有球队 (1-30)
	for id := 1; id <= 30; id++ {
		tid := strconv.Itoa(id)
		if err := s.syncTeamPlayers(ctx, tid); err != nil {
			log.Printf("同步球队 %s 球员信息失败: %v", tid, err)
		}
		// 避免请求过快
		time.Sleep(1 * time.Second)
	}
	return nil
}

func (s *TxNBAService) crawlMatchStat(ctx context.Context, mid string, dateStr string) ([]uint, error) {
	resp, err := s.client.GetMatchStat(ctx, mid)
	if err != nil {
		return nil, err
	}

	gameDate, _ := time.Parse("2006-01-02", dateStr)

	stats := []entity.PlayerGameStats{}
	txPlayerIDs := []uint{}

	// 查找统计数据块 (type=19)
	var playerStatsData struct {
		Left  crawler.TxPlayerStatsTeam `json:"left"`
		Right crawler.TxPlayerStatsTeam `json:"right"`
	}
	found := false
	for _, st := range resp.Data.Stats {
		if st.Type == PlayerStatsType {
			if err := jsoniter.Unmarshal(st.PlayerStats, &playerStatsData); err != nil {
				return nil, fmt.Errorf("解析球员统计数据详情失败: %w", err)
			}
			found = true
			break
		}
	}

	if !found {
		return nil, fmt.Errorf("未找到球员统计数据块 (type=19)")
	}

	// 处理客队 (Left)
	leftStats, leftPIDs := s.parseTeamStats(ctx, playerStatsData.Left, mid, gameDate, resp.Data.TeamInfo.LeftName, resp.Data.TeamInfo.RightName, false)
	stats = append(stats, leftStats...)
	txPlayerIDs = append(txPlayerIDs, leftPIDs...)

	// 处理主队 (Right)
	rightStats, rightPIDs := s.parseTeamStats(ctx, playerStatsData.Right, mid, gameDate, resp.Data.TeamInfo.RightName, resp.Data.TeamInfo.LeftName, true)
	stats = append(stats, rightStats...)
	txPlayerIDs = append(txPlayerIDs, rightPIDs...)

	if len(stats) == 0 {
		return txPlayerIDs, nil
	}

	// 批量入库
	err = s.db.Transaction(func(tx *gorm.DB) error {
		return tx.Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "tx_player_id"}, {Name: "tx_game_id"}},
			UpdateAll: true,
		}).Create(&stats).Error
	})
	return txPlayerIDs, err
}

func (s *TxNBAService) parseTeamStats(_ context.Context, teamData crawler.TxPlayerStatsTeam, mid string, date time.Time, teamName, oppName string, isHome bool) ([]entity.PlayerGameStats, []uint) {
	result := []entity.PlayerGameStats{}
	txPlayerIDs := []uint{}
	for _, p := range teamData.Oncrt {
		if len(p.Row) < 14 {
			continue
		}

		txPlayerID, _ := strconv.Atoi(p.PlayerID)
		if txPlayerID == 0 {
			continue
		}

		fgMade, fgAtt := s.parseShot(p.Row[11])
		tpMade, tpAtt := s.parseShot(p.Row[12])
		ftMade, ftAtt := s.parseShot(p.Row[13])

		pts, _ := strconv.Atoi(p.Row[3])
		reb, _ := strconv.Atoi(p.Row[4])
		ast, _ := strconv.Atoi(p.Row[5])
		stl, _ := strconv.Atoi(p.Row[6])
		blk, _ := strconv.Atoi(p.Row[7])
		to, _ := strconv.Atoi(p.Row[8])

		stat := entity.PlayerGameStats{
			TxPlayerID:             uint(txPlayerID),
			TxGameID:               mid,
			GameDate:               date,
			PlayerTeamName:         convertTeamName(teamName),
			VsTeamName:             convertTeamName(oppName),
			IsHome:                 isHome,
			Points:                 pts,
			Rebounds:               reb,
			Assists:                ast,
			Steals:                 stl,
			Blocks:                 blk,
			Turnovers:              to,
			Minutes:                s.parseMinutes(p.Row[2]),
			FieldGoalsMade:         fgMade,
			FieldGoalsAttempted:    fgAtt,
			ThreePointersMade:      tpMade,
			ThreePointersAttempted: tpAtt,
			FreeThrowsMade:         ftMade,
			FreeThrowsAttempted:    ftAtt,
			DataFrom:               DataFromTxNBA,
		}
		result = append(result, stat)
		txPlayerIDs = append(txPlayerIDs, uint(txPlayerID))
	}
	return result, txPlayerIDs
}

func (s *TxNBAService) parseMinutes(m string) int {
	parts := strings.Split(m, "'")
	if len(parts) > 0 {
		min, _ := strconv.Atoi(parts[0])
		return min
	}
	return 0
}

func (s *TxNBAService) parseShot(shot string) (made, attempted int) {
	parts := strings.Split(shot, "/")
	if len(parts) == 2 {
		made, _ = strconv.Atoi(parts[0])
		attempted, _ = strconv.Atoi(parts[1])
	}
	return
}

func (s *TxNBAService) syncTeamPlayers(ctx context.Context, teamID string) error {
	teamName, ok := teamIdToName[teamID]
	if !ok {
		return fmt.Errorf("未知球队 ID: %s", teamID)
	}

	log.Printf(">>> 开始同步球队阵容: %s (%s) <<<", teamID, teamName)

	resp, err := s.client.GetTeamLineup(ctx, teamID)
	if err != nil {
		return err
	}

	updatedCount := 0
	for _, p := range resp.Data.LineUp.Players {
		// 解析出 players.cnName，将 - 替换为 .
		cnNameReplace := strings.ReplaceAll(p.CnName, "-", ".")

		txPlayerID, _ := strconv.Atoi(p.ID)
		if txPlayerID == 0 {
			continue
		}

		// 查询 players 表中 p_name_show = $cnNameReplace and team_abbr = $teamName 的数据
		var player entity.Player
		err := s.db.WithContext(ctx).Where("p_name_show = ? AND team_abbr = ?", cnNameReplace, teamName).First(&player).Error
		if err != nil {
			if err == gorm.ErrRecordNotFound {
				// 没有对应数据的不处理
				continue
			}
			return err
		}

		// 找到对应数据并修改数据： players.tx_player_id = players.id，players.p_name_en = players.enName
		player.TxPlayerID = uint(txPlayerID)
		player.EnName = p.EnName

		if err := s.db.WithContext(ctx).Save(&player).Error; err != nil {
			log.Printf("更新球员 %s (%d) 失败: %v", player.ShowName, player.ID, err)
			continue
		}
		updatedCount++
	}

	log.Printf("球队 %s 同步完成，更新完成数: %d", teamName, updatedCount)
	return nil
}

func (s *TxNBAService) findStatValue(stats []struct {
	Key   string `json:"key"`
	Value string `json:"value"`
	Rank  int    `json:"rank"`
}, key string) float64 {
	for _, st := range stats {
		if st.Key == key {
			val := strings.TrimSuffix(st.Value, "%")
			val = strings.ReplaceAll(val, ",", "")
			f, _ := strconv.ParseFloat(val, 64)
			if strings.HasSuffix(st.Value, "%") {
				return f / 100.0
			}
			return f
		}
	}
	return 0
}

func (s *TxNBAService) findStatRank(stats []struct {
	Key   string `json:"key"`
	Value string `json:"value"`
	Rank  int    `json:"rank"`
}, key string) uint {
	for _, st := range stats {
		if st.Key == key {
			if st.Rank < 0 {
				return 0
			}
			return uint(st.Rank)
		}
	}
	return 0
}
