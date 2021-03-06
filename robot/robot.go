package robot

import (
	"github.com/tonkla/autotp/app"
	h "github.com/tonkla/autotp/helper"
	t "github.com/tonkla/autotp/types"
)

func Trade(ap *app.AppParams) {
	if ap.BP.OrderType == t.OrderTypeLimit {
		placeAsMaker(ap)
	} else if ap.BP.OrderType == t.OrderTypeMarket {
		placeAsTaker(ap)
	}
}

func placeAsMaker(p *app.AppParams) {
	if p.BP.Product == t.ProductSpot {
		syncLimitOrder(p)
		syncTPOrder(p)
	} else if p.BP.Product == t.ProductFutures {
		syncLimitLongOrder(p)
		syncLimitShortOrder(p)
		syncSLLongOrder(p)
		syncSLShortOrder(p)
		syncTPLongOrder(p)
		syncTPShortOrder(p)
	}
	cancelOrders(p)
	closeOrders(p)
	openLimitOrders(p)
}

func placeAsTaker(p *app.AppParams) {
	openMarketOrders(p)
}

func cancelOrders(p *app.AppParams) {
	for _, o := range p.TO.CancelOrders {
		exo, err := p.EX.GetOrder(o)
		if err != nil || exo == nil {
			h.Log(err)
			continue
		}
		if exo.Status != t.OrderStatusNew {
			o.Status = exo.Status
			o.UpdateTime = exo.UpdateTime
			if exo.Status != t.OrderStatusFilled {
				o.CloseTime = h.Now13()
			}
			err = p.DB.UpdateOrder(o)
			if err != nil {
				h.Log(err)
			}
			continue
		}

		exo, err = p.EX.CancelOrder(o)
		if err != nil || exo == nil {
			h.Log(err)
			continue
		}

		o.Status = exo.Status
		o.UpdateTime = exo.UpdateTime
		o.CloseTime = h.Now13()
		err = p.DB.UpdateOrder(o)
		if err != nil {
			h.Log(err)
			continue
		}

		if o.PosSide != "" {
			h.LogCanceledF(o)
		} else {
			h.LogCanceled(o)
		}
	}
}

func closeOrders(p *app.AppParams) {
	for _, o := range p.TO.CloseOrders {
		exo, err := p.EX.OpenStopOrder(o)
		if err != nil || exo == nil {
			h.Log(err)
			continue
		}

		o.RefID = exo.RefID
		o.OpenTime = exo.OpenTime
		err = p.DB.CreateOrder(o)
		if err != nil {
			h.Log(err)
			continue
		}

		if o.PosSide != "" {
			h.LogNewF(o)
		} else {
			h.LogNew(o)
		}
	}
}

func openLimitOrders(p *app.AppParams) {
	for _, o := range p.TO.OpenOrders {
		exo, err := p.EX.OpenLimitOrder(o)
		if err != nil || exo == nil {
			h.Log(err)
			continue
		}

		o.RefID = exo.RefID
		o.Status = exo.Status
		o.OpenTime = exo.OpenTime
		err = p.DB.CreateOrder(o)
		if err != nil {
			h.Log(err)
			continue
		}

		if o.PosSide != "" {
			h.LogNewF(o)
		} else {
			h.LogNew(o)
		}
	}
}

func openMarketOrders(p *app.AppParams) {
	for _, o := range p.TO.OpenOrders {
		_qty := h.NormalizeDouble(p.BP.QuoteQty/p.TK.Price, p.BP.QtyDigits)
		if _qty > o.Qty {
			o.Qty = _qty
		}
		o.Type = t.OrderTypeMarket
		exo, err := p.EX.OpenMarketOrder(o)
		if err != nil || exo == nil {
			h.Log(err)
			continue
		}

		o.RefID = exo.RefID
		o.Status = exo.Status
		o.OpenTime = exo.OpenTime
		o.OpenPrice = exo.OpenPrice
		o.Qty = exo.Qty
		o.Commission = exo.Commission
		err = p.DB.CreateOrder(o)
		if err != nil {
			h.Log(err)
			continue
		}

		if o.PosSide != "" {
			h.LogFilledF(o)
		} else {
			h.LogFilled(o)
		}
	}
}

