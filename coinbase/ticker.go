package coinbase

import (
	"net/http"
	"fmt"
	"errors"
	"encoding/json"
	"strconv"
	"time"
)

var (
	cache map[ProductID]*Ticker = make(map[ProductID]*Ticker)
	cacheExpiry map[ProductID]time.Time = make(map[ProductID]time.Time)
)

type Ticker struct {
	TradeID int
	Price float64
	Size float64
	Bid float64
	Ask float64
	Volume float64
	Time string
}

func (c *Conn) CurrentTicker(p ProductID) (*Ticker, error) {
	// Check the cache
	exp, ok := cacheExpiry[p]
	if ok && exp.After(time.Now()) {
		if cached, ok := cache[p]; ok {
			return cached, nil
		}
	}


	url := fmt.Sprintf("https://api.gdax.com/products/%s/ticker", p)

	resp, err := c.Requester.makeRequest(http.MethodGet, url, nil, false)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		return nil, errors.New("Unexpected status code: "+resp.Status)
	}

	// Parse the JSON response
	var jsResp struct {
		TradeID int `json:"trade_id"`
		Price string `json:"price"`
		Size string `json:"size"`
		Bid string `json:"bid"`
		Ask string `json:"ask"`
		Volume string `json:"volume"`
		Time string `json:"time"`
	}

	decoder := json.NewDecoder(resp.Body)
	err = decoder.Decode(&jsResp)
	if err != nil {
		return nil, err
	}

	price, err := strconv.ParseFloat(jsResp.Price, 64)
	if err != nil {
		return nil, err
	}

	size, err := strconv.ParseFloat(jsResp.Size, 64)
	if err != nil {
		return nil, err
	}

	bid, err := strconv.ParseFloat(jsResp.Bid, 64)
	if err != nil {
		return nil, err
	}

	ask, err := strconv.ParseFloat(jsResp.Ask, 64)
	if err != nil {
		return nil, err
	}

	volume, err := strconv.ParseFloat(jsResp.Volume, 64)
	if err != nil {
		return nil, err
	}

	ticker := &Ticker{
		TradeID: jsResp.TradeID,
		Price: price,
		Size: size,
		Bid: bid,
		Ask: ask,
		Volume: volume,
		Time: jsResp.Time,
	}

	cache[p] = ticker
	cacheExpiry[p] = time.Now().Add(1 * time.Second)

	return ticker, nil
}
