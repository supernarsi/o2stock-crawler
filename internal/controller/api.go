package controller

import (
	"net/http"
	"strconv"

	"o2stock-crawler/api"
	"o2stock-crawler/internal/db"
	"o2stock-crawler/internal/middleware"
)

type API struct {
	db *db.DB
}

func NewAPI(database *db.DB) *API {
	return &API{db: database}
}

func (a *API) Players() http.HandlerFunc {
	return middleware.Compose(middleware.API(func(r *http.Request) (interface{}, *middleware.APIError) {
		ctx := r.Context()
		limit := parseIntDefault(r.URL.Query().Get("limit"), 100)
		offset := parseIntDefault(r.URL.Query().Get("offset"), 0)
		rows, err := db.ListPlayers(ctx, a.db, limit, offset)
		if err != nil {
			return nil, &middleware.APIError{Status: http.StatusInternalServerError, Code: http.StatusInternalServerError, Msg: err.Error()}
		}
		return api.PlayersRes{Players: rows}, nil
	}), middleware.CORS, middleware.Logging)
}

func (a *API) PlayerHistory() http.HandlerFunc {
	return middleware.Compose(middleware.API(func(r *http.Request) (interface{}, *middleware.APIError) {
		ctx := r.Context()
		playerIDStr := r.URL.Query().Get("player_id")
		if playerIDStr == "" {
			return nil, &middleware.APIError{Status: http.StatusBadRequest, Code: http.StatusBadRequest, Msg: "missing player_id"}
		}
		id64, err := strconv.ParseUint(playerIDStr, 10, 32)
		if err != nil {
			return nil, &middleware.APIError{Status: http.StatusBadRequest, Code: http.StatusBadRequest, Msg: "invalid player_id"}
		}
		limit := parseIntDefault(r.URL.Query().Get("limit"), 200)
		rows, err := db.GetPlayerHistory(ctx, a.db, uint32(id64), limit)
		if err != nil {
			return nil, &middleware.APIError{Status: http.StatusInternalServerError, Code: http.StatusInternalServerError, Msg: err.Error()}
		}
		return api.PlayerHistoryRes{PlayerHistory: rows}, nil
	}), middleware.CORS, middleware.Logging)
}

func (a *API) Healthz() http.HandlerFunc {
	return middleware.Compose(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}, middleware.CORS, middleware.Logging)
}

func parseIntDefault(s string, def int) int {
	if s == "" {
		return def
	}
	v, err := strconv.Atoi(s)
	if err != nil {
		return def
	}
	return v
}
