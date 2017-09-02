package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/tobyjsullivan/btc-frogger/balances"
	"github.com/tobyjsullivan/btc-frogger/coinbase"
	"github.com/tobyjsullivan/btc-frogger/orders"
	"github.com/tobyjsullivan/btc-frogger/rates"
	"github.com/tobyjsullivan/btc-frogger/spread"
)

const (
	TICK_DURATION = 60 * time.Second
)

var (
	dryRun             = os.Getenv("DRY_RUN") != "" && strings.ToLower(os.Getenv("DRY_RUN")) != "false"
	coinbaseAccessKey  = os.Getenv("COINBASE_API_ACCESS_KEY")
	coinbaseSecretKey  = os.Getenv("COINBASE_API_SECRET_KEY")
	coinbasePassphrase = os.Getenv("COINBASE_API_PASSPHRASE")
)

func main() {
	log.SetPrefix("[frogger] ")
	log.SetFlags(0)

	log.Println("Logger initialized.")

	// Attempting a signed request for accounts
	conn := &coinbase.Conn{
		Requester: &coinbase.SignedRequester{
			ApiAccessKey:  coinbaseAccessKey,
			ApiSecretKey:  coinbaseSecretKey,
			ApiPassphrase: coinbasePassphrase,
		},
	}

	ctx := context.Background()

	log.Println("Building balances service...")
	balanceSvc := balances.NewService(ctx, conn)

	log.Println("Building rate service...")
	rateSvc := rates.NewService(ctx, conn)

	log.Println("Building spread service...")
	spreadSvc := spread.NewService(ctx, conn)

	log.Println("Building orders service...")
	orderSvc := orders.NewService(ctx, conn, spreadSvc, dryRun)

	log.Println("Services initialized.")

	// Run the cycle every tick
	ticker := time.NewTicker(TICK_DURATION)
	for range ticker.C {
		// First thing, cancel all pending orders to clear out anything that was unfulfilled last time
		if dryRun {
			log.Println("DRY RUN: Skipping order cancel")
		} else {
			conn.CancelAllOrders()
		}

		ethBtcRate, ok := rateSvc.CurrentRate(coinbase.CURRENCY_ETH, coinbase.CURRENCY_BTC)
		if !ok {
			log.Println("ETH/BTC rate not available")
			continue
		}

		ltcBtcRate, ok := rateSvc.CurrentRate(coinbase.CURRENCY_LTC, coinbase.CURRENCY_BTC)
		if !ok {
			log.Println("LTC/BTC rate not available")
			continue
		}

		log.Printf("Current rates: ETH/BTC - %.4f; LTC/BTC - %.4f\n", ethBtcRate, ltcBtcRate)

		// Check balance
		btcNtvBal, ok := balanceSvc.GetNativeBalance(coinbase.CURRENCY_BTC)
		if !ok {
			log.Println("BTC balance unavailable.")
			continue
		}

		ethNtvBal, ok := balanceSvc.GetNativeBalance(coinbase.CURRENCY_ETH)
		if !ok {
			log.Println("ETH balance unavailable.")
			continue
		}

		ltcNtvBal, ok := balanceSvc.GetNativeBalance(coinbase.CURRENCY_LTC)
		if !ok {
			log.Println("LTC balance unavailable.")
			continue
		}

		log.Printf("Current Holdings: BTC: %s ETH: %s LTC: %s\n", fmtAmount(btcNtvBal), fmtAmount(ethNtvBal), fmtAmount(ltcNtvBal))

		btcAssets := btcNtvBal
		ethAssets, err := rateSvc.Convert(coinbase.CURRENCY_ETH, coinbase.CURRENCY_BTC, ethNtvBal)
		if err != nil {
			log.Println("ETH-BTC convert:", err)
			continue
		}
		ltcAssets, err := rateSvc.Convert(coinbase.CURRENCY_LTC, coinbase.CURRENCY_BTC, ltcNtvBal)
		if err != nil {
			log.Println("LTC-BTC convert:", err)
			continue
		}

		totalAssets := btcAssets + ethAssets + ltcAssets
		log.Printf("Total Assets: %s BTC - %s\n", fmtAmount(totalAssets), time.Now())

		idealDistribution := totalAssets / 3

		ethDiff := idealDistribution - ethAssets
		ltcDiff := idealDistribution - ltcAssets

		ntvEthDiff, err := rateSvc.Convert(coinbase.CURRENCY_BTC, coinbase.CURRENCY_ETH, ethDiff)
		if err != nil {
			log.Println("BTC-ETH convert:", err)
			continue
		}

		ntvLtcDiff, err := rateSvc.Convert(coinbase.CURRENCY_BTC, coinbase.CURRENCY_LTC, ltcDiff)
		if err != nil {
			log.Println("BTC-LTC convert:", err)
			continue
		}

		log.Printf("Trade Goals: %s ETH; %s LTC\n", fmtAmount(ntvEthDiff), fmtAmount(ntvLtcDiff))

		ethBid, _ := spreadSvc.CurrentBid(coinbase.ProductID_ETH_BTC)
		ethAsk, _ := spreadSvc.CurrentAsk(coinbase.ProductID_ETH_BTC)
		ltcBid, _ := spreadSvc.CurrentBid(coinbase.ProductID_LTC_BTC)
		ltcAsk, _ := spreadSvc.CurrentAsk(coinbase.ProductID_LTC_BTC)
		log.Printf("Current spreads: ETH/BTC - %d:%d; LTC/BTC - %d:%d\n", ethBid, ethAsk, ltcBid, ltcAsk)

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
		if ntvEthDiff < 0 {
			orderSvc.PlaceOrder(coinbase.CURRENCY_ETH, coinbase.SideSell, 0-ntvEthDiff)
		}
		if ltcDiff < 0 {
			orderSvc.PlaceOrder(coinbase.CURRENCY_LTC, coinbase.SideSell, 0-ntvLtcDiff)
		}
		// Then buy any ETH or LTC
		if ethDiff > 0 {
			orderSvc.PlaceOrder(coinbase.CURRENCY_ETH, coinbase.SideBuy, ntvEthDiff)
		}
		if ltcDiff > 0 {
			orderSvc.PlaceOrder(coinbase.CURRENCY_LTC, coinbase.SideBuy, ntvLtcDiff)
		}
	}

	log.Println("Done. Goodbye!")
}

func fmtAmount(amount int64) string {
	return fmt.Sprintf("%.8f", float64(amount)/coinbase.AmountCoin)
}
