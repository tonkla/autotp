package spot

import (
	"errors"
	"fmt"
	"strings"

	"github.com/tidwall/gjson"

	b "github.com/tonkla/autotp/exchange/binance"
	h "github.com/tonkla/autotp/helper"
	t "github.com/tonkla/autotp/types"
)

type Client struct {
	baseURL   string
	apiKey    string
	secretKey string
}

// NewSpotClient returns Binance Spot client
func NewSpotClient(apiKey string, secretKey string) Client {
	return Client{
		baseURL:   "https://api.binance.com/api/v3",
		apiKey:    apiKey,
		secretKey: secretKey,
	}
}

// Public APIs -----------------------------------------------------------------

// GetTicker returns the latest ticker
func (c Client) GetTicker(symbol string) *t.Ticker {
	return b.GetTicker(c.baseURL, symbol)
}

// GetOrderBook returns an order book (market depth
func (c Client) GetOrderBook(symbol string, limit int) *t.OrderBook {
	return b.GetOrderBook(c.baseURL, symbol, limit)
}

// GetHistoricalPrices returns historical prices in a format of k-lines/candlesticks
func (c Client) GetHistoricalPrices(symbol string, timeframe string, limit int) []t.HistoricalPrice {
	return b.GetHistoricalPrices(c.baseURL, symbol, timeframe, limit)
}

// Get1wHistoricalPrices returns '1w' historical prices in a format of k-lines/candlesticks
func (c Client) Get1wHistoricalPrices(symbol string, limit int) []t.HistoricalPrice {
	return c.GetHistoricalPrices(symbol, "1w", limit)
}

// Get1dHistoricalPrices returns '1d' historical prices in a format of k-lines/candlesticks
func (c Client) Get1dHistoricalPrices(symbol string, limit int) []t.HistoricalPrice {
	return c.GetHistoricalPrices(symbol, "1d", limit)
}

// Get4hHistoricalPrices returns '4h' historical prices in a format of k-lines/candlesticks
func (c Client) Get4hHistoricalPrices(symbol string, limit int) []t.HistoricalPrice {
	return c.GetHistoricalPrices(symbol, "4h", limit)
}

// Get1hHistoricalPrices returns '1h' historical prices in a format of k-lines/candlesticks
func (c Client) Get1hHistoricalPrices(symbol string, limit int) []t.HistoricalPrice {
	return c.GetHistoricalPrices(symbol, "1h", limit)
}

// Get15mHistoricalPrices returns '15m' historical prices in a format of k-lines/candlesticks
func (c Client) Get15mHistoricalPrices(symbol string, limit int) []t.HistoricalPrice {
	return c.GetHistoricalPrices(symbol, "15m", limit)
}

// Private APIs ----------------------------------------------------------------

// CountOpenOrders returns a number of open orders
func (c Client) CountOpenOrders(symbol string) (int, error) {
	var payload, url strings.Builder

	b.BuildBaseQS(&payload, symbol)

	signature := b.Sign(payload.String(), c.secretKey)

	fmt.Fprintf(&url, "%s/openOrders?%s&signature=%s", c.baseURL, payload.String(), signature)
	data, err := h.GetH(url.String(), b.NewHeader(c.apiKey))
	if err != nil {
		return 0, err
	}

	rs := gjson.ParseBytes(data)

	if rs.Get("code").Int() < 0 {
		h.Log("CountOpenOrders", rs)
		return 0, errors.New("error")
	}

	return len(rs.Array()), nil
}

// GetOpenOrders returns open orders
func (c Client) GetOpenOrders(symbol string) []t.Order {
	var payload, url strings.Builder

	b.BuildBaseQS(&payload, symbol)

	signature := b.Sign(payload.String(), c.secretKey)

	fmt.Fprintf(&url, "%s/openOrders?%s&signature=%s", c.baseURL, payload.String(), signature)
	data, err := h.GetH(url.String(), b.NewHeader(c.apiKey))
	if err != nil {
		return nil
	}

	rs := gjson.ParseBytes(data)

	if rs.Get("code").Int() < 0 {
		h.Log("GetOpenOrders", rs)
		return nil
	}

	var orders []t.Order
	for _, r := range rs.Array() {
		order := t.Order{
			Symbol:     symbol,
			ID:         r.Get("clientOrderId").String(),
			RefID:      r.Get("orderId").String(),
			Side:       r.Get("side").String(),
			Status:     r.Get("status").String(),
			Type:       r.Get("type").String(),
			Qty:        r.Get("origQty").Float(),
			OpenPrice:  r.Get("price").Float(),
			OpenTime:   r.Get("time").Int(),
			UpdateTime: r.Get("updateTime").Int(),
		}
		orders = append(orders, order)
	}
	return orders
}

