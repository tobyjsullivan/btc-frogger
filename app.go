package main

import (
	"log"
	"time"
	"fmt"
	"errors"
	"github.com/tobyjsullivan/btc-frogger/coinbase"
)

const (
	TICK_DURATION = 10 * time.Minute

	COINBASE_FEE = 0.003
	COINBASE_MIN_TRADE = int64(0.01 * float64(AMOUNT_COIN))

	CURRENCY_ETH = Currency("ETH")
	CURRENCY_BTC = Currency("BTC")
	CURRENCY_LTC = Currency("LTC")

	AMOUNT_SATOSHI = 1
	AMOUNT_COIN = 100000000 * AMOUNT_SATOSHI

	PRODUCT_ID_ETH_BTC = "ETH-BTC"
	PRODUCT_ID_LTC_BTC = "LTC-BTC"
)

var (
	sim *simulation
)

func init() {
	sim = &simulation{
		balances: make(map[Currency]int64),
	}
	sim.balances[CURRENCY_BTC] = 32281620
	sim.balances[CURRENCY_ETH] = 399644680
	sim.balances[CURRENCY_LTC] = 2260356890
}

type Currency string

type simulation struct {
	balances map[Currency]int64
}

func (s *simulation) transfer(from Currency, to Currency, amount int64) error {
	fromRate, err := currentRate(from)
	if err != nil {
		return err
	}

	toRate, err := currentRate(to)
	if err != nil {
		return err
	}

	log.Printf("TRANSFER: %d %s @ %f to %s @ %f (Fee: %d)\n", amount, from, fromRate, to, 1.0 / toRate, int(float64(amount) * COINBASE_FEE))

	s.balances[from] -= int64(float64(amount) * fromRate)
	s.balances[to] += int64(float64(amount) * (1.0 - COINBASE_FEE) / toRate)
	return nil
}

func main() {
	log.SetPrefix("[frogger] ")
	log.SetFlags(0)

	log.Println("Logger initialized.")

	// Run the cycle every tick
	ticker := time.NewTicker(TICK_DURATION)
	for range ticker.C {
		// Check balance
		btcBalance, err := currentBalance(CURRENCY_BTC)
		if err != nil {
			log.Panicln(err)
		}
		ethBalance, err := currentBalance(CURRENCY_ETH)
		if err != nil {
			log.Panicln(err)
		}
		ltcBalance, err := currentBalance(CURRENCY_LTC)
		if err != nil {
			log.Panicln(err)
		}
		//log.Printf("Current Holdings: BTC: %s ETH: %s LTC: %s\n", fmtAmount(btcBalance), fmtAmount(ethBalance), fmtAmount(ltcBalance))

		totalBalance := btcBalance + ethBalance + ltcBalance
		log.Printf("Total Assets: %s BTC - %s\n", fmtAmount(totalBalance), time.Now())

		idealDistribution := totalBalance / 3

		btcDiff := idealDistribution - btcBalance
		ethDiff := idealDistribution - ethBalance
		ltcDiff := idealDistribution - ltcBalance

		log.Printf("Trade Goals: BTC: %s ETH: %s LTC: %s\n", fmtAmount(btcDiff), fmtAmount(ethDiff), fmtAmount(ltcDiff))

		// There are six potential cases
		// 1. +BTC, -ETH, -LTC
		// 2. +BTC, +ETH, -LTC
		// 3. +BTC, -ETH, +LTC
		// 4. -BTC, +ETH, -LTC
		// 5. -BTC, -ETH, +LTC
		// 6. -BTC, +ETH, +LTC
		// Although we cannot exchange ETH <-> LTC directly, we can satisfy all of these cases in two trades
		// NOTE: Since BTC is the intermediary currency, we never actually have to buy or sell it explicitly
		// Sell any ETH or LTC first
		if ethDiff < 0 {
			sell(CURRENCY_ETH, 0-ethDiff)
		}
		if ltcDiff < 0 {
			sell(CURRENCY_LTC, 0-ltcDiff)
		}
		// Then buy any ETH or LTC
		if ethDiff > 0 {
			buy(CURRENCY_ETH, ethDiff)
		}
		if ltcDiff > 0 {
			buy(CURRENCY_LTC, ltcDiff)
		}
	}



	log.Println("Done. Goodbye!")
}

func fmtAmount(amount int64) string {
	return fmt.Sprintf("%.8f", float64(amount) / AMOUNT_COIN)
}

func sell(c Currency, amount int64) error {
	if amount < COINBASE_MIN_TRADE {
		// Not an error. Just skip the trade
		return nil
	}

	// TODO Implement for real
	switch c {
	case CURRENCY_ETH:
		sim.transfer(CURRENCY_ETH, CURRENCY_BTC, amount)
	case CURRENCY_LTC:
		sim.transfer(CURRENCY_LTC, CURRENCY_BTC, amount)
	default:
		return errors.New("Unsupported currency.")
	}

	return nil
}

func buy(c Currency, amount int64) error {
	if amount < COINBASE_MIN_TRADE {
		// Not an error. Just skip the trade
		return nil
	}

	// TODO Implement for real
	switch c {
	case CURRENCY_ETH:
		sim.transfer(CURRENCY_BTC, CURRENCY_ETH, amount)
	case CURRENCY_LTC:
		sim.transfer(CURRENCY_BTC, CURRENCY_LTC, amount)
	default:
		return errors.New("Unsupported currency.")
	}

	return nil
}

func currentBalance(c Currency) (int64, error) {
	amt, err := currentNativeBalance(c)
	if err != nil {
		log.Println("currentNativeBalance:", err)
		return 0, err
	}
	rate, err := currentRate(c)
	if err != nil {
		log.Println("currentRate:", err)
		return 0, err
	}
	return int64(float64(amt) * rate), nil
}

func currentRate(c Currency) (float64, error) {
	switch c {
	case CURRENCY_BTC:
		return 1.0, nil
	case CURRENCY_ETH:
		ticker, err := coinbase.CurrentTicker(coinbase.ProductID_ETH_BTC)
		if err != nil {
			log.Println("ticker:", err)
			return 0, err
		}

		return ticker.Price, nil
	case CURRENCY_LTC:
		ticker, err := coinbase.CurrentTicker(coinbase.ProductID_LTC_BTC)
		if err != nil {
			log.Println("ticker:", err)
			return 0, err
		}

		return ticker.Price, nil
	default:
		return 0, errors.New("Unknown currency")
	}
}

func currentNativeBalance(c Currency) (int64, error) {
	// TODO Implement for real
	nativeBal, ok := sim.balances[c]
	if !ok {
		return 0, errors.New("Unknown currency")
	}

	return nativeBal, nil
}
