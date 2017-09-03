package reporting

import (
	"os"
	"log"
	"github.com/tobyjsullivan/btc-frogger/coinbase"
	"net/url"
	"fmt"
	"net/http"
	"encoding/json"
	"bytes"
)

const (
	dweetApiBaseUrl = "https://dweet.io:443/"
)


type ReportingSvc struct {
	dweetThingName string
	logger *log.Logger
}

func NewService(dweetThingName string) *ReportingSvc {
	svc := &ReportingSvc{
		dweetThingName: dweetThingName,
		logger: log.New(os.Stdout, "[reporting] ", 0),
	}

	return svc
}

func (svc *ReportingSvc) ReportMetrics(totalAssets, ntvBtcBal, ntvEthBal, ntvLtcBal int64, ethRate, ltcRate float64) {
	go svc.sendReport(totalAssets, ntvBtcBal, ntvEthBal, ntvLtcBal, ethRate, ltcRate)
}

func (svc *ReportingSvc) sendReport(totalAssets, ntvBtcBal, ntvEthBal, ntvLtcBal int64, ethRate, ltcRate float64) {
	dweetBody := struct{
		TotalAssets float64 `json:"totalAssets"`
		BtcBalance float64 `json:"btcBalance"`
		EthBalance float64 `json:"ethBalance"`
		LtcBalance float64 `json:"ltcBalance"`
		EthRate float64 `json:"ethRate"`
		LtcRate float64 `json:"ltcRate"`
	}{
		TotalAssets: float64(totalAssets) / float64(coinbase.AmountCoin),
		BtcBalance: float64(ntvBtcBal) / float64(coinbase.AmountCoin),
		EthBalance: float64(ntvEthBal) / float64(coinbase.AmountCoin),
		LtcBalance: float64(ntvLtcBal) / float64(coinbase.AmountCoin),
		EthRate: ethRate,
		LtcRate: ltcRate,
	}

	var body bytes.Buffer
	encoder := json.NewEncoder(&body)
	if err := encoder.Encode(&dweetBody); err != nil {
		svc.logger.Println("encode:", err)
		return
	}

	endpointUrl := dweetEndpointUrl(fmt.Sprintf("/dweet/quietly/for/%s", svc.dweetThingName), svc.logger)

	_, err := http.Post(endpointUrl, "application/json", &body)
	if err != nil {
		svc.logger.Println("post:", err)
	}
}

func dweetEndpointUrl(endpointPath string, logger *log.Logger) string {
	apiUrl, err := url.Parse(dweetApiBaseUrl)
	if err != nil {
		logger.Panicln("dweet url parse:", err)
	}

	endpointUrl := apiUrl.ResolveReference(&url.URL{Path: endpointPath})

	return endpointUrl.String()
}
