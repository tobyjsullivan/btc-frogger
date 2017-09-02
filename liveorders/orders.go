package liveorders


import (
	"log"
	"os"
	"github.com/gorilla/websocket"
	"net/url"
	"encoding/json"
	"github.com/satori/go.uuid"
	"strconv"
	"errors"
	"sync"
	"time"
	"net/http"
	"encoding/base64"
	"github.com/tobyjsullivan/btc-frogger/coinbase"
)

const (
	PRODUCT_ID_ETH_BTC = "ETH-BTC"
)

const (
	SIDE_BUY = "buy"
	SIDE_SELL = "sell"
)

func RunOrders() {
	// Create a logger
	logger := log.New(os.Stdout, "[orders] ", 0)
	logger.Println("Logger instantiated.")

	// Initialize order book
	logger.Println("Creating order books...")
	ethOrderBook := newOrderBook()
	logger.Println("Order book created.")

	// Connect to GDAX
	logger.Println("Connecting to GDAX...")
	url, err := url.Parse("wss://ws-feed.gdax.com")
	if err != nil {
		logger.Fatalln("parse:", err)
	}
	logger.Println("url:", url.String())

	conn, r, err := websocket.DefaultDialer.Dial(url.String(), nil)
	if err != nil {
		logger.Fatalln("dial:", err)
	}
	defer conn.Close()
	logger.Println("Connected:", r.Status)

	// Compute sig for authenticated subscribe
	timestamp := strconv.FormatInt(time.Now().UTC().Unix(), 10)
	method := http.MethodGet
	path := "/users/self"
	accessKey := os.Getenv("COINBASE_API_ACCESS_KEY")
	passphrase := os.Getenv("COINBASE_API_PASSPHRASE")
	bSecretKey, err := base64.StdEncoding.DecodeString(os.Getenv("COINBASE_API_SECRET_KEY"))
	if err != nil {
		logger.Panicln("decode:", err)
	}
	signature := coinbase.ComputeRequestSignature(timestamp, method, path, "", bSecretKey)

	// Send a subscribe message
	logger.Println("Generating subscribe message...")
	subscribeMsg := struct {
		Type string `json:"type"`
		ProductIDs []string `json:"product_ids"`
		Signature string `json:"signature"`
		Key string `json:"key"`
		Passphrase string `json:"passphrase"`
		Timestamp string `json:"timestamp"`
	}{
		Type: "subscribe",
		ProductIDs: []string{PRODUCT_ID_ETH_BTC},
		Signature: base64.StdEncoding.EncodeToString(signature),
		Key: accessKey,
		Passphrase: passphrase,
		Timestamp: timestamp,
	}
	msg, err := json.Marshal(&subscribeMsg)
	if err != nil {
		logger.Fatalln("marshal:", err)
	}

	logger.Printf("Sending subscribe message: %s\n", msg)
	err = conn.WriteMessage(websocket.TextMessage, msg)
	if err != nil {
		logger.Fatalln("write:", err)
	}
	logger.Println("Subscribe message sent.")

	// Print order count
	go func() {
		ticker := time.Tick(100 * time.Millisecond)
		for range ticker {
			logger.Printf("ETH ORDER BOOK: %d open orders\n", ethOrderBook.numOrders())
		}
	}()

	// Print high buy order
	go func() {
		ticker := time.Tick(300 * time.Millisecond)
		for range ticker {
			high := ethOrderBook.highBuy()

			logger.Printf("HIGH BUY: %.8f\n", high)
		}
	}()

	// Print low sell order
	go func() {
		ticker := time.Tick(300 * time.Millisecond)
		for range ticker {
			high := ethOrderBook.lowSell()

			logger.Printf("LOW SELL: %.8f\n", high)
		}
	}()

	// Read incoming messages
	done := make(chan struct{})
	messageQueue := make(chan string, 2000)
	go func(c *websocket.Conn, queue chan string, done chan struct{}, logger *log.Logger) {
		defer close(done)
		logger.Println("Checking for messages...")

		for {
			t, message, err := c.ReadMessage()
			if err != nil {
				logger.Println("read:", err)
				return
			}

			switch t {
			case websocket.TextMessage:
				//logger.Printf("Received message: %s\n", message)
				queue <- string(message)
			case websocket.BinaryMessage:
				logger.Println("Received a binary message...")
				return
			default:
				logger.Println("Unexpected case.", t)
				return
			}
		}
	}(conn, messageQueue, done, logger)

	// Parse messages and update stats
	go func(queue chan string, done chan struct{}, logger *log.Logger) {
		defer close(done)
		for {
			select {
			case msg := <-queue:
				go processMessage(ethOrderBook, msg, logger)
			}
		}
	}(messageQueue, done, logger)

	select {
	case <-done:
		logger.Println("Done. Goodbye.")
	}
}

