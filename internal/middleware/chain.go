package middleware

import "net/http"

type Middleware func(http.HandlerFunc) http.HandlerFunc

func Compose(h http.HandlerFunc, mws ...Middleware) http.HandlerFunc {
	for i := len(mws) - 1; i >= 0; i-- {
		h = mws[i](h)
	}
	return h
}
