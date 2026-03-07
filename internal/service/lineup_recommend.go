package service

import (
	"context"

	"o2stock-crawler/internal/crawler"
	"o2stock-crawler/internal/db"
	"o2stock-crawler/internal/entity"
)

// LineupRecommendService 推荐引擎核心服务。
//
// 设计约定：该服务按“导入数据/生成推荐/赛后回测”三个阶段组织，
// 具体实现已按职责拆分到同目录多个文件，便于维护与测试。
type LineupRecommendService struct {
	db           *db.DB
	injuryClient *crawler.InjuryClient
	txNBAClient  txTeamLineupClient
}

const (
	defaultSalaryCap = 150
	defaultPickCount = 5
	defaultTopN      = 3
)

// NewLineupRecommendService 创建推荐引擎服务。
func NewLineupRecommendService(database *db.DB) *LineupRecommendService {
	return &LineupRecommendService{
		db:           database,
		injuryClient: crawler.NewInjuryClient(),
		txNBAClient:  crawler.NewTxNBAClient(),
	}
}

type txTeamLineupClient interface {
	GetTeamLineup(ctx context.Context, teamID string) (*crawler.TxTeamLineupResponse, error)
}

// MatchData 比赛数据 JSON 结构。
type MatchData struct {
	IMatchId    string `json:"iMatchId"`
	IHomeTeamId string `json:"iHomeTeamId"`
	IAwayTeamId string `json:"iAwayTeamId"`
	DtDate      string `json:"dtDate"`
	DtTime      string `json:"dtTime"`
}

// PlayerSalary 球员工资 JSON 结构。
type PlayerSalary struct {
	ID            string `json:"id"`
	IPlayerId     string `json:"iPlayerId"`
	ITeamId       string `json:"iTeamId"`
	SPlayerName   string `json:"sPlayerName"`
	SPlayerEnName string `json:"sPlayerEnName"`
	IPosition     string `json:"iPosition"`
	FCombatPower  string `json:"fCombatPower"`
	ISalary       string `json:"iSalary"`
}

// ActualFeedbackItem 真实战力反馈项（仅支持阵容 list 格式）。
type ActualFeedbackItem struct {
	Rank        uint     `json:"-"`
	NBAPlayerID uint     `json:"nba_player_id"`
	Salary      *uint    `json:"salary"`
	ActualPower *float64 `json:"actual_power"`
	Source      string   `json:"source"`
}

// ActualFeedbackLineupPayload 阵容反馈 JSON 结构（list -> players）。
type ActualFeedbackLineupPayload struct {
	GameDate string                 `json:"game_date"`
	Source   string                 `json:"source"`
	List     []ActualFeedbackLineup `json:"list"`
}

type ActualFeedbackLineup struct {
	Rank    uint                 `json:"rank"`
	Players []ActualFeedbackItem `json:"players"`
}

// PlayerPrediction 预测结果及各因子明细。
type PlayerPrediction struct {
	PredictedPower        float64
	OptimizedPower        float64
	BaseValue             float64
	AvailabilityScore     float64
	StatusTrend           float64
	MatchupFactor         float64
	DefRatingFactor       float64
	PaceFactor            float64
	DvPFactor             float64
	HistoryFactor         float64
	OpponentFormFactor    float64
	RimDeterrenceFactor   float64
	DefenseAnchorFactor   float64
	HomeAwayFactor        float64
	TeamContextFactor     float64
	MinutesFactor         float64
	UsageFactor           float64
	StabilityFactor       float64
	DefenseUpsideFactor   float64
	ArchetypeFactor       float64
	RoleSecurityFactor    float64
	DataReliabilityFactor float64
	TeamExposureFactor    float64
	FatigueFactor         float64
	GameRiskFactor        float64
}

// PlayerCandidate 候选球员（含预测值）。
type PlayerCandidate struct {
	Player             entity.NBAGamePlayer
	Prediction         PlayerPrediction
	BacktestTxPlayerID uint
	BacktestName       string
}

// DetailPlayer detail_json 中的球员信息。
type DetailPlayer struct {
	NBAPlayerID    uint    `json:"nba_player_id"`
	Name           string  `json:"name"`
	Team           string  `json:"team"`
	Salary         uint    `json:"salary"`
	CombatPower    float64 `json:"combat_power"`
	PredictedPower float64 `json:"predicted_power"`
	OptimizedPower float64 `json:"optimized_power,omitempty"`
	Factors        struct {
		BaseValue             float64 `json:"base_value"`
		AvailabilityScore     float64 `json:"availability_score"`
		StatusTrend           float64 `json:"status_trend"`
		MatchupFactor         float64 `json:"matchup_factor"`
		DefRatingFactor       float64 `json:"def_rating_factor,omitempty"`
		PaceFactor            float64 `json:"pace_factor,omitempty"`
		DvPFactor             float64 `json:"dvp_factor,omitempty"`
		HistoryFactor         float64 `json:"history_factor,omitempty"`
		OpponentFormFactor    float64 `json:"opponent_form_factor,omitempty"`
		RimDeterrenceFactor   float64 `json:"rim_deterrence_factor,omitempty"`
		DefenseAnchorFactor   float64 `json:"defense_anchor_factor,omitempty"`
		HomeAwayFactor        float64 `json:"home_away_factor"`
		TeamContextFactor     float64 `json:"team_context_factor"`
		MinutesFactor         float64 `json:"minutes_factor"`
		UsageFactor           float64 `json:"usage_factor"`
		StabilityFactor       float64 `json:"stability_factor"`
		DefenseUpsideFactor   float64 `json:"defense_upside_factor,omitempty"`
		ArchetypeFactor       float64 `json:"archetype_factor,omitempty"`
		RoleSecurityFactor    float64 `json:"role_security_factor,omitempty"`
		DataReliabilityFactor float64 `json:"data_reliability_factor,omitempty"`
		TeamExposureFactor    float64 `json:"team_exposure_factor,omitempty"`
		FatigueFactor         float64 `json:"fatigue_factor"`
		GameRiskFactor        float64 `json:"game_risk_factor"`
		DbPowerPer5           float64 `json:"db_power_per5,omitempty"`
		DbPowerPer10          float64 `json:"db_power_per10,omitempty"`
	} `json:"factors"`
}

// DetailJSON detail_json 结构。
type DetailJSON struct {
	Players []DetailPlayer `json:"players"`
}
