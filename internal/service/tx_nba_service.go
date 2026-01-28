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
func (s *TxNBAService) CrawlDailyStats(ctx context.Context, date string) error {
	log.Printf(">>> 开始从腾讯体育抓取比赛列表 (参考日期: %s) <<<", date)

	resp, err := s.client.GetMatchList(ctx, date)
	if err != nil {
		return fmt.Errorf("获取比赛列表失败: %w", err)
	}

	totalMatches := 0
	for matchDate, dayMatches := range resp.Data.Matches {
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
		var internalPlayerID uint = 0

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
			PlayerID:               internalPlayerID,
			TxPlayerID:             uint(txPlayerID),
			TxGameID:               mid,
			GameID:                 "", // 暂时用一样的
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
