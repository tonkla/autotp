package daily

import (
	"github.com/tonkla/autotp/exchange"
	h "github.com/tonkla/autotp/helper"
	"github.com/tonkla/autotp/rdb"
	"github.com/tonkla/autotp/strategy/common"
	"github.com/tonkla/autotp/talib"
	t "github.com/tonkla/autotp/types"
)

type Strategy struct {
	DB *rdb.DB
	BP *t.BotParams
	EX exchange.Repository
}

func New(db *rdb.DB, bp *t.BotParams, ex exchange.Repository) Strategy {
	return Strategy{
		DB: db,
		BP: bp,
		EX: ex,
	}
}

func (s Strategy) OnTick(ticker t.Ticker) *t.TradeOrders {
	var openOrders, closeOrders, cancelOrders []t.Order

	qo := t.QueryOrder{
		BotID:    s.BP.BotID,
		Exchange: s.BP.Exchange,
		Symbol:   s.BP.Symbol,
	}

	if s.BP.CloseLong || s.BP.CloseShort {
		if s.BP.CloseLong {
			closeOrders = append(closeOrders, common.CloseLong(s.DB, s.BP, qo, ticker)...)
		}
		if s.BP.CloseShort {
			closeOrders = append(closeOrders, common.CloseShort(s.DB, s.BP, qo, ticker)...)
		}
		return &t.TradeOrders{
			CloseOrders: closeOrders,
		}
	}

	const numberOfBars = 50
	prices := s.EX.GetHistoricalPrices(s.BP.Symbol, s.BP.MATf1st, numberOfBars)

	if len(prices) < numberOfBars || prices[len(prices)-1].Open == 0 || prices[len(prices)-2].Open == 0 {
		return nil
	}

	highs, lows, closes := common.GetHighsLowsCloses(prices)

	cma := talib.WMA(closes, int(s.BP.MAPeriod1st))
	cma_0 := cma[len(cma)-1]
	cma_1 := cma[len(cma)-2]

	hma := talib.WMA(highs, int(s.BP.MAPeriod1st))
	hma_0 := hma[len(hma)-1]

	lma := talib.WMA(lows, int(s.BP.MAPeriod1st))
	lma_0 := lma[len(lma)-1]

	atr := hma_0 - lma_0

	qo.Qty = h.NormalizeDouble(s.BP.BaseQty, s.BP.QtyDigits)
	qty := h.NormalizeDouble(s.BP.QuoteQty/ticker.Price, s.BP.QtyDigits)
	if qty > qo.Qty {
		qo.Qty = qty
	}

	if s.BP.AutoSL {
		closeOrders = append(closeOrders, common.SLLong(s.DB, s.BP, qo, ticker, atr)...)
		closeOrders = append(closeOrders, common.SLShort(s.DB, s.BP, qo, ticker, atr)...)
	}

	if s.BP.AutoTP {
		closeOrders = append(closeOrders, common.TPLong(s.DB, s.BP, qo, ticker, atr)...)
		closeOrders = append(closeOrders, common.TPShort(s.DB, s.BP, qo, ticker, atr)...)
	}

	openLimit := float64(s.BP.SLim.OpenLimit)
	isFutures := s.BP.Product == t.ProductFutures

	p_0 := prices[len(prices)-1]
	t_0 := p_0.Time
	h_0 := p_0.High
	l_0 := p_0.Low

	p_1 := prices[len(prices)-2]
	o_1 := p_1.Open
	c_1 := p_1.Close
	h_1 := p_1.High
	l_1 := p_1.Low

	p_2 := prices[len(prices)-3]
	h_2 := p_2.High
	l_2 := p_2.Low

	hh := h_0
	if h_1 > hh {
		hh = h_1
	}
	if h_2 > hh {
		hh = h_2
	}
	ll := l_0
	if l_1 < ll {
		ll = l_1
	}
	if l_2 < ll {
		ll = l_2
	}
	shouldCloseLong := ticker.Price < hh-atr*0.5
	shouldCloseShort := ticker.Price > ll+atr*0.5

	if shouldCloseLong {
		cancelOrders = append(cancelOrders, s.DB.GetNewLimitLongOrders(qo)...)
		closeOrders = append(closeOrders, common.CloseLong(s.DB, s.BP, qo, ticker)...)
	}

	if shouldCloseShort {
		cancelOrders = append(cancelOrders, s.DB.GetNewLimitShortOrders(qo)...)
		closeOrders = append(closeOrders, common.CloseShort(s.DB, s.BP, qo, ticker)...)
	}

	isUp := cma_1 < cma_0 && o_1 < c_1 && h_1-c_1 < c_1-l_1
	isDown := cma_1 > cma_0 && o_1 > c_1 && h_1-c_1 > c_1-l_1

	shouldOpenLong := isUp && ticker.Price < c_1 && atr > 0 && ticker.Price < hma_0+atr*0.5
	shouldOpenShort := isDown && ticker.Price > c_1 && atr > 0 && ticker.Price > lma_0-atr*0.5

	if shouldOpenLong && !shouldCloseLong && (s.BP.View == t.ViewNeutral || s.BP.View == t.ViewLong) {
		openPrice := h.CalcStopLowerTicker(ticker.Price, openLimit, s.BP.PriceDigits)
		qo.OpenPrice = openPrice
		qo.Side = t.OrderSideBuy
		norder := s.DB.GetNearestOrder(qo)
		if norder == nil {
			o := t.Order{
				ID:        h.GenID(),
				BotID:     s.BP.BotID,
				Exchange:  qo.Exchange,
				Symbol:    qo.Symbol,
				Side:      t.OrderSideBuy,
				Type:      t.OrderTypeLimit,
				Status:    t.OrderStatusNew,
				Qty:       qo.Qty,
				OpenPrice: openPrice,
			}
			if isFutures {
				o.PosSide = t.OrderPosSideLong
			}
			openOrders = append(openOrders, o)
		} else if norder.Status == t.OrderStatusNew && norder.OpenTime < t_0 {
			cancelOrders = append(cancelOrders, *norder)
		}
	}

	if shouldOpenShort && !shouldCloseShort && (s.BP.View == t.ViewNeutral || s.BP.View == t.ViewShort) {
		openPrice := h.CalcStopUpperTicker(ticker.Price, openLimit, s.BP.PriceDigits)
		qo.OpenPrice = openPrice
		qo.Side = t.OrderSideSell
		norder := s.DB.GetNearestOrder(qo)
		if norder == nil {
			o := t.Order{
				ID:        h.GenID(),
				BotID:     s.BP.BotID,
				Exchange:  qo.Exchange,
				Symbol:    qo.Symbol,
				Side:      t.OrderSideSell,
				Type:      t.OrderTypeLimit,
				Status:    t.OrderStatusNew,
				Qty:       qo.Qty,
				OpenPrice: openPrice,
			}
			if isFutures {
				o.PosSide = t.OrderPosSideShort
			}
			openOrders = append(openOrders, o)
		} else if norder.Status == t.OrderStatusNew && norder.OpenTime < t_0 {
			cancelOrders = append(cancelOrders, *norder)
		}
	}

	return &t.TradeOrders{
		OpenOrders:   openOrders,
		CloseOrders:  closeOrders,
		CancelOrders: cancelOrders,
	}
}
