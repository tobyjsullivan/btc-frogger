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
	"github.com/tobyjsullivan/btc-frogger/reporting"
	"errors"
	"math"
)

const (
	TICK_DURATION = 10 * time.Second
)

var (
	dryRun             = os.Getenv("DRY_RUN") != "" && strings.ToLower(os.Getenv("DRY_RUN")) != "false"
	coinbaseAccessKey  = os.Getenv("COINBASE_API_ACCESS_KEY")
	coinbaseSecretKey  = os.Getenv("COINBASE_API_SECRET_KEY")
	coinbasePassphrase = os.Getenv("COINBASE_API_PASSPHRASE")
	dweetThingName = os.Getenv("DWEET_THING_NAME")
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

	log.Println("Building reporting service...")
	reportingSvc := reporting.NewService(dweetThingName, dryRun)

	log.Println("Building balances service...")
	balanceSvc := balances.NewService(ctx, conn)

	log.Println("Building rate service...")
	rateSvc := rates.NewService(ctx, conn)

	log.Println("Building spread service...")
	spreadSvc := spread.NewService(ctx, conn)

	log.Println("Building orders service...")
	orderSvc := orders.NewService(ctx, conn, spreadSvc, dryRun)

	log.Println("Services initialized.")

	go func(rateSvc *rates.RateSvc, balanceSvc *balances.BalanceSvc){
		ticker := time.Tick(2 * time.Second)
		for range ticker {
			distro, err := computeDistribution(rateSvc, balanceSvc)
			if err != nil {
				log.Println("reporting assets:", err)
				continue
			}
			ethRate, _ := rateSvc.CurrentRate(coinbase.CurrencyEth, coinbase.CurrencyBtc)
			ltcRate, _ := rateSvc.CurrentRate(coinbase.CurrencyLtc, coinbase.CurrencyBtc)
			usdRate, _ := rateSvc.CurrentRate(coinbase.CurrencyBtc, coinbase.CurrencyUsd)

			totalAssets := float64(distro.totalAssets) / float64(coinbase.AmountCoin)
			reportingSvc.ReportMetrics(&reporting.Report{
				TotalAssets: totalAssets,
				AssetValueUsd: math.Floor((totalAssets * usdRate) * 100) / 100,
				BtcBalance: float64(distro.ntvBtcBalance) / float64(coinbase.AmountCoin),
				EthBalance: float64(distro.ntvEthBalance) / float64(coinbase.AmountCoin),
				LtcBalance: float64(distro.ntvLtcBalance) / float64(coinbase.AmountCoin),
				EthRate: ethRate,
				LtcRate: ltcRate,
				UsdRate: usdRate,
			})
		}
	}(rateSvc, balanceSvc)

	// Run the cycle every tick
	ticker := time.NewTicker(TICK_DURATION)
	for range ticker.C {
		// First thing, cancel all pending orders to clear out anything that was unfulfilled last time
		if dryRun {
			log.Println("DRY RUN: Skipping order cancel")
		} else {
			conn.CancelAllOrders()
		}

		ethBtcRate, ok := rateSvc.CurrentRate(coinbase.CurrencyEth, coinbase.CurrencyBtc)
		if !ok {
			log.Println("ETH/BTC rate not available")
			continue
		}

		ltcBtcRate, ok := rateSvc.CurrentRate(coinbase.CurrencyLtc, coinbase.CurrencyBtc)
		if !ok {
			log.Println("LTC/BTC rate not available")
			continue
		}

		log.Printf("Current rates: ETH/BTC - %.4f; LTC/BTC - %.4f\n", ethBtcRate, ltcBtcRate)

		distro, err := computeDistribution(rateSvc, balanceSvc)
		if err != nil {
			log.Println("compute total assets:", err)
			continue
		}
		log.Printf("Current Holdings: BTC: %s ETH: %s LTC: %s\n", fmtAmount(distro.ntvBtcBalance),
			fmtAmount(distro.ntvEthBalance), fmtAmount(distro.ntvLtcBalance))
		log.Printf("Total Assets: %s BTC - %s\n", fmtAmount(distro.totalAssets), time.Now())


		ntvEthDiff, err := rateSvc.Convert(coinbase.CurrencyBtc, coinbase.CurrencyEth, distro.diffEth)
		if err != nil {
			log.Println("BTC-ETH convert:", err)
			continue
		}

		ntvLtcDiff, err := rateSvc.Convert(coinbase.CurrencyBtc, coinbase.CurrencyLtc, distro.diffLtc)
		if err != nil {
			log.Println("BTC-LTC convert:", err)
			continue
		}

		log.Printf("Trade Goals: %s ETH; %s LTC\n", fmtAmount(ntvEthDiff), fmtAmount(ntvLtcDiff))

		ethBid, _ := spreadSvc.CurrentBid(coinbase.ProductEthBtc)
		ethAsk, _ := spreadSvc.CurrentAsk(coinbase.ProductEthBtc)
		ltcBid, _ := spreadSvc.CurrentBid(coinbase.ProductLtcBtc)
		ltcAsk, _ := spreadSvc.CurrentAsk(coinbase.ProductLtcBtc)
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
			orderSvc.PlaceOrder(coinbase.CurrencyEth, coinbase.SideSell, 0-ntvEthDiff)
		}
		if ntvLtcDiff < 0 {
			orderSvc.PlaceOrder(coinbase.CurrencyLtc, coinbase.SideSell, 0-ntvLtcDiff)
		}
		// Then buy any ETH or LTC
		if ntvEthDiff > 0 {
			orderSvc.PlaceOrder(coinbase.CurrencyEth, coinbase.SideBuy, ntvEthDiff)
		}
		if ntvLtcDiff > 0 {
			orderSvc.PlaceOrder(coinbase.CurrencyLtc, coinbase.SideBuy, ntvLtcDiff)
		}
	}

	log.Println("Done. Goodbye!")
}

