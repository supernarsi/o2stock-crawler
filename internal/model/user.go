package model

import "time"

// User 用户模型
type User struct {
	ID           uint      `json:"id"`
	Nick         string    `json:"nick"`
	Avatar       string    `json:"avatar"`
	WxOpenID     string    `json:"wx_openid"`
	WxUnionID    string    `json:"wx_unionid"`
	WxSessionKey string    `json:"wx_session_key"`
	Sta          uint8     `json:"sta"` // 状态：1.正常；2.封禁
	CTime        time.Time `json:"c_time"`
}
