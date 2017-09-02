package spread

import (
	"context"
	"log"
	"os"
	"time"

	"github.com/tobyjsullivan/btc-frogger/coinbase"
)

const (
	loopDuration = 1 * time.Second
)

type spread struct {
	bid int64
	ask int64
}

type SpreadSvc struct {
	conn   *coinbase.Conn
	spreads  map[coinbase.ProductID]*spread
	logger *log.Logger
}

func NewService(ctx context.Context, conn *coinbase.Conn) *SpreadSvc {
	svc := &SpreadSvc{
		conn:   conn,
		spreads:  make(map[coinbase.ProductID]*spread),
		logger: log.New(os.Stdout, "[spread] ", 0),
	}

	go svc.loop(ctx)

	return svc
}

func (svc *SpreadSvc) CurrentBid(pid coinbase.ProductID) (int64, bool) {
	cur, ok := svc.spreads[pid]
	if !ok {
		return 0, false
	}

	return cur.bid, true
}

func (svc *SpreadSvc) CurrentAsk(pid coinbase.ProductID) (int64, bool) {
	cur, ok := svc.spreads[pid]
	if !ok {
		return 0, false
	}

	return cur.ask, true
}

func (svc *SpreadSvc) loop(ctx context.Context) {
	ticker := time.NewTicker(loopDuration)

	for {
		select {
		case <-ticker.C:
			svc.updateSpreads()
		case <-ctx.Done():
			return
		}
	}
}

func (svc *SpreadSvc) updateSpreads() {
	ratesToGet := []coinbase.ProductID{coinbase.ProductID_ETH_BTC, coinbase.ProductID_LTC_BTC}

	for _, prodId := range ratesToGet {
		book, err := svc.conn.CurrentBook(prodId)
		if err != nil {
			svc.logger.Println("book:", err)
			continue
		}

		svc.spreads[prodId] = &spread{
			bid: book.Bid,
			ask: book.Ask,
		}
	}
}
