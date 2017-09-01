package main

import (
	"log"
	"time"
	"fmt"
	"errors"
	"github.com/tobyjsullivan/btc-frogger/coinbase"
	"os"
)

const (
	TICK_DURATION = 2 * time.Second

	COINBASE_FEE = 0.003
	COINBASE_MIN_TRADE = int64(0.01 * float64(AMOUNT_COIN))

	AMOUNT_SATOSHI = 1
	AMOUNT_COIN = 100000000 * AMOUNT_SATOSHI

	PRODUCT_ID_ETH_BTC = "ETH-BTC"
	PRODUCT_ID_LTC_BTC = "LTC-BTC"
)

var (
	coinbaseAccessKey = os.Getenv("COINBASE_API_ACCESS_KEY")
	coinbaseSecretKey = os.Getenv("COINBASE_API_SECRET_KEY")
	coinbasePassphrase = os.Getenv("COINBASE_API_PASSPHRASE")

	sim *simulation
)

func init() {
	sim = &simulation{
		balances: make(map[coinbase.Currency]int64),
	}
}

type simulation struct {
	balances map[coinbase.Currency]int64
}

func (s *simulation) transfer(conn *coinbase.Conn, from, to coinbase.Currency, amount int64) error {
	fromRate, err := currentRate(conn, from)
	if err != nil {
		return err
	}

	toRate, err := currentRate(conn, to)
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

	// Attempting a signed request for accounts
	conn := &coinbase.Conn{
		Requester: &coinbase.SignedRequester{
			ApiAccessKey: coinbaseAccessKey,
			ApiSecretKey: coinbaseSecretKey,
			ApiPassphrase: coinbasePassphrase,
		},
	}

	accounts, err := conn.GetAccounts()
	if err != nil {
		log.Fatalln("getAccounts:", err)
	}

	for _, acct := range accounts {
		log.Println("Account:", acct.ID, string(acct.Currency), acct.Balance)
		sim.balances[acct.Currency] = acct.Balance
	}

	// Run the cycle every tick
	ticker := time.NewTicker(TICK_DURATION)
	for range ticker.C {
		// Check balance
		btcBalance, err := currentBalance(conn, coinbase.CURRENCY_BTC)
		if err != nil {
			log.Panicln(err)
		}
		ethBalance, err := currentBalance(conn, coinbase.CURRENCY_ETH)
		if err != nil {
			log.Panicln(err)
		}
		ltcBalance, err := currentBalance(conn, coinbase.CURRENCY_LTC)
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
			sell(conn, coinbase.CURRENCY_ETH, 0-ethDiff)
		}
		if ltcDiff < 0 {
			sell(conn, coinbase.CURRENCY_LTC, 0-ltcDiff)
		}
		// Then buy any ETH or LTC
		if ethDiff > 0 {
			buy(conn, coinbase.CURRENCY_ETH, ethDiff)
		}
		if ltcDiff > 0 {
			buy(conn, coinbase.CURRENCY_LTC, ltcDiff)
		}
	}



	log.Println("Done. Goodbye!")
}

func fmtAmount(amount int64) string {
	return fmt.Sprintf("%.8f", float64(amount) / AMOUNT_COIN)
}

func sell(conn *coinbase.Conn, c coinbase.Currency, amount int64) error {
	if amount < COINBASE_MIN_TRADE {
		// Not an error. Just skip the trade
		return nil
	}

	// TODO Implement for real
	switch c {
	case coinbase.CURRENCY_ETH:
		sim.transfer(conn, coinbase.CURRENCY_ETH, coinbase.CURRENCY_BTC, amount)
	case coinbase.CURRENCY_LTC:
		sim.transfer(conn, coinbase.CURRENCY_LTC, coinbase.CURRENCY_BTC, amount)
	default:
		return errors.New("Unsupported currency.")
	}

	return nil
}

func buy(conn *coinbase.Conn, c coinbase.Currency, amount int64) error {
	if amount < COINBASE_MIN_TRADE {
		// Not an error. Just skip the trade
		return nil
	}

	// TODO Implement for real
	switch c {
	case coinbase.CURRENCY_ETH:
		sim.transfer(conn, coinbase.CURRENCY_BTC, coinbase.CURRENCY_ETH, amount)
	case coinbase.CURRENCY_LTC:
		sim.transfer(conn, coinbase.CURRENCY_BTC, coinbase.CURRENCY_LTC, amount)
	default:
		return errors.New("Unsupported currency.")
	}

	return nil
}

func currentBalance(conn *coinbase.Conn, c coinbase.Currency) (int64, error) {
	amt, err := currentNativeBalance(c)
	if err != nil {
		log.Println("currentNativeBalance:", err)
		return 0, err
	}
	rate, err := currentRate(conn, c)
	if err != nil {
		log.Println("currentRate:", err)
		return 0, err
	}
	return int64(float64(amt) * rate), nil
}

func currentRate(conn *coinbase.Conn, c coinbase.Currency) (float64, error) {
	switch c {
	case coinbase.CURRENCY_BTC:
		return 1.0, nil
	case coinbase.CURRENCY_ETH:
		ticker, err := conn.CurrentTicker(coinbase.ProductID_ETH_BTC)
		if err != nil {
			log.Println("ticker:", err)
			return 0, err
		}

		return ticker.Price, nil
	case coinbase.CURRENCY_LTC:
		ticker, err := conn.CurrentTicker(coinbase.ProductID_LTC_BTC)
		if err != nil {
			log.Println("ticker:", err)
			return 0, err
		}

		return ticker.Price, nil
	default:
		return 0, errors.New("Unknown currency")
	}
}

func currentNativeBalance(c coinbase.Currency) (int64, error) {
	// TODO Implement for real
	nativeBal, ok := sim.balances[c]
	if !ok {
		return 0, errors.New("Unknown currency")
	}

	return nativeBal, nil
}
