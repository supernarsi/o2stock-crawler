package consts

// 订阅类型：用于 u_p_own.notify_type 及接口
const (
	NotifyTypeNone      uint8 = 0 // 不订阅
	NotifyTypeBreakEven uint8 = 1 // 回本通知
	NotifyTypeProfit15  uint8 = 2 // 盈利 15% 通知
)
