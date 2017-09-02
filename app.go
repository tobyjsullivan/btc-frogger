package main

import (
	"log"
	"time"
	"fmt"
	"errors"
	"github.com/tobyjsullivan/btc-frogger/coinbase"
	"os"
	"strings"
)

const (
	TICK_DURATION = 10 * time.Minute

	COINBASE_FEE = 0.003
	COINBASE_MIN_TRADE = int64(0.01 * float64(AMOUNT_COIN))

	AMOUNT_SATOSHI = 1
	AMOUNT_COIN = 100000000 * AMOUNT_SATOSHI

	PRODUCT_ID_ETH_BTC = "ETH-BTC"
	PRODUCT_ID_LTC_BTC = "LTC-BTC"
)

var (
	dryRun = os.Getenv("DRY_RUN") != "" && strings.ToLower(os.Getenv("DRY_RUN")) != "false"
	coinbaseAccessKey = os.Getenv("COINBASE_API_ACCESS_KEY")
	coinbaseSecretKey = os.Getenv("COINBASE_API_SECRET_KEY")
	coinbasePassphrase = os.Getenv("COINBASE_API_PASSPHRASE")

	currentState *state
)

func init() {
	currentState = &state{
		balances: make(map[coinbase.Currency]int64),
	}
}

type state struct {
	balances map[coinbase.Currency]int64
}

func updateBalances(conn *coinbase.Conn) {
	accounts, err := conn.GetAccounts()
	if err != nil {
		log.Fatalln("getAccounts:", err)
	}

	for _, acct := range accounts {
		currentState.balances[acct.Currency] = acct.Balance
	}
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

	// Run the cycle every tick
	ticker := time.NewTicker(TICK_DURATION)
	for range ticker.C {
		updateBalances(conn)

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

		btcNativeBal, _ := currentNativeBalance(coinbase.CURRENCY_BTC)
		ethNativeBal, _ := currentNativeBalance(coinbase.CURRENCY_ETH)
		ltcNativeBal, _ := currentNativeBalance(coinbase.CURRENCY_LTC)

		log.Printf("Current Holdings: BTC: %s ETH: %s LTC: %s\n", fmtAmount(btcNativeBal), fmtAmount(ethNativeBal), fmtAmount(ltcNativeBal))

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
			if err := sellEth(conn, 0-ethDiff); err != nil {
				log.Println("sellEth:", err)
			}
		}
		if ltcDiff < 0 {
			if err := sellLtc(conn, 0-ltcDiff); err != nil {
				log.Println("sellLtc:", err)
			}
		}
		// Then buy any ETH or LTC
		if ethDiff > 0 {
			if err := buyEth(conn, ethDiff); err != nil {
				log.Println("buyEth:", err)
			}
		}
		if ltcDiff > 0 {
			if err := buyLtc(conn, ltcDiff); err != nil {
				log.Println("buyLtc:", err)
			}
		}
	}

	log.Println("Done. Goodbye!")
}

func fmtAmount(amount int64) string {
	return fmt.Sprintf("%.8f", float64(amount) / AMOUNT_COIN)
}

func toNativeAmount(conn *coinbase.Conn, c coinbase.Currency, amountBtc int64) (int64, error) {
	rate, err := currentRate(conn, c)
	if err != nil {
		return 0, err
	}

	return int64(float64(amountBtc) / rate), nil
}

func buyEth(conn *coinbase.Conn, amountBtc int64) error {
	amountNative, err := toNativeAmount(conn, coinbase.CURRENCY_ETH, amountBtc)
	if err != nil {
		return err
	}

	if amountNative < COINBASE_MIN_TRADE {
		return errors.New("Trade too small: "+fmtAmount(amountNative))
	}

	log.Println("Buying ETH")

	if dryRun {
		log.Println("DRY RUN: order skipped")
		return nil
	}

	return conn.PlaceOrder(coinbase.CURRENCY_ETH, coinbase.SideBuy, amountNative)
}

func sellEth(conn *coinbase.Conn, amountBtc int64) error {
	amountNative, err := toNativeAmount(conn, coinbase.CURRENCY_ETH, amountBtc)
	if err != nil {
		return err
	}

	if amountNative < COINBASE_MIN_TRADE {
		return errors.New("Trade too small: "+fmtAmount(amountNative))
	}

	log.Println("Selling ETH")

	if dryRun {
		log.Println("DRY RUN: order skipped")
		return nil
	}

	return conn.PlaceOrder(coinbase.CURRENCY_ETH, coinbase.SideSell, amountNative)
}

func buyLtc(conn *coinbase.Conn, amountBtc int64) error {
	amountNative, err := toNativeAmount(conn, coinbase.CURRENCY_LTC, amountBtc)
	if err != nil {
		return err
	}

	if amountNative < COINBASE_MIN_TRADE {
		return errors.New("Trade too small: "+fmtAmount(amountNative))
	}

	log.Println("Buying LTC")

	if dryRun {
		log.Println("DRY RUN: order skipped")
		return nil
	}

	return conn.PlaceOrder(coinbase.CURRENCY_LTC, coinbase.SideBuy, amountNative)
}

func sellLtc(conn *coinbase.Conn, amountBtc int64) error {
	amountNative, err := toNativeAmount(conn, coinbase.CURRENCY_LTC, amountBtc)
	if err != nil {
		return err
	}

	if amountNative < COINBASE_MIN_TRADE {
		return errors.New("Trade too small: "+fmtAmount(amountNative))
	}

	log.Println("Selling LTC")

	if dryRun {
		log.Println("DRY RUN: order skipped")
		return nil
	}

	return conn.PlaceOrder(coinbase.CURRENCY_LTC, coinbase.SideSell, amountNative)
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
	nativeBal, ok := currentState.balances[c]
	if !ok {
		return 0, errors.New("Unknown currency")
	}

	return nativeBal, nil
}
