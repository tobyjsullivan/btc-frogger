package coinbase

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
)

const (
	SideBuy  = OrderSide("buy")
	SideSell = OrderSide("sell")
)

type OrderSide string

func (conn *Conn) PlaceOrder(c Currency, side OrderSide, amountNative int64, price int64) error {
	log.Printf("PLACE ORDER: %s %s %s", side, fmtAmount(amountNative), c)

	var productId ProductID
	switch c {
	case CurrencyEth:
		productId = ProductEthBtc
	case CurrencyLtc:
		productId = ProductLtcBtc
	default:
		return errors.New(fmt.Sprintf("Unexpected currency: %s", c))
	}

	// Price must be a rounded multiple of QuoteIncrement or Coinbase will resp w/ BAD_REQUEST
	if price % QuoteIncrement != 0 {
		rndUp := (price % QuoteIncrement) >= (QuoteIncrement / 2)
		price = (price / QuoteIncrement) * QuoteIncrement
		if rndUp {
			price += QuoteIncrement
		}
	}

	reqBody := struct {
		Price     string `json:"price"`
		Size      string `json:"size"`
		Side      string `json:"side"`
		Type      string `json:"type"`
		ProductID string `json:"product_id"`
		PostOnly  bool   `json:"post_only"`
	}{
		Price:     fmt.Sprintf("%.8f", float64(price)/AmountCoin),
		Size:      fmt.Sprintf("%.8f", float64(amountNative)/AmountCoin),
		Side:      string(side),
		Type:      "limit",
		ProductID: string(productId),
		PostOnly:  true,
	}

	reqJs, err := json.Marshal(&reqBody)
	if err != nil {
		return err
	}
	endpointUrl := getEndpointUrl("/orders")

	log.Println("Order request:", endpointUrl, string(reqJs))

	var buf bytes.Buffer
	buf.WriteString(string(reqJs))

	resp, err := conn.Requester.makeRequest(http.MethodPost, endpointUrl, &buf, true)
	if err != nil {
		log.Println("order:", err)
		return err
	}

	var orderResp struct {
		RejectReason string `json:"reject_reason,omitempty"`
		Status       string `json:"status"`
	}
	decoder := json.NewDecoder(resp.Body)
	err = decoder.Decode(&orderResp)

	if orderResp.Status == "rejected" {
		log.Println("order rejected:", orderResp.RejectReason)
		return nil // Return peacefully so we can try again next time
	}

	log.Println("order resp:", resp.Status, orderResp.Status)

	return nil
}

func (conn *Conn) CancelAllOrders() error {
	endpointUrl := getEndpointUrl("/orders")

	log.Println("Cancelling all oper orders")

	resp, err := conn.Requester.makeRequest(http.MethodDelete, endpointUrl, nil, true)
	if err != nil {
		log.Println("cancel orders:", err)
		return err
	}

	log.Println("Cancel resp:", resp.Status)

	return nil
}

func fmtAmount(amount int64) string {
	return fmt.Sprintf("%.8f", float64(amount)/AmountCoin)
}
