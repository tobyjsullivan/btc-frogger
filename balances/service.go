package balances

import (
	"context"
	"log"
	"os"
	"time"

	"github.com/tobyjsullivan/btc-frogger/coinbase"
)

const (
	loopDuration = 3 * time.Second
)

type BalanceSvc struct {
	conn        *coinbase.Conn
	ntvBalances map[coinbase.Currency]int64
	logger      *log.Logger
}

func NewService(ctx context.Context, conn *coinbase.Conn) *BalanceSvc {
	svc := &BalanceSvc{
		conn:        conn,
		ntvBalances: make(map[coinbase.Currency]int64),
		logger:      log.New(os.Stdout, "[balances] ", 0),
	}

	go svc.loop(ctx)

	return svc
}

func (svc *BalanceSvc) GetNativeBalance(c coinbase.Currency) (int64, bool) {
	val, ok := svc.ntvBalances[c]

	return val, ok
}

func (svc *BalanceSvc) loop(ctx context.Context) {
	ticker := time.NewTicker(loopDuration)

	for {
		select {
		case <-ticker.C:
			svc.updateBalances()
		case <-ctx.Done():
			return
		}
	}
}

func (svc *BalanceSvc) updateBalances() error {
	accounts, err := svc.conn.GetAccounts()
	if err != nil {
		log.Println("getAccounts:", err)
		return err
	}

	for _, acct := range accounts {
		svc.ntvBalances[acct.Currency] = acct.Balance
	}

	return nil
}
