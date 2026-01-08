package api

import "o2stock-crawler/internal/model"

type PlayersRes struct {
	Players []*model.Players `json:"players"`
}

type PlayerHistoryRes struct {
	PlayerHistory []*model.PriceHistoryRow `json:"history"`
}
