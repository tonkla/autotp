package main

import (
	"fmt"
	"os"
	"path"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	rds "github.com/tonkla/autotp/db"
	"github.com/tonkla/autotp/exchange/binance"
	h "github.com/tonkla/autotp/helper"
	"github.com/tonkla/autotp/strategy/grid"
	t "github.com/tonkla/autotp/types"
)

var rootCmd = &cobra.Command{
	Use:   "autotp",
	Short: "AutoTP: Auto Take Profit (Grid)",
	Long:  "AutoTP: Auto Trading Platform (Grid)",
	Run:   func(cmd *cobra.Command, args []string) {},
}

var (
	configFile string
)

type params struct {
	db          *rds.DB
	ticker      *t.Ticker
	tradeOrders *t.TradeOrders
	exchange    *binance.Client
	queryOrder  *t.Order
	symbol      string
	priceDigits int64
	qtyDigits   int64
	quoteQty    float64
}

func init() {
	rootCmd.Flags().StringVarP(&configFile, "configFile", "c", "", "Configuration File (required)")
	rootCmd.MarkFlagRequired("configFile")
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(0)
	} else if _, err := os.Stat(configFile); os.IsNotExist(err) {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(0)
	} else if ext := path.Ext(configFile); ext != ".yml" && ext != ".yaml" {
		fmt.Fprintln(os.Stderr, "Accept only YAML file")
		os.Exit(0)
	}

	viper.SetConfigFile(configFile)
	err := viper.ReadInConfig()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(0)
	}

	apiKey := viper.GetString("apiKey")
	secretKey := viper.GetString("secretKey")
	dbName := viper.GetString("dbName")
	symbol := viper.GetString("symbol")
	botID := viper.GetInt64("botID")
	priceDigits := viper.GetInt64("priceDigits")
	qtyDigits := viper.GetInt64("qtyDigits")
	upperPrice := viper.GetFloat64("upperPrice")
	lowerPrice := viper.GetFloat64("lowerPrice")
	startPrice := viper.GetFloat64("startPrice")
	gridSize := viper.GetFloat64("gridSize")
	gridTP := viper.GetFloat64("gridTP")
	openZones := viper.GetInt64("openZones")
	baseQty := viper.GetFloat64("baseQty")
	quoteQty := viper.GetFloat64("quoteQty")
	intervalSec := viper.GetInt64("intervalSec")
	autoTP := viper.GetBool("autoTP")
	orderType := viper.GetString("orderType")

	if upperPrice <= lowerPrice {
		fmt.Fprintln(os.Stderr, "The upper price must be greater than the lower price")
		os.Exit(0)
	} else if gridSize < 2 {
		fmt.Fprintln(os.Stderr, "Grid size must be greater than 1")
		os.Exit(0)
	} else if baseQty == 0 && quoteQty == 0 {
		fmt.Fprintln(os.Stderr, "Quantity per grid must be greater than 0")
		os.Exit(0)
	}

	db := rds.Connect(dbName)

	exchange := binance.NewSpotClient(apiKey, secretKey)

	bp := t.BotParams{
		BotID:      botID,
		UpperPrice: upperPrice,
		LowerPrice: lowerPrice,
		GridSize:   gridSize,
		GridTP:     gridTP,
		OpenZones:  openZones,
		Qty:        baseQty,
		View:       "LONG",
		AutoTP:     autoTP,
	}

	queryOrder := t.Order{
		BotID:    botID,
		Exchange: t.ExcBinance,
		Symbol:   symbol,
	}

	_params := params{
		db:          db,
		exchange:    &exchange,
		queryOrder:  &queryOrder,
		symbol:      symbol,
		priceDigits: priceDigits,
		qtyDigits:   qtyDigits,
	}

	if intervalSec == 0 {
		intervalSec = 3
	}

	h.Logf("{Exchange:BinanceSpot Symbol:%s BotID:%d}\n", symbol, botID)

	for range time.Tick(time.Duration(intervalSec) * time.Second) {
		ticker := exchange.GetTicker(symbol)
		if ticker == nil || ticker.Price <= 0 {
			continue
		}

		if startPrice > 0 && ticker.Price > startPrice && len(db.GetActiveOrders(queryOrder)) == 0 {
			continue
		}

		tradeOrders := grid.OnTick(grid.OnTickParams{
			DB:        db,
			Ticker:    ticker,
			BotParams: bp,
		})
		if tradeOrders == nil {
			continue
		}

		_params.ticker = ticker
		_params.tradeOrders = tradeOrders
		if orderType == t.OrderTypeLimit {
			placeAsLimit(&_params)
		} else if orderType == t.OrderTypeMarket {
			placeAsMarket(&_params)
		}
	}
}

