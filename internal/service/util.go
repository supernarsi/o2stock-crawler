package service

// calcPower 计算球员战力值
func calcPower(points, rebounds, assists, steals, blocks, turnovers float64) float64 {
	return points + 1.2*rebounds + 1.5*assists + 3.0*steals + 3.0*blocks - turnovers
}
