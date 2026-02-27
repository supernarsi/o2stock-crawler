package api

// DebugSendPlayerBreakEvenNotifyReq 内部调试：给指定用户推送指定球员回本订阅消息
type DebugSendPlayerBreakEvenNotifyReq struct {
	UID      uint `json:"uid"`
	PlayerID uint `json:"player_id"`
}
