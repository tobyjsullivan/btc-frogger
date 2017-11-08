package reporting

import (
	"os"
	"log"
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

type Report struct {
	TotalAssets float64 `json:"totalAssets"`
	AssetValueUsd float64 `json:"assetValueUsd"`
	BtcBalance float64 `json:"btcBalance"`
	EthBalance float64 `json:"ethBalance"`
	LtcBalance float64 `json:"ltcBalance"`
	EthRate float64 `json:"ethRate"`
	LtcRate float64 `json:"ltcRate"`
	UsdRate float64 `json:"usdRate"`
}

func (svc *ReportingSvc) ReportMetrics(report *Report) {
	go svc.sendReport(report)
}

func (svc *ReportingSvc) sendReport(dweetBody *Report) {
	var body bytes.Buffer
	encoder := json.NewEncoder(&body)
	if err := encoder.Encode(&dweetBody); err != nil {
		svc.logger.Println("encode:", err)
		return
	}

	svc.logger.Printf("Metrics sent to %s", svc.dweetThingName)
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
