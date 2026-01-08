package middleware

import (
	"encoding/json"
	"net/http"
	"o2stock-crawler/api"
)

type APIError struct {
	Status int
	Code   int
	Msg    string
}

func API(handler func(r *http.Request) (interface{}, *APIError)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		data, err := handler(r)
		if err != nil {
			w.WriteHeader(err.Status)
			writeJSON(w, api.Error(err.Code, err.Msg))
			return
		}
		writeJSON(w, api.Success(data))
	}
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	_ = enc.Encode(v)
}
