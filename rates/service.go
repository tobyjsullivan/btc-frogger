package rates

import (
	"context"
	"errors"
	"log"
	"os"
	"time"

	"github.com/tobyjsullivan/btc-frogger/coinbase"
)

const (
	loopDuration = 1 * time.Second
)

type RateSvc struct {
	conn   *coinbase.Conn
	rates  map[coinbase.ProductID]float64
	logger *log.Logger
}

func NewService(ctx context.Context, conn *coinbase.Conn) *RateSvc {
	svc := &RateSvc{
		conn:   conn,
		rates:  make(map[coinbase.ProductID]float64),
		logger: log.New(os.Stdout, "[rates] ", 0),
	}

	go svc.loop(ctx)

	return svc
}

func (svc *RateSvc) CurrentRate(from, to coinbase.Currency) (float64, bool) {
	var prodId coinbase.ProductID
	var invert bool

	if from == coinbase.CurrencyBtc && to == coinbase.CurrencyEth {
		prodId = coinbase.ProductEthBtc
		invert = true
	} else if from == coinbase.CurrencyEth && to == coinbase.CurrencyBtc {
		prodId = coinbase.ProductEthBtc
		invert = false
	} else if from == coinbase.CurrencyBtc && to == coinbase.CurrencyLtc {
		prodId = coinbase.ProductLtcBtc
		invert = true
	} else if from == coinbase.CurrencyLtc && to == coinbase.CurrencyBtc {
		prodId = coinbase.ProductLtcBtc
		invert = false
	} else if from == coinbase.CurrencyBtc && to == coinbase.CurrencyUsd {
		prodId = coinbase.ProductBtcUsd
		invert = false
	} else if from == coinbase.CurrencyUsd && to == coinbase.CurrencyBtc {
		prodId = coinbase.ProductBtcUsd
		invert = true
	} else {
		return 0, false
	}

	rate, ok := svc.rates[prodId]
	if !ok {
		return 0, false
	}

	if invert {
		rate = 1.0 / rate
	}

	return rate, true
}

func (svc *RateSvc) Convert(from, to coinbase.Currency, amount int64) (int64, error) {
	rate, ok := svc.CurrentRate(from, to)

	if !ok {
		return 0, errors.New("Rate unavailable")
	}

	return int64(float64(amount) * rate), nil
}

func (svc *RateSvc) loop(ctx context.Context) {
	ticker := time.NewTicker(loopDuration)

	for {
		select {
		case <-ticker.C:
			svc.updateRates()
		case <-ctx.Done():
			return
		}
	}
}

func (svc *RateSvc) updateRates() {
	ratesToGet := []coinbase.ProductID{
		coinbase.ProductEthBtc,
		coinbase.ProductLtcBtc,
		coinbase.ProductBtcUsd,
	}

	for _, prodId := range ratesToGet {
		ticker, err := svc.conn.CurrentTicker(prodId)
		if err != nil {
			svc.logger.Println("ticker:", err)
			continue
		}

		svc.rates[prodId] = ticker.Price
	}
}
