package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/gorilla/websocket"
)

const (
	wsURL = "wss://ws3.indodax.com/ws/"

	// TP / SL
	tpPercent = 0.0025 // +0.5%
	slPercent = 0.0025 // -0.3%

	// trading
	fixedTradeLimit = 10000.0

	// fee
	buyFeePercent  = 0.002111
	sellFeePercent = 0.004211

	// simulasi spread/slippage
	spreadPercent   = 0.0001
	slippagePercent = 0.0001
)

var (
	virtualBalance = 13825.0

	inPosition = false

	entryPrice = 0.0
	tpPrice    = 0.0
	slPrice    = 0.0

	tradeAmount = 0.0
	coinAmount  = 0.0

	ema9  float64
	ema21 float64
	ema50 float64
	rsi   float64

	currentCandle Candle
	candleCloses  []float64

	lastReport time.Time
)

type Candle struct {
	Open  float64
	High  float64
	Low   float64
	Close float64
	Time  time.Time
}

type AuthMessage struct {
	Params struct {
		Token string `json:"token"`
	} `json:"params"`
	ID int `json:"id"`
}

type SubscribeMessage struct {
	Method int `json:"method"`
	Params struct {
		Channel string `json:"channel"`
	} `json:"params"`
	ID int `json:"id"`
}

type WSMessage struct {
	Result struct {
		Channel string `json:"channel"`
		Data struct {
			Data [][]interface{} `json:"data"`
		} `json:"data"`
	} `json:"result"`
}

func calculateEMA(period int, prices []float64) float64 {

	if len(prices) < period {
		return 0
	}

	k := 2.0 / float64(period+1)

	ema := prices[0]

	for i := 1; i < len(prices); i++ {
		ema = (prices[i] * k) + (ema * (1 - k))
	}

	return ema
}

func calculateRSI(period int, prices []float64) float64 {

	if len(prices) < period+1 {
		return 0
	}

	var gain float64
	var loss float64

	for i := len(prices) - period; i < len(prices); i++ {

		diff := prices[i] - prices[i-1]

		if diff > 0 {
			gain += diff
		} else {
			loss += -diff
		}
	}

	if loss == 0 {
		return 100
	}

	rs := gain / loss

	return 100 - (100 / (1 + rs))
}

func sendTelegram(message string) {

	token := os.Getenv("BOT_TOKEN")
	chatID := os.Getenv("CHAT_ID")

	if token == "" || chatID == "" {
		log.Println(message)
		return
	}

	url := fmt.Sprintf(
		"https://api.telegram.org/bot%s/sendMessage",
		token,
	)

	payload := map[string]string{
		"chat_id": chatID,
		"text":    message,
	}

	body, _ := json.Marshal(payload)

	_, err := http.Post(
		url,
		"application/json",
		bytes.NewBuffer(body),
	)

	if err != nil {
		log.Println("telegram error:", err)
	}
}

func openPosition(price float64) {

	if virtualBalance < 10000 {

		sendTelegram(
			fmt.Sprintf(
				"🛑 BOT STOP\nSaldo: Rp%.0f",
				virtualBalance,
			),
		)

		return
	}

	tradeAmount = fixedTradeLimit

	if tradeAmount > virtualBalance {
		tradeAmount = virtualBalance
	}

	realBuyPrice := price *
		(1 + spreadPercent + slippagePercent)

	buyFee := tradeAmount * buyFeePercent

	netTrade := tradeAmount - buyFee

	coinAmount = netTrade / realBuyPrice

	entryPrice = realBuyPrice

	tpPrice = entryPrice * (1 + tpPercent)
	slPrice = entryPrice * (1 - slPercent)

	inPosition = true

	sendTelegram(
		fmt.Sprintf(
			"🚀 BUY ETHIDR\n\nEntry: %.0f\nTP: %.0f\nSL: %.0f\nRSI: %.2f",
			entryPrice,
			tpPrice,
			slPrice,
			rsi,
		),
	)
}

func closePosition(price float64, reason string) {

	if !inPosition {
		return
	}

	realSellPrice := price *
		(1 - spreadPercent - slippagePercent)

	result := coinAmount * realSellPrice

	sellFee := result * sellFeePercent

	netResult := result - sellFee

	pnl := netResult - tradeAmount

	virtualBalance += pnl

	sendTelegram(
		fmt.Sprintf(
			"%s\n\nExit: %.0f\nPnL: %.0f\nSaldo: Rp%.0f",
			reason,
			realSellPrice,
			pnl,
			virtualBalance,
		),
	)

	inPosition = false

	entryPrice = 0
	tpPrice = 0
	slPrice = 0
	coinAmount = 0
	tradeAmount = 0
}