func placeAsLimit(p *params) {
	// Open new orders -----------------------------------------------------------
	for _, o := range p.tradeOrders.OpenOrders {
		o.ID = h.GenID()
		_qty := h.RoundToDigits(p.quoteQty/o.OpenPrice, p.qtyDigits)
		if _qty > o.Qty {
			o.Qty = _qty
		}
		exo, err := p.exchange.PlaceLimitOrder(o)
		if err != nil || exo == nil {
			h.Log("OpenOrder")
			os.Exit(1)
		}

		o.RefID = exo.RefID
		o.Status = exo.Status
		o.OpenPrice = exo.OpenPrice
		o.OpenTime = exo.OpenTime
		err = p.db.CreateOrder(o)
		if err != nil {
			h.Log("CreateOrder", err)
			continue
		}
		log := t.LogOpenOrder{
			Action: "NEW",
			Qty:    o.Qty,
			Open:   o.OpenPrice,
			Zone:   o.ZonePrice,
			TP:     o.TPPrice,
		}
		h.Log(log)
	}

	// Synchronize order status / Place TP order ---------------------------------
	for _, o := range p.db.GetLimitOrders(*p.queryOrder) {
		exo, err := p.exchange.GetOrder(o)
		if err != nil || exo == nil {
			h.Log("GetOrder")
			os.Exit(1)
		}
		if exo.Status == t.OrderStatusNew {
			continue
		}

		// Synchronize FILLED/CANCELED order
		if o.Status != exo.Status {
			o.Status = exo.Status
			o.UpdateTime = exo.UpdateTime
			err := p.db.UpdateOrder(o)
			if err != nil {
				h.Log("UpdateOrder", err)
				continue
			}
			if exo.Status == t.OrderStatusFilled {
				log := t.LogOpenOrder{
					Action: "FILLED",
					Qty:    o.Qty,
					Open:   o.OpenPrice,
					Zone:   o.ZonePrice,
					TP:     o.TPPrice,
				}
				h.Log(log)
			}
			if exo.Status == t.OrderStatusCanceled {
				log := t.LogOpenOrder{
					Action: "CANCELED",
					Qty:    o.Qty,
					Open:   o.OpenPrice,
					Zone:   o.ZonePrice,
				}
				h.Log(log)
				continue
			}
		}
		if exo.Status == t.OrderStatusCanceled {
			continue
		}

		// Place a new Take Profit order
		if o.TPPrice > 0 && o.CloseOrderID == "" && p.db.GetTPOrder(o.ID) == nil {
			// Cancel the highest price TP order, because of 'MAX_NUM_ALGO_ORDERS=5'
			const maxNumAlgoOrders = 5
			tpOrders := p.db.GetNewTPOrders(*p.queryOrder)
			// Keep only 2 TP orders at the time
			if len(tpOrders)+3 >= maxNumAlgoOrders {
				_tpo := tpOrders[0]
				// Ignore when the order TP price is so far, keep calm and waiting
				if _tpo.OpenPrice < o.TPPrice {
					continue
				}
				exo, err := p.exchange.CancelOrder(_tpo)
				if err != nil || exo == nil {
					h.Log("CancelOrder")
					os.Exit(1)
				}

				_tpo.Status = exo.Status
				_tpo.UpdateTime = exo.UpdateTime
				err = p.db.UpdateOrder(_tpo)
				if err != nil {
					h.Log(err)
					continue
				}
				log := t.LogTPOrder{
					Action: "CANCELED_TP",
					Qty:    _tpo.Qty,
					Open:   _tpo.OpenPrice,
				}
				h.Log(log)
			}

			book := p.exchange.GetOrderBook(p.symbol, 5)
			if book == nil || len(book.Asks) == 0 {
				continue
			}
			stopPrice := book.Asks[1].Price
			sellPrice := book.Asks[3].Price
			if sellPrice < o.TPPrice {
				continue
			}

			tpo := t.Order{
				BotID:       o.BotID,
				Exchange:    o.Exchange,
				Symbol:      o.Symbol,
				ID:          h.GenID(),
				OpenOrderID: o.ID,
				Qty:         o.Qty,
				Side:        h.Reverse(o.Side),
				Type:        t.OrderTypeTP,
				Status:      t.OrderStatusNew,
				StopPrice:   stopPrice,
				OpenPrice:   sellPrice,
			}
			exo, err := p.exchange.PlaceStopOrder(tpo)
			if err != nil || exo == nil {
				h.Log("PlaceTPOrder", tpo)
				os.Exit(1)
			}

			tpo.RefID = exo.RefID
			tpo.OpenTime = exo.OpenTime
			err = p.db.CreateOrder(tpo)
			if err != nil {
				h.Log(err)
				continue
			}
			log := t.LogTPOrder{
				Action: "NEW_TP",
				Qty:    tpo.Qty,
				Close:  tpo.OpenPrice,
				Zone:   o.ZonePrice,
			}
			h.Log(log)
		}
	}

	// Synchronize TP order status -----------------------------------------------
	for _, tpo := range p.db.GetTPOrders(*p.queryOrder) {
		exo, err := p.exchange.GetOrder(tpo)
		if err != nil || exo == nil {
			h.Log("GetTPOrder")
			os.Exit(1)
		}
		if exo.Status == t.OrderStatusNew {
			continue
		}

		if tpo.Status != exo.Status {
			tpo.Status = exo.Status
			tpo.UpdateTime = exo.UpdateTime
			err := p.db.UpdateOrder(tpo)
			if err != nil {
				h.Log(err)
				continue
			}
			if exo.Status == t.OrderStatusCanceled {
				log := t.LogTPOrder{
					Action: "CANCELED_TP",
					Qty:    tpo.Qty,
					Open:   tpo.OpenPrice,
				}
				h.Log(log)
				continue
			}
		}
		if exo.Status == t.OrderStatusCanceled {
			continue
		}

		oo := p.db.GetOrderByID(tpo.OpenOrderID)
		if oo.CloseOrderID == "" && p.ticker.Price > tpo.OpenPrice {
			oo.CloseOrderID = tpo.ID
			oo.ClosePrice = tpo.OpenPrice
			oo.CloseTime = h.Now13()
			oo.PL = h.RoundToDigits(((oo.ClosePrice-oo.OpenPrice)*tpo.Qty)-oo.Commission-tpo.Commission, p.priceDigits)
			err := p.db.UpdateOrder(*oo)
			if err != nil {
				h.Log(err)
				continue
			}
			log := t.LogTPOrder{
				Action: "FILLED_TP",
				Qty:    tpo.Qty,
				Close:  oo.ClosePrice,
				Open:   oo.OpenPrice,
				Zone:   oo.ZonePrice,
				Profit: oo.PL,
			}
			h.Log(log)
		}
	}
}