func syncLimitOrder(p *app.AppParams) {
	o := p.DB.GetHighestNewBuyOrder(p.QO)
	if o == nil {
		return
	}

	syncStatus(*o, p)
}

func syncTPOrder(p *app.AppParams) {
	tpo := p.DB.GetLowestTPOrder(p.QO)
	if tpo == nil {
		return
	}

	isTraded := syncStatus(*tpo, p)
	if isTraded {
		syncTPLong(*tpo, p)
	}
}

func syncLimitLongOrder(p *app.AppParams) {
	o := p.DB.GetHighestNewLongOrder(p.QO)
	if o == nil {
		return
	}

	syncStatus(*o, p)
}

func syncLimitShortOrder(p *app.AppParams) {
	o := p.DB.GetLowestNewShortOrder(p.QO)
	if o == nil {
		return
	}

	syncStatus(*o, p)
}

func syncSLLongOrder(p *app.AppParams) {
	slo := p.DB.GetHighestSLLongOrder(p.QO)
	if slo == nil {
		return
	}

	isTraded := syncStatus(*slo, p)
	if isTraded {
		syncSLLong(*slo, p)
	}
}

func syncSLShortOrder(p *app.AppParams) {
	slo := p.DB.GetLowestSLShortOrder(p.QO)
	if slo == nil {
		return
	}

	isTraded := syncStatus(*slo, p)
	if isTraded {
		syncSLShort(*slo, p)
	}
}

func syncTPLongOrder(p *app.AppParams) {
	tpo := p.DB.GetLowestTPLongOrder(p.QO)
	if tpo == nil {
		return
	}

	isTraded := syncStatus(*tpo, p)
	if isTraded {
		syncTPLong(*tpo, p)
	}
}

func syncTPShortOrder(p *app.AppParams) {
	tpo := p.DB.GetHighestTPShortOrder(p.QO)
	if tpo == nil {
		return
	}

	isTraded := syncStatus(*tpo, p)
	if isTraded {
		syncTPShort(*tpo, p)
	}
}

func syncStatus(o t.Order, p *app.AppParams) bool {
	exo, err := p.EX.GetOrder(o)
	if err != nil || exo == nil {
		h.Log(err)
		return false
	}

	if exo.Status == t.OrderStatusNew {
		if p.BP.TimeSecCancel > 0 && (h.Now13()-o.OpenTime)/1000 > p.BP.TimeSecCancel {
			exo, err = p.EX.CancelOrder(o)
			if err != nil || exo == nil {
				h.Log(err)
				return false
			}

			o.Status = exo.Status
			o.UpdateTime = exo.UpdateTime
			o.CloseTime = h.Now13()
			err = p.DB.UpdateOrder(o)
			if err != nil {
				h.Log(err)
				return false
			}

			if o.PosSide != "" {
				h.LogCanceledF(o)
			} else {
				h.LogCanceled(o)
			}
		}
		return false
	}

	if o.Status != exo.Status {
		o.Status = exo.Status
		o.UpdateTime = exo.UpdateTime

		canceledStatuses := []string{
			t.OrderStatusCanceled,
			t.OrderStatusExpired,
			t.OrderStatusRejected,
		}
		if h.ContainsString(canceledStatuses, exo.Status) {
			o.CloseTime = h.Now13()
		}

		if exo.Status == t.OrderStatusFilled {
			commission := p.EX.GetCommission(p.BP.Symbol, o.RefID)
			if commission != nil {
				o.Commission = *commission
			}
		}

		err := p.DB.UpdateOrder(o)
		if err != nil {
			h.Log(err)
			return false
		}

		if exo.Status == t.OrderStatusFilled {
			if o.PosSide != "" {
				h.LogFilledF(o)
			} else {
				h.LogFilled(o)
			}
		}

		if h.ContainsString(canceledStatuses, exo.Status) {
			if o.PosSide != "" {
				h.LogCanceledF(o)
			} else {
				h.LogCanceled(o)
			}
		}
	}

	orders, _ := p.EX.GetTradeList(o.Symbol, 5, 0, 0)
	for _, to := range orders {
		if o.RefID == to.RefID && o.CloseTime == 0 && o.Status == t.OrderStatusFilled {
			return true
		}
	}
	return false
}

