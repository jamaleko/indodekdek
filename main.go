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
	wsURL           = "wss://ws3.indodax.com/ws/"
	tpPercent       = 0.005 // +0.5%
	slPercent       = 0.003 // -0.3%
	maxDailyLoss    = 1000.0
	fixedTradeLimit = 50000.0
)

var (
	virtualBalance = 50000.0
	dailyLoss      = 0.0
	currentDay     = time.Now().Day()

	inPosition = false

	entryPrice = 0.0
	tpPrice    = 0.0
	slPrice    = 0.0

	tradeAmount = 0.0
	coinAmount  = 0.0
)

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

func resetDailyLoss() {
	today := time.Now().Day()

	if today != currentDay {
		currentDay = today
		dailyLoss = 0

		sendTelegram("🔄 Reset rugi harian")
	}
}

func shouldTrade() bool {
	if dailyLoss >= maxDailyLoss {
		return false
	}

	return true
}

func getTradeAmount() float64 {
	if virtualBalance >= fixedTradeLimit {
		return fixedTradeLimit
	}

	return virtualBalance
}

func openPosition(price float64) {
	tradeAmount = getTradeAmount()

	if tradeAmount <= 0 {
		return
	}

	coinAmount = tradeAmount / price

	entryPrice = price
	tpPrice = entryPrice * (1 + tpPercent)
	slPrice = entryPrice * (1 - slPercent)

	inPosition = true

	sendTelegram(fmt.Sprintf(
		"🚀 BUY BTCIDR\n\nEntry: %.0f\nTP: %.0f\nSL: %.0f\nTrade: Rp%.0f\nSaldo: Rp%.0f",
		entryPrice,
		tpPrice,
		slPrice,
		tradeAmount,
		virtualBalance,
	))
}

func closePosition(price float64) {
	result := coinAmount * price
	pnl := result - tradeAmount

	virtualBalance += pnl

	status := "✅ TP HIT"

	if pnl < 0 {
		status = "❌ SL HIT"
		dailyLoss += -pnl
	}

	sendTelegram(fmt.Sprintf(
		"%s\n\nExit: %.0f\nPnL: %.0f\nSaldo: Rp%.0f\nRugi harian: Rp%.0f / Rp%.0f",
		status,
		price,
		pnl,
		virtualBalance,
		dailyLoss,
		maxDailyLoss,
	))

	inPosition = false
}

func connectWS() {
	for {
		log.Println("connecting websocket...")

		conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
		if err != nil {
			log.Println("dial error:", err)
			time.Sleep(5 * time.Second)
			continue
		}

		sendTelegram("🟢 WebSocket connected")

		// AUTH
		auth := AuthMessage{
			ID: 1,
		}

		auth.Params.Token = "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJleHAiOjE5NDY2MTg0MTV9.UR1lBM6Eqh0yWz-PVirw1uPCxe60FdchR8eNVdsskeo"

		err = conn.WriteJSON(auth)
		if err != nil {
			log.Println("auth error:", err)
			conn.Close()
			continue
		}

		time.Sleep(1 * time.Second)

		// SUBSCRIBE
		subscribe := SubscribeMessage{
			Method: 1,
			ID:     2,
		}

		subscribe.Params.Channel = "chart:tick-btcidr"

		err = conn.WriteJSON(subscribe)
		if err != nil {
			log.Println("subscribe error:", err)
			conn.Close()
			continue
		}

		for {
			_, message, err := conn.ReadMessage()
			if err != nil {
				log.Println("read error:", err)
				sendTelegram("🔴 WebSocket disconnected, reconnecting...")
				conn.Close()
				break
			}

			log.Println(string(message))

			resetDailyLoss()

			if !shouldTrade() {
				continue
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

			priceFloat, ok := last[2].(float64)
			if !ok {
				continue
			}

			currentPrice := priceFloat

			log.Println("price:", currentPrice)

			if !inPosition {
				openPosition(currentPrice)
				continue
			}

			if currentPrice >= tpPrice {
				closePosition(currentPrice)
				continue
			}

			if currentPrice <= slPrice {
				closePosition(currentPrice)
				continue
			}
		}
	}
}

func main() {
	sendTelegram("🤖 Bot started")
	connectWS()
}