func placeAsMarket(p *params) {
	// Open new orders -----------------------------------------------------------
	for _, o := range p.tradeOrders.OpenOrders {
		book := p.exchange.GetOrderBook(p.symbol, 5)
		if book == nil || len(book.Asks) == 0 {
			continue
		}
		buyPrice := book.Asks[0].Price
		if buyPrice > o.ZonePrice || buyPrice == 0 {
			continue
		}

		o.ID = h.GenID()
		_qty := h.RoundToDigits(p.quoteQty/buyPrice, p.qtyDigits)
		if _qty > o.Qty {
			o.Qty = _qty
		}
		o.Type = t.OrderTypeMarket
		exo, err := p.exchange.PlaceMarketOrder(o)
		if err != nil || exo == nil {
			h.Log("OpenOrder")
			os.Exit(1)
		}

		o.RefID = exo.RefID
		o.Status = exo.Status
		o.OpenTime = exo.OpenTime
		o.OpenPrice = exo.OpenPrice
		o.Qty = exo.Qty
		o.Commission = exo.Commission
		err = p.db.CreateOrder(o)
		if err != nil {
			h.Log("CreateOrder", err)
			continue
		}
		log := t.LogOpenOrder{
			Action: "FILLED",
			Qty:    o.Qty,
			Open:   o.OpenPrice,
			Zone:   o.ZonePrice,
			TP:     o.TPPrice,
		}
		h.Log(log)
	}

	// Take Profit ---------------------------------------------------------------
	for _, o := range p.db.GetActiveOrders(*p.queryOrder) {
		if o.TPPrice > 0 && p.db.GetTPOrder(o.ID) == nil {
			book := p.exchange.GetOrderBook(p.symbol, 5)
			if book == nil || len(book.Bids) == 0 {
				continue
			}
			sellPrice := book.Bids[0].Price
			if o.TPPrice > sellPrice || sellPrice == 0 {
				continue
			}

			tpo := t.Order{
				BotID:       o.BotID,
				Exchange:    o.Exchange,
				Symbol:      o.Symbol,
				ID:          h.GenID(),
				OpenOrderID: o.ID,
				Qty:         o.Qty,
				Side:        h.Reverse(o.Side),
				Status:      t.OrderStatusNew,
				Type:        t.OrderTypeMarket,
			}
			exo, err := p.exchange.PlaceMarketOrder(tpo)
			if err != nil || exo == nil {
				h.Log("TakeProfit")
				os.Exit(1)
			}

			tpo.Type = t.OrderTypeTP // Save to the local DB as a TAKE_PROFIT_LIMIT type
			tpo.RefID = exo.RefID
			tpo.Status = exo.Status
			tpo.OpenTime = exo.OpenTime
			tpo.OpenPrice = exo.OpenPrice
			tpo.Qty = exo.Qty
			tpo.Commission = exo.Commission
			err = p.db.CreateOrder(tpo)
			if err != nil {
				h.Log("CreateTPOrder", err)
				continue
			}

			o.CloseOrderID = tpo.ID
			o.ClosePrice = tpo.OpenPrice
			o.CloseTime = tpo.OpenTime
			o.PL = h.RoundToDigits(((o.ClosePrice-o.OpenPrice)*tpo.Qty)-o.Commission-tpo.Commission, p.priceDigits)
			err = p.db.UpdateOrder(o)
			if err != nil {
				h.Log("UpdateOrder", err)
				continue
			}
			log := t.LogTPOrder{
				Action: "FILLED_TP",
				Qty:    tpo.Qty,
				Close:  o.ClosePrice,
				Open:   o.OpenPrice,
				Zone:   o.ZonePrice,
				Profit: o.PL,
			}
			h.Log(log)
		}
	}
}