func processMessage(ethOrderBook *orderBook, msg string, logger *log.Logger) {
	parsedMsg := struct {
		Type string `json:"type"`
		//OrderId string `json:"order_id"`
		//ClientOID string `json:"client_oid"`
		//Side string `json:"side"`
		//Reason string `json:"reason"`
		//ProductID string `json:"product_id"`
		//Price string `json:"price"`
		//RemainingSize string `json:"remaining_size"`
		//Size string `json:"size"`
		//Sequence int `json:"sequence"`
		//Time string `json:"time"`
		//OrderType string `json:"order_type"`

	}{}
	err := json.Unmarshal([]byte(msg), &parsedMsg)
	if err != nil {
		logger.Println("type unmarshal:", err)
		return
	}

	//logger.Println("Message received:", parsedMsg.Type)
	switch parsedMsg.Type {
	case "open":
		openMsg, err := parseOpenMessage(msg, logger)
		if err != nil {
			logger.Println("parse open:", err)
			return
		}

		order := &order{
			OrderID: openMsg.OrderID,
			Sequence: openMsg.Sequence,
			Price: openMsg.Price,
			RemainingSize: openMsg.RemainingSize,
			Side: openMsg.Side,
		}

		switch openMsg.ProductID {
		case PRODUCT_ID_ETH_BTC:
			ethOrderBook.openOrder(order)
		default:
			logger.Println("Unknown product ID:", openMsg.ProductID)
			return
		}
	case "done":
		doneMsg, err := parseDoneMessage(msg, logger)
		if err != nil {
			logger.Println("parse done:", err)
			return
		}

		ethOrderBook.closeOrder(doneMsg.OrderID)
	case "match":
		matchMsg, err := parseMatchMessage(msg, logger)
		if err != nil {
			logger.Println("parse match:", err)
			return
		}
		ethOrderBook.closeOrder(matchMsg.MakerOrderID)
	case "received":
		// IGNORE
	default:
		logger.Println("Message not processed:", parsedMsg.Type)
	}

}

func parseDoneMessage(msg string, logger *log.Logger) (*doneMsg, error) {
	parsed := struct {
		OrderID string `json:"order_id"`
	}{}

	err := json.Unmarshal([]byte(msg), &parsed)
	if err != nil {
		logger.Println("open unmarshal:", err)
		return nil, err
	}

	orderId, err := uuid.FromString(parsed.OrderID)
	if err != nil {
		logger.Println("orderId:", err)
		return nil, err
	}

	return &doneMsg{
		OrderID: orderId,
	}, nil
}

func parseMatchMessage(msg string, logger *log.Logger) (*matchMsg, error) {
	parsed := struct {
		MakerOrderId string `json:"maker_order_id"`
	}{}

	err := json.Unmarshal([]byte(msg), &parsed)
	if err != nil {
		logger.Println("open unmarshal:", err)
		return nil, err
	}

	makerOrderId, err := uuid.FromString(parsed.MakerOrderId)
	if err != nil {
		logger.Println("orderId:", err)
		return nil, err
	}

	return &matchMsg{
		MakerOrderID: makerOrderId,
	}, nil
}

func parseOpenMessage(msg string, logger *log.Logger) (*openMsg, error) {
	parsed := struct {
		Side string `json:"side"`
		Price string `json:"price"`
		OrderID string `json:"order_id"`
		RemainingSize string `json:"remaining_size"`
		ProductID string `json:"product_id"`
		Sequence uint64 `json:"sequence"`
		Time string `json:"time"`
	}{}

	err := json.Unmarshal([]byte(msg), &parsed)
	if err != nil {
		logger.Println("open unmarshal:", err)
		return nil, err
	}

	orderId, err := uuid.FromString(parsed.OrderID)
	if err != nil {
		logger.Println("orderId:", err)
		return nil, err
	}

	price, err := strconv.ParseFloat(parsed.Price, 64)
	if err != nil {
		logger.Println("parse price:", err)
		return nil, err
	}

	size, err := strconv.ParseFloat(parsed.Price, 64)
	if err != nil {
		logger.Println("parse size:", err)
		return nil, err
	}

	var side string
	switch parsed.Side {
	case "buy":
		side = SIDE_BUY
	case "sell":
		side = SIDE_SELL
	default:
		logger.Println("side: unknown side:", parsed.Side)
		return nil, errors.New("Unknown side: "+ parsed.Side)
	}

	return &openMsg{
		OrderID: orderId,
		Sequence: parsed.Sequence,
		ProductID: parsed.ProductID,
		Side: side,
		Price: price,
		RemainingSize: size,
	}, nil
}

type openMsg struct {
	OrderID uuid.UUID
	Sequence uint64
	ProductID string
	Side string
	Price float64
	RemainingSize float64
}

type doneMsg struct {
	OrderID uuid.UUID
}

type matchMsg struct {
	MakerOrderID uuid.UUID
}

type orderBook struct {
	mx sync.Mutex
	OpenOrders map[uuid.UUID]*order
}

func newOrderBook() *orderBook {
	return &orderBook{
		OpenOrders: make(map[uuid.UUID]*order),
	}
}

func (b *orderBook) openOrder(order *order) {
	b.mx.Lock()
	defer b.mx.Unlock()

	b.OpenOrders[order.OrderID] = order
}

func (b *orderBook) closeOrder(orderId uuid.UUID) {
	b.mx.Lock()
	defer b.mx.Unlock()

	delete(b.OpenOrders, orderId)
}

func (b *orderBook) numOrders() int {
	return len(b.OpenOrders)
}

func (b *orderBook) highBuy() float64 {
	var high float64
	for _, o := range b.OpenOrders {
		if o.Side != SIDE_BUY {
			continue
		}

		if o.Price > high {
			high = o.Price
		}
	}

	return high
}

func (b *orderBook) lowSell() float64 {
	var low float64
	for _, o := range b.OpenOrders {
		if o.Side != SIDE_SELL {
			continue
		}

		if o.Price < low {
			low = o.Price
		}
	}

	return low
}

type order struct {
	OrderID uuid.UUID
	Sequence uint64
	Price float64
	RemainingSize float64
	Side string
}

