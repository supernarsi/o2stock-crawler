package middleware

import (
	"io"
	"net/http"

	jsoniter "github.com/json-iterator/go"
)

// DecodeJSONBody 统一解析 JSON body
func DecodeJSONBody(r *http.Request, v any) error {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		return err
	}
	defer r.Body.Close()

	if len(body) == 0 {
		return nil // 空 body 不报错
	}

	return jsoniter.Unmarshal(body, v)
}
