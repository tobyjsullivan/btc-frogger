package orders

import (
	"context"
	"log"
	"os"

	"github.com/tobyjsullivan/btc-frogger/coinbase"
	"github.com/tobyjsullivan/btc-frogger/spread"
)

const (
	coinbaseMinTrade = int64(0.01 * float64(coinbase.AmountCoin))
	coinbaseQuoteIncrement = 1000
)

type OrderSvc struct {
	conn       *coinbase.Conn
	orderQueue chan *orderReq
	spreadSvc  *spread.SpreadSvc
	dryRun     bool
	logger     *log.Logger
}

func NewService(ctx context.Context, conn *coinbase.Conn, spreadSvc *spread.SpreadSvc, dryRun bool) *OrderSvc {
	svc := &OrderSvc{
		conn:       conn,
		orderQueue: make(chan *orderReq, 2),
		spreadSvc:  spreadSvc,
		dryRun:     dryRun,
		logger:     log.New(os.Stdout, "[orders] ", 0),
	}

	go svc.loop(ctx)

	return svc
}

func (svc *OrderSvc) PlaceOrder(c coinbase.Currency, side coinbase.OrderSide, ntvAmount int64) {
	svc.orderQueue <- &orderReq{
		currency:  c,
		side:      side,
		ntvAmount: ntvAmount,
	}
}

type orderReq struct {
	currency  coinbase.Currency
	side      coinbase.OrderSide
	ntvAmount int64
}

func (svc *OrderSvc) loop(ctx context.Context) {
	mSpreadIndex := map[coinbase.Currency]coinbase.ProductID{
		coinbase.CURRENCY_ETH: coinbase.ProductID_ETH_BTC,
		coinbase.CURRENCY_LTC: coinbase.ProductID_LTC_BTC,
	}

	for {
		select {
		case ord := <-svc.orderQueue:
			svc.logger.Println("Processing order:", ord.side, ord.ntvAmount, ord.currency)

			if ord.ntvAmount < coinbaseMinTrade {
				svc.logger.Println("Skipping: Trade too small.")
				continue
			}

			// Determine limit price
			var price int64
			pid, _ := mSpreadIndex[ord.currency]
			var spreadReady bool
			switch ord.side {
			case coinbase.SideBuy:
				price, spreadReady = svc.spreadSvc.CurrentAsk(pid)
				price -= coinbaseQuoteIncrement // Exactly one point below
			case coinbase.SideSell:
				price, spreadReady = svc.spreadSvc.CurrentBid(pid)
				price += coinbaseQuoteIncrement // Exactly one point above current bid
			default:
				svc.logger.Println("Unexpected side:", ord.side)
				continue
			}
			if !spreadReady {
				svc.logger.Println("Spread wasnt ready:", pid)
				continue
			}

			svc.logger.Println("Order limit price:", price)

			if svc.dryRun {
				svc.logger.Println("DRY RUN: order skipped.")
				continue
			}

			if err := svc.conn.PlaceOrder(ord.currency, ord.side, ord.ntvAmount, price); err != nil {
				svc.logger.Println("place order:", err)
				continue
			}
		case <-ctx.Done():
			return
		}
	}
}
