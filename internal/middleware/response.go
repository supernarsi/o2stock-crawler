package middleware

import (
	"net/http"
	"o2stock-crawler/api"

	jsoniter "github.com/json-iterator/go"
)

var json = jsoniter.ConfigCompatibleWithStandardLibrary

type APIError struct {
	Status int
	Code   int
	Msg    string
}

func API(handler func(r *http.Request) (any, *APIError)) http.HandlerFunc {
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
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	_, _ = w.Write(data)
}
