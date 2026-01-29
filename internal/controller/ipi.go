package controller

import (
	"net/http"
	"strconv"

	"o2stock-crawler/api"
	"o2stock-crawler/internal/db/repositories"
	"o2stock-crawler/internal/entity"
	"o2stock-crawler/internal/middleware"
)

// IPIRank 获取 IPI 排行：最新一批，按 ipi 降序分页，可选仅税后安全边际；每条含 p_name_show
func (a *API) IPIRank() http.HandlerFunc {
	return middleware.API(func(r *http.Request) (any, *middleware.APIError) {
		ctx := r.Context()
		page := parseIntDefault(r.URL.Query().Get("page"), 1)
		limit := parseIntDefault(r.URL.Query().Get("limit"), 20)
		taxSafeOnly := r.URL.Query().Get("tax_safe_only") == "true"
		if limit <= 0 {
			limit = 20
		}
		if limit > 100 {
			limit = 100
		}

		calculatedAt, ok, err := a.ipiRepo.GetLatestCalculatedAt(ctx)
		if err != nil {
			return nil, &middleware.APIError{Status: http.StatusInternalServerError, Code: http.StatusInternalServerError, Msg: err.Error()}
		}
		if !ok {
			return api.IPIRankRes{
				List:         []api.IPIRankItem{},
				CalculatedAt: calculatedAt,
				Page:         page,
				Limit:        limit,
				Total:        0,
			}, nil
		}

		list, total, err := a.ipiRepo.ListLatest(ctx, repositories.ListLatestFilter{
			Page:         page,
			Limit:        limit,
			TaxSafeOnly:  taxSafeOnly,
			CalculatedAt: calculatedAt,
		})
		if err != nil {
			return nil, &middleware.APIError{Status: http.StatusInternalServerError, Code: http.StatusInternalServerError, Msg: err.Error()}
		}

		items := make([]api.IPIRankItem, len(list))
		if len(list) > 0 {
			playerIDs := make([]uint, len(list))
			for i := range list {
				playerIDs[i] = list[i].PlayerID
			}
			players, _ := a.playerRepo.BatchGetByIDs(ctx, playerIDs)
			nameMap := make(map[uint]entity.Player)
			for _, p := range players {
				nameMap[p.PlayerID] = p
			}
			for i := range list {
				items[i] = ipiRowToRankItem(list[i], nameMap[list[i].PlayerID].ShowName, nameMap[list[i].PlayerID].PriceStandard)
			}
		}

		return api.IPIRankRes{
			List:         items,
			CalculatedAt: calculatedAt,
			Page:         page,
			Limit:        limit,
			Total:        total,
		}, nil
	})
}

// IPIPlayer 获取单球员最新一批 IPI，含 p_name_show
func (a *API) IPIPlayer() http.HandlerFunc {
	return middleware.API(func(r *http.Request) (any, *middleware.APIError) {
		ctx := r.Context()
		playerIDStr := r.URL.Query().Get("player_id")
		if playerIDStr == "" {
			return nil, &middleware.APIError{Status: http.StatusBadRequest, Code: http.StatusBadRequest, Msg: "missing player_id"}
		}
		playerID, err := strconv.ParseUint(playerIDStr, 10, 32)
		if err != nil {
			return nil, &middleware.APIError{Status: http.StatusBadRequest, Code: http.StatusBadRequest, Msg: "invalid player_id"}
		}
		pid := uint(playerID)

		row, err := a.ipiRepo.GetByPlayerIDLatest(ctx, pid)
		if err != nil {
			return nil, &middleware.APIError{Status: http.StatusInternalServerError, Code: http.StatusInternalServerError, Msg: err.Error()}
		}
		if row == nil {
			return nil, &middleware.APIError{Status: http.StatusNotFound, Code: http.StatusNotFound, Msg: "player ipi not found"}
		}

		pNameShow := ""
		if p, _ := a.playerRepo.GetByID(ctx, pid); p != nil {
			pNameShow = p.ShowName
		}
		res := api.IPIPlayerRes{
			PlayerID:           row.PlayerID,
			PNameShow:          pNameShow,
			IPI:                row.IPI,
			SPerf:              row.SPerf,
			VGap:               row.VGap,
			MGrowth:            row.MGrowth,
			RRisk:              row.RRisk,
			MeetsTaxSafeMargin: row.MeetsTaxSafeMargin,
			RankInversionIndex: row.RankInversionIndex,
			CalculatedAt:       row.CalculatedAt,
		}
		return res, nil
	})
}

func ipiRowToRankItem(row entity.PlayerIPI, pNameShow string, priceStandard uint) api.IPIRankItem {
	return api.IPIRankItem{
		PlayerID:      row.PlayerID,
		PNameShow:     pNameShow,
		IPI:           row.IPI,
		PriceStandard: priceStandard,
		// SPerf:              row.SPerf,
		// VGap:               row.VGap,
		// MGrowth:            row.MGrowth,
		// RRisk:              row.RRisk,
		// MeetsTaxSafeMargin: row.MeetsTaxSafeMargin,
		// RankInversionIndex: row.RankInversionIndex,
		// CalculatedAt:       row.CalculatedAt,
	}
}
