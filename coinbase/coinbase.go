package coinbase

import (
	"net/http"
	"time"
	"net/url"
	"io"
	"bytes"
	"encoding/base64"
	"crypto/hmac"
	"crypto/sha256"
	"strconv"
	"log"
	"math/rand"
)

const (
	MAX_RETRIES = 20
	RETRY_DELAY = 60 * time.Second

	ProductID_ETH_BTC = ProductID("ETH-BTC")
	ProductID_LTC_BTC = ProductID("LTC-BTC")

	CURRENCY_ETH = Currency("ETH")
	CURRENCY_BTC = Currency("BTC")
	CURRENCY_LTC = Currency("LTC")
)

type ProductID string
type Currency string

type SignedRequester struct {
	ApiAccessKey string
	ApiSecretKey string
	ApiPassphrase string
}

func (r *SignedRequester) makeRequest(method string, urlStr string, body io.Reader, signed bool) (*http.Response, error) {
	success := false
	tries := 0
	var err error
	for !success && tries < MAX_RETRIES {
		if tries > 0 {
			log.Println("Sleeping before retry...")
			time.Sleep(RETRY_DELAY + time.Duration(rand.Intn(2000)) * time.Millisecond)
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
	timestamp := strconv.FormatInt(time.Now().UTC().Unix(), 10)

	parsedUrl, err := url.Parse(urlStr)
	if err != nil {
		return nil, err
	}

	requestPath := parsedUrl.Path

	var bodyBuf *bytes.Buffer // Can just be nil for empty bodies
	var bodyContent string
	if body != nil {
		io.Copy(bodyBuf, body)

		bodyContent = bodyBuf.String()

		bodyBuf.Reset()
		bodyBuf.WriteString(bodyContent)
	}


	req, err := http.NewRequest(method, urlStr, nil)
	if err != nil {
		return nil, err
	}

	if signed {
		secret, err := base64.StdEncoding.DecodeString(r.ApiSecretKey)
		if err != nil {
			return nil, err
		}

		sig := computeRequestSignature(timestamp, method, requestPath, bodyContent, secret)

		log.Println("Header:", "CB-ACCESS-KEY", r.ApiAccessKey)
		log.Println("Header:", "CB-ACCESS-SIGN", base64.StdEncoding.EncodeToString(sig))
		log.Println("Header:", "CB-ACCESS-TIMESTAMP", timestamp)
		log.Println("Header:", "CB-ACCESS-PASSPHRASE", r.ApiPassphrase)

		req.Header.Add("CB-ACCESS-KEY", r.ApiAccessKey)
		req.Header.Add("CB-ACCESS-SIGN", base64.StdEncoding.EncodeToString(sig))
		req.Header.Add("CB-ACCESS-TIMESTAMP", timestamp)
		req.Header.Add("CB-ACCESS-PASSPHRASE", r.ApiPassphrase)
	}

	return http.DefaultClient.Do(req)
}

func computeRequestSignature(timestamp string, method string, requestPath string, body string, secretKey []byte) []byte {
	input := timestamp + method + requestPath + body

	mac := hmac.New(sha256.New, secretKey)
	mac.Write([]byte(input))

	return mac.Sum(nil)
}
