package consts

import "time"

const (
	DefaultLimit          int           = 100
	MaxLimit              int           = 500
	TimestampToleranceSec int           = 300
	MaxUint8              uint8         = 255
	NonceExpiration       time.Duration = 10 * time.Minute
)
