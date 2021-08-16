package helper

import (
	"math"
	"strconv"
	"time"

	gonanoid "github.com/matoous/go-nanoid/v2"
	t "github.com/tonkla/autotp/types"
)

// Now13 returns a millisecond Unix timestamp
func Now13() int64 {
	return time.Now().UnixNano() / 1e6
}

// GenID returns a string of a millisecond Unix timestamp
func GenID() string {
	return strconv.FormatInt(Now13(), 10)
}

// RandomStr returns a random string, generated by NanoID
func RandomStr(size int) (string, error) {
	if size == 0 {
		size = 13
	}
	return gonanoid.Generate("0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz", size)
}

// CalcSLStop calculates a stop price of the stop loss price
func CalcSLStop(side string, sl float64, gap float64, digits int64) float64 {
	if gap == 0 {
		gap = 500
	}
	pow := math.Pow(10, float64(digits))
	if side == t.OrderSideBuy {
		return round((sl*pow+gap)/pow, pow)
	}
	return round((sl*pow-gap)/pow, pow)
}

// CalcTPStop calculates a stop price of the take profit price
func CalcTPStop(side string, tp float64, gap float64, digits int64) float64 {
	if gap == 0 {
		gap = 500
	}
	pow := math.Pow(10, float64(digits))
	if side == t.OrderSideBuy {
		return round((tp*pow-gap)/pow, pow)
	}
	return round((tp*pow+gap)/pow, pow)
}

// Reverse returns the opposite side
func Reverse(side string) string {
	if side == t.OrderSideBuy {
		return t.OrderSideSell
	}
	return t.OrderSideBuy
}

// NormalizeDouble rounds a floating-point number to a specified accuracy
func NormalizeDouble(number float64, digits int64) float64 {
	pow := math.Pow(10, float64(digits))
	return math.Round(number*pow) / pow
}

func round(number float64, pow float64) float64 {
	return math.Round(number*pow) / pow
}
