package coinbase

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"log"
	"math/rand"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

const (
	gdaxApiBaseUrl = "https://api.gdax.com"

	maxRequestRetries = 1
	requestRetryDelay = 60 * time.Second

	ProductEthBtc = ProductID("ETH-BTC")
	ProductLtcBtc = ProductID("LTC-BTC")
	ProductBtcUsd = ProductID("BTC-USD")

	CurrencyEth = Currency("ETH")
	CurrencyBtc = Currency("BTC")
	CurrencyLtc = Currency("LTC")
	CurrencyUsd = Currency("USD")

	QuoteIncrement = 1000
)

type ProductID string
type Currency string

type SignedRequester struct {
	ApiAccessKey  string
	ApiSecretKey  string
	ApiPassphrase string
}

func getEndpointUrl(path string) string {
	u, err := url.Parse(gdaxApiBaseUrl)
	if err != nil {
		log.Panicln("url.Parse:", err)
	}
	return u.ResolveReference(&url.URL{Path: path}).String()
}

func (r *SignedRequester) makeRequest(method string, urlStr string, body io.Reader, signed bool) (*http.Response, error) {
	success := false
	tries := 0
	var err error
	for !success && tries < maxRequestRetries {
		if tries > 0 {
			log.Println("Sleeping before retry...")
			time.Sleep(requestRetryDelay + time.Duration(rand.Intn(2000))*time.Millisecond)
			log.Println("Retrying...")
		}

		var resp *http.Response
		resp, err = r.doMakeRequest(method, urlStr, body, signed)
		if err != nil {
			tries++
			log.Println("request failed:", err)
			continue
		}

		return resp, nil
	}

	return nil, err
}

func (r *SignedRequester) doMakeRequest(method string, urlStr string, body io.Reader, signed bool) (*http.Response, error) {
	parsedUrl, err := url.Parse(urlStr)
	if err != nil {
		return nil, err
	}

	requestPath := parsedUrl.Path

	var bodyBuf bytes.Buffer // Can just be nil for empty bodies
	var bodyContent string
	if body != nil {
		var buf0 bytes.Buffer
		io.Copy(&buf0, body)

		bodyContent = buf0.String()

		bodyBuf.WriteString(bodyContent)
	}

	req, err := http.NewRequest(method, urlStr, &bodyBuf)
	if err != nil {
		return nil, err
	}

	timestamp := strconv.FormatInt(time.Now().UTC().Unix(), 10)

	if signed {
		secret, err := base64.StdEncoding.DecodeString(r.ApiSecretKey)
		if err != nil {
			return nil, err
		}

		sig := ComputeRequestSignature(timestamp, method, requestPath, bodyContent, secret)

		req.Header.Add("Content-Type", "application/json")
		req.Header.Add("CB-ACCESS-KEY", r.ApiAccessKey)
		req.Header.Add("CB-ACCESS-SIGN", base64.StdEncoding.EncodeToString(sig))
		req.Header.Add("CB-ACCESS-TIMESTAMP", timestamp)
		req.Header.Add("CB-ACCESS-PASSPHRASE", r.ApiPassphrase)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		log.Println("request error:", resp.StatusCode)
		var errResp struct {
			Message string `json:"message"`
		}
		decoder := json.NewDecoder(resp.Body)
		err = decoder.Decode(&errResp)
		if err != nil {
			log.Println("Error decoding error response.", err)
			return nil, err
		}

		log.Println("Error message:", errResp.Message)
		return resp, errors.New("request error: " + errResp.Message)
	}

	return resp, nil
}

func ComputeRequestSignature(timestamp string, method string, requestPath string, body string, secretKey []byte) []byte {
	mac := hmac.New(sha256.New, secretKey)
	mac.Write([]byte(timestamp))
	mac.Write([]byte(method))
	mac.Write([]byte(requestPath))
	mac.Write([]byte(body))

	return mac.Sum(nil)
}