func updateCandle(currentPrice float64) {

	if currentCandle.Open == 0 {

		currentCandle.Open = currentPrice
		currentCandle.High = currentPrice
		currentCandle.Low = currentPrice
		currentCandle.Close = currentPrice
		currentCandle.Time = time.Now()

		return
	}

	if currentPrice > currentCandle.High {
		currentCandle.High = currentPrice
	}

	if currentPrice < currentCandle.Low {
		currentCandle.Low = currentPrice
	}

	currentCandle.Close = currentPrice

	if time.Since(currentCandle.Time) >= 5*time.Minute {

		candleCloses = append(
			candleCloses,
			currentCandle.Close,
		)

		if len(candleCloses) > 100 {
			candleCloses = candleCloses[1:]
		}

		ema9 = calculateEMA(9, candleCloses)
		ema21 = calculateEMA(21, candleCloses)

		// versi cepat
		ema50 = calculateEMA(10, candleCloses)

		rsi = calculateRSI(14, candleCloses)

		sendTelegram(
			fmt.Sprintf(
				"📊 CHECK\nCandle: %d\nEMA9: %.0f\nEMA21: %.0f\nEMA10: %.0f\nRSI: %.2f",
				len(candleCloses),
				ema9,
				ema21,
				ema50,
				rsi,
			),
		)

		currentCandle = Candle{}
	}
}

func connectWS() {

	for {

		log.Println("connecting websocket...")

		conn, _, err := websocket.DefaultDialer.Dial(
			wsURL,
			nil,
		)

		if err != nil {
			log.Println(err)
			time.Sleep(5 * time.Second)
			continue
		}

		sendTelegram("🟢 WebSocket connected")

		auth := AuthMessage{
			ID: 1,
		}

		auth.Params.Token = "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJleHAiOjE5NDY2MTg0MTV9.UR1lBM6Eqh0yWz-PVirw1uPCxe60FdchR8eNVdsskeo"

		conn.WriteJSON(auth)

		time.Sleep(1 * time.Second)

		subscribe := SubscribeMessage{
			Method: 1,
			ID:     2,
		}

		subscribe.Params.Channel = "chart:tick-ethidr"

		conn.WriteJSON(subscribe)

		for {

			_, message, err := conn.ReadMessage()

			if err != nil {

				sendTelegram(
					"🔴 WebSocket disconnected",
				)

				conn.Close()

				break
			}

			var wsMsg WSMessage

			err = json.Unmarshal(message, &wsMsg)

			if err != nil {
				continue
			}

			rows := wsMsg.Result.Data.Data

			if len(rows) == 0 {
				continue
			}

			last := rows[len(rows)-1]

			if len(last) < 3 {
				continue
			}

			priceRaw := last[2]

			var currentPrice float64

			switch v := priceRaw.(type) {

			case float64:
				currentPrice = v

			case string:
				fmt.Sscanf(v, "%f", &currentPrice)

			default:
				continue
			}

			if currentPrice <= 0 {
				continue
			}

			updateCandle(currentPrice)

			// versi cepat:
			// mulai trading setelah 21 candle
			if !inPosition {

				if len(candleCloses) >= 21 &&
					ema9 > ema21 &&
					currentPrice > ema50 &&
					rsi > 45 &&
					rsi < 70 {

					sendTelegram("📈 BUY SIGNAL")

					openPosition(currentPrice)
				}
			}

			if inPosition {

				if currentPrice >= tpPrice {

					closePosition(
						currentPrice,
						"✅ TP HIT",
					)

					continue
				}

				if currentPrice <= slPrice {

					closePosition(
						currentPrice,
						"❌ SL HIT",
					)

					continue
				}
			}

			if time.Since(lastReport) >= 4*time.Hour {

				changePercent :=
					((currentPrice - entryPrice) / entryPrice) * 100

				sendTelegram(
					fmt.Sprintf(
						"📊 ETH LIVE\nCurrent: %.0f\nPerubahan: %.2f%%",
						currentPrice,
						changePercent,
					),
				)

				lastReport = time.Now()
			}
		}
	}
}

func main() {

	lastReport = time.Now()

	sendTelegram("🤖 Bot started")

	connectWS()
}
