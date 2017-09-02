package coinbase

import (
	"fmt"
	"net/http"
	"encoding/json"
	"errors"
	"strconv"
)

type Book struct {
	Bid int64
	Ask int64
}

func (c *Conn) CurrentBook(p ProductID) (*Book, error) {
	endpointUrl := getEndpointUrl(fmt.Sprintf("/products/%s/book", p))

	resp, err := c.Requester.makeRequest(http.MethodGet, endpointUrl, nil, false)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		return nil, errors.New("Unexpected status code: " + resp.Status)
	}

	// Parse the JSON response
	var jsResp struct {
		Bids [][]interface{} `json:"bids"`
		Asks [][]interface{} `json:"asks"`
	}

	decoder := json.NewDecoder(resp.Body)
	err = decoder.Decode(&jsResp)
	if err != nil {
		return nil, err
	}

	strBid := jsResp.Bids[0][0].(string)
	strAsk := jsResp.Asks[0][0].(string)

	fBid, err := strconv.ParseFloat(strBid, 64)
	if err != nil {
		return nil, err
	}

	fAsk, err := strconv.ParseFloat(strAsk, 64)
	if err != nil {
		return nil, err
	}

	bid := int64(fBid * float64(AmountCoin))
	ask := int64(fAsk * float64(AmountCoin))

	return &Book{
		Bid: bid,
		Ask: ask,
	}, nil
}