func syncSLLong(slo t.Order, p *app.AppParams) {
	o := p.DB.GetOrderByID(slo.OpenOrderID)
	if o == nil {
		slo.CloseTime = h.Now13()
		err := p.DB.UpdateOrder(slo)
		if err != nil {
			h.Log(err)
		}
		return
	}

	o.CloseOrderID = slo.ID
	o.ClosePrice = slo.OpenPrice
	o.CloseTime = h.Now13()
	o.PL = h.NormalizeDouble(((o.ClosePrice-o.OpenPrice)*slo.Qty)-o.Commission-slo.Commission, p.BP.PriceDigits)
	err := p.DB.UpdateOrder(*o)
	if err != nil {
		h.Log(err)
		return
	}

	slo.CloseTime = o.CloseTime
	err = p.DB.UpdateOrder(slo)
	if err != nil {
		h.Log(err)
		return
	}

	h.LogClosedF(*o, slo)
}

func syncSLShort(slo t.Order, p *app.AppParams) {
	o := p.DB.GetOrderByID(slo.OpenOrderID)
	if o == nil {
		slo.CloseTime = h.Now13()
		err := p.DB.UpdateOrder(slo)
		if err != nil {
			h.Log(err)
		}
		return
	}

	o.CloseOrderID = slo.ID
	o.ClosePrice = slo.OpenPrice
	o.CloseTime = h.Now13()
	o.PL = h.NormalizeDouble(((o.OpenPrice-o.ClosePrice)*slo.Qty)-o.Commission-slo.Commission, p.BP.PriceDigits)
	err := p.DB.UpdateOrder(*o)
	if err != nil {
		h.Log(err)
		return
	}

	slo.CloseTime = o.CloseTime
	err = p.DB.UpdateOrder(slo)
	if err != nil {
		h.Log(err)
		return
	}

	h.LogClosedF(*o, slo)
}

func syncTPLong(tpo t.Order, p *app.AppParams) {
	o := p.DB.GetOrderByID(tpo.OpenOrderID)
	if o == nil {
		tpo.CloseTime = h.Now13()
		err := p.DB.UpdateOrder(tpo)
		if err != nil {
			h.Log(err)
		}
		return
	}

	o.CloseOrderID = tpo.ID
	o.ClosePrice = tpo.OpenPrice
	o.CloseTime = h.Now13()
	o.PL = h.NormalizeDouble(((o.ClosePrice-o.OpenPrice)*tpo.Qty)-o.Commission-tpo.Commission, p.BP.PriceDigits)
	err := p.DB.UpdateOrder(*o)
	if err != nil {
		h.Log(err)
		return
	}

	tpo.CloseTime = o.CloseTime
	err = p.DB.UpdateOrder(tpo)
	if err != nil {
		h.Log(err)
		return
	}

	if o.PosSide != "" {
		h.LogClosedF(*o, tpo)
	} else {
		h.LogClosed(*o, tpo)
	}
}

func syncTPShort(tpo t.Order, p *app.AppParams) {
	o := p.DB.GetOrderByID(tpo.OpenOrderID)
	if o == nil {
		tpo.CloseTime = h.Now13()
		err := p.DB.UpdateOrder(tpo)
		if err != nil {
			h.Log(err)
		}
		return
	}

	o.CloseOrderID = tpo.ID
	o.ClosePrice = tpo.OpenPrice
	o.CloseTime = h.Now13()
	o.PL = h.NormalizeDouble(((o.OpenPrice-o.ClosePrice)*tpo.Qty)-o.Commission-tpo.Commission, p.BP.PriceDigits)
	err := p.DB.UpdateOrder(*o)
	if err != nil {
		h.Log(err)
		return
	}

	tpo.CloseTime = o.CloseTime
	err = p.DB.UpdateOrder(tpo)
	if err != nil {
		h.Log(err)
		return
	}

	if o.PosSide != "" {
		h.LogClosedF(*o, tpo)
	} else {
		h.LogClosed(*o, tpo)
	}
}
