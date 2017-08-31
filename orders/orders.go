package orders


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
)

const (
	PRODUCT_ID_ETH_BTC = "ETH-BTC"
)

const (
	SIDE_BUY = iota
	SIDE_SELL = iota
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

	// Send a subscribe message
	logger.Println("Generating subscribe message...")
	subscribeMsg := struct {
		Type string `json:"type"`
		ProductIDs []string `json:"product_ids"`
	}{
		Type: "subscribe",
		ProductIDs: []string{PRODUCT_ID_ETH_BTC},
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
				logger.Printf("Received message: %s\n", message)
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
				msgType := struct {
					Type string `json:"type"`
				}{}
				err := json.Unmarshal([]byte(msg), &msgType)
				if err != nil {
					logger.Println("type unmarshal:", err)
					return
				}

				logger.Println("Message received:", msgType.Type)
				switch msgType.Type {
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
					}

					switch openMsg.Side {
					case SIDE_BUY:
						switch openMsg.ProductID {
						case PRODUCT_ID_ETH_BTC:
							ethOrderBook.submitBuyOrder(order)
						default:
							logger.Println("Unknown product ID:", openMsg.ProductID)
							return
						}
					case SIDE_SELL:
						switch openMsg.ProductID {
						case PRODUCT_ID_ETH_BTC:
							ethOrderBook.submitSellOrder(order)
						default:
							logger.Println("Unknown product ID:", openMsg.ProductID)
							return
						}
					}

				}
				logger.Printf("ETH ORDER BOOK: %d BUYS; %d SELLS\n", ethOrderBook.numBuyOrders(), ethOrderBook.numSellOrders())

			}
		}
	}(messageQueue, done, logger)

	select {
	case <-done:
		logger.Println("Done. Goodbye.")
	}
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

	var side uint = 0
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
	Side uint
	Price float64
	RemainingSize float64
}

type orderBook struct {
	mx sync.Mutex
	BuyOrders map[uuid.UUID]*order
	SellOrders map[uuid.UUID]*order
}

func newOrderBook() *orderBook {
	return &orderBook{
		BuyOrders: make(map[uuid.UUID]*order),
		SellOrders: make(map[uuid.UUID]*order),
	}
}

func (b *orderBook) submitBuyOrder(order *order) error {
	b.mx.Lock()
	defer b.mx.Unlock()

	b.BuyOrders[order.OrderID] = order

	return nil
}

func (b *orderBook) submitSellOrder(order *order) error {
	b.mx.Lock()
	defer b.mx.Unlock()

	b.SellOrders[order.OrderID] = order

	return nil
}

func (b *orderBook) numBuyOrders() int {
	return len(b.BuyOrders)
}

func (b *orderBook) numSellOrders() int {
	return len(b.SellOrders)
}

type order struct {
	OrderID uuid.UUID
	Sequence uint64
	Price float64
	RemainingSize float64
}

