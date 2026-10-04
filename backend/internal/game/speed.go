package game

import "math"

// SpeedRate 对齐 legacy server/game/common.php:29 的 GAME_SPEED_RATE（私有服 10 倍速）。
// 配置表里的 *_time 均为基准秒数，落库前需按倍速换算。
const SpeedRate = 10

// ScaledSeconds 把配置秒数按倍速与系数换算为真实秒数。
// 向上取整并保证至少 1 秒，避免 0 秒即时完成。
func ScaledSeconds(raw int64, factor float64) int64 {
	if raw <= 0 {
		return 0
	}
	s := math.Ceil(float64(raw) * factor / float64(SpeedRate))
	if s < 1 {
		s = 1
	}
	return int64(s)
}