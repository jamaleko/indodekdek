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
	paperTrade		= true
	apikey			= "ini"
	secretKey		= "ini"

	symbol			= "SUIUSDT"
	//wsURL         = "wss://ws3.indodax.com/ws/"
	timeframe		= "5m"
	emaFast			= 9
	emaSlow			= 14
	emaTrend		= 10

	rsiMin			= 40
	rsiMax			= 75
	
	tpPercent       = 0.008 // +0.5% (kalau eth 0.0025 masih rugi walaupun TP)
	slPercent       = 0.004 // -0.3%
	maxDailyLoss    = 0.34
	fixedTradeLimit = 5.0

	buyFeePercent  = 0.001 // ganti sesuai fee asli
	sellFeePercent = 0.001 // ganti sesuai fee asli
	
	spreadPercent  = 0.0001
	slippagePercent = 0.0001
)

var (
	virtualBalance = 5.65
	dailyLoss      = 0.0
	currentDay     = time.Now().Day()

	inPosition = false

	entryPrice = 0.0
	tpPrice    = 0.0
	slPrice    = 0.0

	tradeAmount = fixedTradeLimit
	coinAmount  = 0.0

	lastReport time.Time
	prices []float64

	ema9 float64
	ema21 float64
	
	prevEMA9 float64
	prevEMA21 float64
	rsi float64
	ema50 float64

	currentCandle Candle
 	candleCloses []float64

	winCount  = 0
    loseCount = 0
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
func calculateRSI(period int, prices []float64) float64 {

 if len(prices) < period+1 {
  return 0
 }

 var gain float64
 var loss float64

 for i := len(prices)-period; i < len(prices); i++ {

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

  /*sendTelegram(
   fmt.Sprintf(
    "🕯️ Candle 5m\nO: %.0f\nH: %.0f\nL: %.0f\nC: %.0f",
    currentCandle.Open,
    currentCandle.High,
    currentCandle.Low,
    currentCandle.Close,
   ),
  )*/
  prevEMA9 = ema9
	prevEMA21 = ema21
	
	ema9 = calculateEMA(9, candleCloses)
	ema21 = calculateEMA(21, candleCloses)
	ema50 = calculateEMA(50, candleCloses)
	rsi = calculateRSI(14, candleCloses)
  /*sendTelegram(fmt.Sprintf(
				"📊 CHECK\nEMA9: %.0f\nEMA21: %.0f\nEMA50: %.0f\nRSI: %.2f",
				ema9,
				ema21,
				ema50,
				rsi,
			))*/

  currentCandle = Candle{}
 }
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
totalTrade := winCount + loseCount

winRate := 0.0

if totalTrade > 0 {
    winRate =
        float64(winCount) /
        float64(totalTrade) * 100
}

sendTelegram(
 fmt.Sprintf(
  "📊 Statistik Harian\n\n"+
   "✅ Win: %d\n"+
   "❌ Lose: %d\n"+
   "💰 Saldo: %.4f USDT\n"+
   "📈 Win Rate: %.1f%%",

  winCount,
  loseCount,
  virtualBalance,
  winRate,
 ),
)
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
	
	 // spread + slippage saat BUY
	 realBuyPrice := price *
	  (1 + spreadPercent + slippagePercent)
	
	 buyFee := tradeAmount * buyFeePercent
	
	 netTrade := tradeAmount - buyFee
	
	 coinAmount = netTrade / realBuyPrice
	
	 entryPrice = realBuyPrice
	
	 tpPrice = entryPrice * (1 + tpPercent)
	 slPrice = entryPrice * (1 - slPercent)
	
	 inPosition = true
	
	 sendTelegram(fmt.Sprintf(
 "🚀 BUY %s\n\nEntry: %.4f\nFee: %.4f\nTP: %.4f\nSL: %.4f\nTrade: %.2f USDT",
 symbol,
 entryPrice,
 buyFee,
 tpPrice,
 slPrice,
 tradeAmount,
))
}

func closePosition(price float64, reason string) {
	 if !inPosition {
	  return
	 }
	
	 if coinAmount <= 0 {
	  return
	 }
	 // spread + slippage saat SELL
	 realSellPrice := price *
	  (1 - spreadPercent - slippagePercent)
	
	 result := coinAmount * realSellPrice
	
	 sellFee := result * sellFeePercent
	
	 netResult := result - sellFee
	
	 pnl := netResult - tradeAmount
	
	 virtualBalance += pnl
	
	 status := reason
	 
	 /*
	sendTelegram(
 fmt.Sprintf(
  "%s\n\nExit: %.4f\nFee: %.4f\nPnL: %.4f USDT",
  status,
  realSellPrice,
  sellFee,
  pnl,
 ),
)*/
	if reason == "✅ TP HIT" {
    winCount++
}

if reason == "❌ SL HIT" {
    loseCount++
}
	sendTelegram(
 fmt.Sprintf(
  "%s\n\n"+
   "Entry: %.4f\n"+
   "Exit: %.4f\n"+
   "Fee: %.4f USDT\n"+
   "PnL: %.4f USDT\n"+
   "Saldo: %.4f USDT",

  status,
  entryPrice,
  realSellPrice,
  sellFee,
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
func connectWS() {

 for {

  log.Println("connecting Binance websocket...")

  conn, _, err := websocket.DefaultDialer.Dial(
   "wss://stream.binance.com:9443/ws/suiusdt@kline_5m",
   nil,
  )

  if err != nil {
   log.Println("dial error:", err)
   time.Sleep(5 * time.Second)
   continue
  }

  sendTelegram("🟢 Binance WebSocket connected")

  for {

   _, message, err := conn.ReadMessage()

   if err != nil {

    log.Println("read error:", err)

    sendTelegram(
     "🔴 Binance disconnected, reconnecting...",
    )

    conn.Close()

    break
   }

   resetDailyLoss()

   if !shouldTrade() {
    continue
   }

   var wsMsg BinanceKline

   err = json.Unmarshal(
    message,
    &wsMsg,
   )

   if err != nil {
    continue
   }

   // hanya candle yang selesai
   if !wsMsg.K.Closed {
    continue
   }

   var currentPrice float64

   fmt.Sscanf(
    wsMsg.K.Close,
    "%f",
    &currentPrice,
   )

   if currentPrice <= 0 {
    continue
   }

   log.Println(
    "close:",
    currentPrice,
   )

   updateCandle(currentPrice)

   if time.Since(lastReport) >= 4*time.Hour {

    changePercent :=
     ((currentPrice-entryPrice)/entryPrice)*100

    sendTelegram(
     fmt.Sprintf(
      "📊 SUI LIVE\n\nCurrent: %.4f\nEntry: %.4f\nTP: %.4f\nSL: %.4f\nPerubahan: %.3f%%",
      currentPrice,
      entryPrice,
      tpPrice,
      slPrice,
      changePercent,
     ),
    )

    lastReport = time.Now()
   }

   if !inPosition {

    if len(candleCloses) >= 50 &&
     ema9 > ema21 &&
	 ema21 > ema50 &&
	 prevEMA9 <= prevEMA21 &&
     currentPrice > ema50 &&
     rsi > 50 &&
     rsi < 65 {

     /*sendTelegram(
      fmt.Sprintf(
       "📈 BUY SIGNAL\nEMA9: %.4f\nEMA21: %.4f\nEMA50: %.4f\nRSI: %.2f",
       ema9,
       ema21,
       ema50,
       rsi,
      ),
     )*/
		 tradeAmount := getTradeAmount()
	 sendTelegram(
	    fmt.Sprintf(
	        "📈 BUY SIGNAL\n\nTrade: %.2f USDT\nEMA9: %.4f\nEMA21: %.4f\nEMA50: %.4f\nRSI: %.2f",
	        tradeAmount,
	        ema9,
	        ema21,
	        ema50,
	        rsi,
	    ),
	)

     openPosition(
      currentPrice,
     )
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
  }
 }
}

func main() {
	lastReport = time.Now()
	sendTelegram("🤖 Bot started")
	connectWS()
}
