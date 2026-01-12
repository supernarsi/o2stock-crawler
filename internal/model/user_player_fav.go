package model

import "time"

// UserPlayerFav 表示 u_p_fav 表的一行
type UserPlayerFav struct {
	ID       uint      `json:"id"`
	UserID   uint      `json:"uid"`
	PlayerID uint      `json:"pid"`
	CTime    time.Time `json:"c_time"`
}
