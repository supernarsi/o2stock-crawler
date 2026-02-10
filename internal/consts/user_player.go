package consts

// 持仓状态：用于 u_p_own / u_i_own.own_sta
const (
	OwnStaNone      = 0 // 未拥有
	OwnStaPurchased = 1 // 已购买（持有中）
	OwnStaSold      = 2 // 已售出
)

// 持仓类型：用于 u_p_own.own_goods
const (
	OwnGoodsPlayer = 1 // 球员
	OwnGoodsItem   = 2 // 道具
)
