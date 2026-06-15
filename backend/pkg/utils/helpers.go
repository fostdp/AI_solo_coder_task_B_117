package utils

import (
	"math"
	"time"
)

func RoundFloat(val float64, precision int) float64 {
	ratio := math.Pow(10, float64(precision))
	return math.Round(val*ratio) / ratio
}

func ClampFloat(val, min, max float64) float64 {
	if val < min {
		return min
	}
	if val > max {
		return max
	}
	return val
}

func TimeBucket(t time.Time, interval time.Duration) time.Time {
	return t.Truncate(interval)
}

func FormatTime(t time.Time) string {
	return t.Format("2006-01-02 15:04:05")
}
