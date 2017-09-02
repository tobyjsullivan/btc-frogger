package orders

import (
	"context"
	"log"
	"os"

	"github.com/tobyjsullivan/btc-frogger/coinbase"
)

const (
	coinbaseMinTrade = int64(0.01 * float64(coinbase.AmountCoin))
)

type OrderSvc struct {
	conn       *coinbase.Conn
	orderQueue chan *orderReq
	dryRun     bool
	logger     *log.Logger
}

func NewService(ctx context.Context, conn *coinbase.Conn, dryRun bool) *OrderSvc {
	svc := &OrderSvc{
		conn:       conn,
		orderQueue: make(chan *orderReq, 2),
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

	for {
		select {
		case ord := <-svc.orderQueue:
			svc.logger.Println("Processing order:", ord.side, ord.ntvAmount, ord.currency)

			if ord.ntvAmount < coinbaseMinTrade {
				svc.logger.Println("Skipping: Trade too small.")
				continue
			}

			if svc.dryRun {
				svc.logger.Println("DRY RUN: order skipped.")
				continue
			}

			if err := svc.conn.PlaceOrder(ord.currency, ord.side, ord.ntvAmount); err != nil {
				svc.logger.Println("place order:", err)
				continue
			}
		case <-ctx.Done():
			return
		}
	}
}
