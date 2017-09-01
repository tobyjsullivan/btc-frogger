package coinbase

import (
	"net/http"
	"encoding/json"
	"log"
	"github.com/satori/go.uuid"
	"strconv"
)

const (
	AmountCoin = 100000000
)

func (conn *Conn) GetAccounts() ([]*Account, error) {
	endpoingUrl := getEndpointUrl("/accounts")
	resp, err := conn.Requester.makeRequest(http.MethodGet, endpoingUrl, nil, true)
	if err != nil {
		log.Println("signed request:", err)
		return []*Account{}, err
	}

	var accountsResp []struct{
		ID string `json:"id"`
		Currency string `json:"currency"`
		Balance string `json:"balance"`
		Available string `json:"available"`
	}
	decoder := json.NewDecoder(resp.Body)
	err = decoder.Decode(&accountsResp)

	out := []*Account{}
	for _, acct := range accountsResp {
		id, err := uuid.FromString(acct.ID)
		if err != nil {
			return []*Account{}, err
		}

		fBalance, err := strconv.ParseFloat(acct.Balance, 64)
		if err != nil {
			return []*Account{}, err
		}

		balance := int64(fBalance * float64(AmountCoin))

		out = append(out, &Account{
			ID: id,
			Currency: Currency(acct.Currency),
			Balance: balance,
		})
	}

	return out, nil
}

type Account struct {
	ID uuid.UUID
	Currency Currency
	Balance int64
}