// GetTradeList returns trades list for a specified symbol
func (c Client) GetTradeList(symbol string, limit int, startTime int, endTime int) ([]t.TradeOrder, error) {
	var payload, url strings.Builder

	b.BuildBaseQS(&payload, symbol)

	if limit > 0 {
		fmt.Fprintf(&payload, "&limit=%d", limit)
	} else {
		fmt.Fprintf(&payload, "&limit=10")
	}
	if startTime > 0 {
		fmt.Fprintf(&payload, "&startTime=%d", startTime)
	}
	if endTime > 0 {
		fmt.Fprintf(&payload, "&endTime=%d", endTime)
	}

	signature := b.Sign(payload.String(), c.secretKey)

	fmt.Fprintf(&url, "%s/myTrades?%s&signature=%s", c.baseURL, payload.String(), signature)
	data, err := h.GetH(url.String(), b.NewHeader(c.apiKey))
	if err != nil {
		return nil, err
	}

	rs := gjson.ParseBytes(data)

	if rs.Get("code").Int() < 0 {
		h.Log("GetTradeOrders", rs)
		return nil, errors.New(rs.Get("msg").String())
	}

	var orders []t.TradeOrder
	for _, r := range rs.Array() {
		order := t.TradeOrder{
			Symbol:          r.Get("symbol").String(),
			RefID:           r.Get("orderId").String(),
			Price:           r.Get("price").Float(),
			Qty:             r.Get("qty").Float(),
			QuoteQty:        r.Get("quoteQty").Float(),
			Commission:      r.Get("commission").Float(),
			CommissionAsset: r.Get("commissionAsset").String(),
			Time:            r.Get("time").Int(),
			IsBuyer:         r.Get("isBuyer").Bool(),
			IsMaker:         r.Get("isMaker").Bool(),
		}
		orders = append(orders, order)
	}
	return orders, nil
}

// GetAllOrders returns all account orders; active, canceled, or filled
func (c Client) GetAllOrders(symbol string, limit int, startTime int, endTime int) []t.Order {
	var payload, url strings.Builder

	b.BuildBaseQS(&payload, symbol)

	if limit > 0 {
		fmt.Fprintf(&payload, "&limit=%d", limit)
	} else {
		fmt.Fprintf(&payload, "&limit=10")
	}
	if startTime > 0 {
		fmt.Fprintf(&payload, "&startTime=%d", startTime)
	}
	if endTime > 0 {
		fmt.Fprintf(&payload, "&endTime=%d", endTime)
	}

	signature := b.Sign(payload.String(), c.secretKey)

	fmt.Fprintf(&url, "%s/allOrders?%s&signature=%s", c.baseURL, payload.String(), signature)
	data, err := h.GetH(url.String(), b.NewHeader(c.apiKey))
	if err != nil {
		return nil
	}

	rs := gjson.ParseBytes(data)

	if rs.Get("code").Int() < 0 {
		h.Log("GetAllOrders", rs)
		return nil
	}

	var orders []t.Order
	for _, r := range rs.Array() {
		order := t.Order{
			Symbol:     symbol,
			ID:         r.Get("clientOrderId").String(),
			RefID:      r.Get("orderId").String(),
			Side:       r.Get("side").String(),
			Status:     r.Get("status").String(),
			Type:       r.Get("type").String(),
			Qty:        r.Get("origQty").Float(),
			OpenPrice:  r.Get("price").Float(),
			OpenTime:   r.Get("time").Int(),
			UpdateTime: r.Get("updateTime").Int(),
		}
		orders = append(orders, order)
	}
	return orders
}

// GetCommission returns order commission
func (c Client) GetCommission(symbol string, orderRefID string) *float64 {
	var payload, url strings.Builder

	b.BuildBaseQS(&payload, symbol)

	fmt.Fprintf(&payload, "&limit=10")

	signature := b.Sign(payload.String(), c.secretKey)

	fmt.Fprintf(&url, "%s/myTrades?%s&signature=%s", c.baseURL, payload.String(), signature)
	data, err := h.GetH(url.String(), b.NewHeader(c.apiKey))
	if err != nil {
		return nil
	}

	rs := gjson.ParseBytes(data)

	if rs.Get("code").Int() < 0 {
		h.Log("GetCommission", rs)
		return nil
	}

	for _, r := range rs.Array() {
		orderID := r.Get("orderId").String()
		if orderRefID == orderID {
			commission := r.Get("commission").Float()
			return &commission
		}
	}
	return nil
}

