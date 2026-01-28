package service

import (
	"context"
	"fmt"
	"log"
	"math/rand"
	"o2stock-crawler/internal/crawler"
	"o2stock-crawler/internal/db"
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
func (s *TxNBAService) CrawlDailyStats(ctx context.Context, targetDate string, flag int) error {
	log.Printf(">>> 开始从腾讯体育抓取比赛列表 (日期: %s, flag: %d) <<<", targetDate, flag)

	resp, err := s.client.GetMatchList(ctx, targetDate, flag)
	if err != nil {
		return fmt.Errorf("获取比赛列表失败: %w", err)
	}

	totalMatches := 0
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

			if err := s.crawlMatchStat(ctx, m.MatchInfo.Mid, matchDate); err != nil {
				log.Printf("抓取比赛 %s 详情失败: %v", m.MatchInfo.Mid, err)
				continue
			}
			totalMatches++
		}
	}

	log.Printf(">>> 比赛数据抓取完成，共处理比赛: %d <<<", totalMatches)
	return nil
}

func (s *TxNBAService) crawlMatchStat(ctx context.Context, mid string, dateStr string) error {
	resp, err := s.client.GetMatchStat(ctx, mid)
	if err != nil {
		return err
	}

	gameDate, _ := time.Parse("2006-01-02", dateStr)

	stats := []entity.PlayerGameStats{}

	// 查找统计数据块 (type=19)
	var playerStatsData struct {
		Left  crawler.TxPlayerStatsTeam `json:"left"`
		Right crawler.TxPlayerStatsTeam `json:"right"`
	}
	found := false
	for _, st := range resp.Data.Stats {
		if st.Type == PlayerStatsType {
			if err := jsoniter.Unmarshal(st.PlayerStats, &playerStatsData); err != nil {
				return fmt.Errorf("解析球员统计数据详情失败: %w", err)
			}
			found = true
			break
		}
	}

	if !found {
		return fmt.Errorf("未找到球员统计数据块 (type=19)")
	}

	// 处理客队 (Left)
	stats = append(stats, s.parseTeamStats(ctx, playerStatsData.Left, mid, gameDate, resp.Data.TeamInfo.LeftName, resp.Data.TeamInfo.RightName, false)...)
	// 处理主队 (Right)
	stats = append(stats, s.parseTeamStats(ctx, playerStatsData.Right, mid, gameDate, resp.Data.TeamInfo.RightName, resp.Data.TeamInfo.LeftName, true)...)

	if len(stats) == 0 {
		return nil
	}

	// 批量入库
	return s.db.Transaction(func(tx *gorm.DB) error {
		return tx.Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "tx_player_id"}, {Name: "tx_game_id"}},
			UpdateAll: true,
		}).Create(&stats).Error
	})
}

func (s *TxNBAService) parseTeamStats(_ context.Context, teamData crawler.TxPlayerStatsTeam, mid string, date time.Time, teamName, oppName string, isHome bool) []entity.PlayerGameStats {
	result := []entity.PlayerGameStats{}
	for _, p := range teamData.Oncrt {
		if len(p.Row) < 14 {
			continue
		}

		txPlayerID, _ := strconv.Atoi(p.PlayerID)
		if txPlayerID == 0 {
			continue
		}
		// playerName := p.Row[1]

		// 尝试映射到内部 player_id
		// var internalPlayerID uint = 0

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
	}
	return result
}

// findInternalPlayerID 尝试通过腾讯 ID 或姓名匹配内部球员 ID
func (s *TxNBAService) findInternalPlayerID(ctx context.Context, txPlayerID uint, playerName string) uint {
	// 1. 尝试按姓名匹配 (模糊匹配或包含关系)
	var player entity.Player
	// 腾讯的名字通常是简写，比如 "米勒"
	// 我们的名字是 "布兰登-米勒"
	// 尝试用 LIKE %name%
	err := s.db.WithContext(ctx).Where("p_name_show LIKE ?", "%"+playerName+"%").First(&player).Error
	if err == nil {
		return player.NBAPlayerID // 返回 NBA Player ID 作为内部 PlayerID
	}

	return 0 // 未找到则返回 0
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