func fmtAmount(amount int64) string {
	return fmt.Sprintf("%.8f", float64(amount)/coinbase.AmountCoin)
}

type distribution struct {
	totalAssets int64
	ntvBtcBalance int64
	ntvEthBalance int64
	ntvLtcBalance int64
	curBtcAssets int64
	curEthAssets int64
	curLtcAssets int64
	diffEth int64
	diffLtc int64
}

func computeDistribution(rateSvc *rates.RateSvc, balanceSvc *balances.BalanceSvc) (*distribution, error) {
	// Check balance
	btcNtvBal, ok := balanceSvc.GetNativeBalance(coinbase.CurrencyBtc)
	if !ok {
		return nil, errors.New("BTC balance unavailable.")
	}

	ethNtvBal, ok := balanceSvc.GetNativeBalance(coinbase.CurrencyEth)
	if !ok {
		return nil, errors.New("ETH balance unavailable.")
	}

	ltcNtvBal, ok := balanceSvc.GetNativeBalance(coinbase.CurrencyLtc)
	if !ok {
		return nil, errors.New("LTC balance unavailable.")
	}

	btcAssets := btcNtvBal
	ethAssets, err := rateSvc.Convert(coinbase.CurrencyEth, coinbase.CurrencyBtc, ethNtvBal)
	if err != nil {
		log.Println("ETH-BTC convert:", err)
		return nil, err
	}
	ltcAssets, err := rateSvc.Convert(coinbase.CurrencyLtc, coinbase.CurrencyBtc, ltcNtvBal)
	if err != nil {
		log.Println("LTC-BTC convert:", err)
		return nil, err
	}

	totalAssets := btcAssets + ethAssets + ltcAssets

	idealDistribution := totalAssets / 3

	ethDiff := idealDistribution - ethAssets
	ltcDiff := idealDistribution - ltcAssets

	return &distribution{
		totalAssets: totalAssets,
		ntvBtcBalance: btcNtvBal,
		ntvEthBalance: ethNtvBal,
		ntvLtcBalance: ltcNtvBal,
		curBtcAssets: btcAssets,
		curEthAssets: ethAssets,
		curLtcAssets: ltcAssets,
		diffEth: ethDiff,
		diffLtc: ltcDiff,
	}, nil
}