// OpenLimitOrder opens a limit order on the Binance Spot
func (c Client) OpenLimitOrder(o t.Order) (*t.Order, error) {
	if o.Type != t.OrderTypeLimit {
		return nil, nil
	}

	var payload, url strings.Builder

	b.BuildBaseQS(&payload, o.Symbol)
	fmt.Fprintf(&payload, "&newClientOrderId=%s&side=%s&type=%s&quantity=%f&price=%f&timeInForce=GTC",
		o.ID, o.Side, o.Type, o.Qty, o.OpenPrice)

	signature := b.Sign(payload.String(), c.secretKey)

	fmt.Fprintf(&url, "%s/order?%s&signature=%s", c.baseURL, payload.String(), signature)
	data, err := h.Post(url.String(), b.NewHeader(c.apiKey))
	if err != nil {
		return nil, err
	}

	r := gjson.ParseBytes(data)

	if r.Get("code").Int() < 0 {
		return nil, fmt.Errorf("OpenLimitOrder: %s", r.Get("msg").String())
	}

	status := r.Get("status").String()
	if !(status == t.OrderStatusNew || status == t.OrderStatusFilled) {
		return nil, nil
	}
	o.Status = status
	o.RefID = r.Get("orderId").String()
	o.OpenTime = r.Get("transactTime").Int()
	return &o, nil
}

// OpenStopOrder opens a stop order on the Binance Spot
func (c Client) OpenStopOrder(o t.Order) (*t.Order, error) {
	if o.Type != t.OrderTypeSL && o.Type != t.OrderTypeTP {
		return nil, nil
	}

	var payload, url strings.Builder

	b.BuildBaseQS(&payload, o.Symbol)
	fmt.Fprintf(&payload,
		"&newClientOrderId=%s&side=%s&type=%s&quantity=%f&price=%f&stopPrice=%f&timeInForce=GTC",
		o.ID, o.Side, o.Type, o.Qty, o.OpenPrice, o.StopPrice)

	signature := b.Sign(payload.String(), c.secretKey)

	fmt.Fprintf(&url, "%s/order?%s&signature=%s", c.baseURL, payload.String(), signature)
	data, err := h.Post(url.String(), b.NewHeader(c.apiKey))
	if err != nil {
		return nil, err
	}

	r := gjson.ParseBytes(data)

	if r.Get("code").Int() < 0 {
		return nil, fmt.Errorf("OpenStopOrder: %s", r.Get("msg").String())
	}

	o.RefID = r.Get("orderId").String()
	o.OpenTime = r.Get("transactTime").Int()
	return &o, nil
}

// OpenMarketOrder opens a market order on the Binance Spot
func (c Client) OpenMarketOrder(o t.Order) (*t.Order, error) {
	if o.Type != t.OrderTypeMarket {
		return nil, nil
	}

	var payload, url strings.Builder

	b.BuildBaseQS(&payload, o.Symbol)
	fmt.Fprintf(&payload, "&newClientOrderId=%s&side=%s&type=%s&quantity=%f",
		o.ID, o.Side, o.Type, o.Qty)

	signature := b.Sign(payload.String(), c.secretKey)

	fmt.Fprintf(&url, "%s/order?%s&signature=%s", c.baseURL, payload.String(), signature)
	data, err := h.Post(url.String(), b.NewHeader(c.apiKey))
	if err != nil {
		return nil, err
	}

	r := gjson.ParseBytes(data)

	if r.Get("code").Int() < 0 {
		return nil, fmt.Errorf("OpenMarketOrder: %s", r.Get("msg").String())
	}

	o.RefID = r.Get("orderId").String()
	o.OpenTime = r.Get("transactTime").Int()
	o.Status = r.Get("status").String()

	fills := r.Get("fills").Array()
	if len(fills) > 0 {
		o.OpenPrice = fills[0].Get("price").Float()
		o.Qty = fills[0].Get("qty").Float()
		o.Commission = fills[0].Get("commission").Float()
	}

	return &o, nil
}

// CancelOrder cancels an order on the Binance Spot
func (c Client) CancelOrder(o t.Order) (*t.Order, error) {
	var payload, url strings.Builder

	b.BuildBaseQS(&payload, o.Symbol)
	fmt.Fprintf(&payload, "&orderId=%s&origClientOrderId=%s", o.RefID, o.ID)

	signature := b.Sign(payload.String(), c.secretKey)

	fmt.Fprintf(&url, "%s/order?%s&signature=%s", c.baseURL, payload.String(), signature)
	data, err := h.Delete(url.String(), b.NewHeader(c.apiKey))
	if err != nil {
		return nil, err
	}

	r := gjson.ParseBytes(data)

	if r.Get("code").Int() < 0 {
		return nil, fmt.Errorf("CancelOrder: %s", r.Get("msg").String())
	}

	status := r.Get("status").String()
	if status != t.OrderStatusCanceled {
		return nil, nil
	}
	o.Status = status
	o.UpdateTime = h.Now13()
	return &o, nil
}

// GetOrder returns the order by its IDs
func (c Client) GetOrder(o t.Order) (*t.Order, error) {
	cc := b.Client{
		BaseURL:   c.baseURL,
		ApiKey:    c.apiKey,
		SecretKey: c.secretKey,
	}
	return b.GetOrder(cc, o)
}

func (c Client) CloseOrder(o t.Order) (*t.Order, error) {
	return nil, nil
}
