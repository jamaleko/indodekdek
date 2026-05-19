package main

import (
 "encoding/json"
 "log"

 "github.com/gorilla/websocket"
)

type BinanceKline struct {
 K struct {
  Close string json:"c"
  Closed bool   json:"x"
 } json:"k"
}

func connectBinanceWS() {

 url := "wss://stream.binance.com:9443/ws/suiusdt@kline_5m"

 conn, _, err := websocket.DefaultDialer.Dial(url, nil)
 if err != nil {
  log.Fatal(err)
 }

 defer conn.Close()

 log.Println("✅ Binance WS connected")

 for {

  _, msg, err := conn.ReadMessage()
  if err != nil {
   log.Println(err)
   continue
  }

  var data BinanceKline

  err = json.Unmarshal(msg, &data)
  if err != nil {
   continue
  }

  // hanya saat candle selesai
  if data.K.Closed {

   updateCandle(data.K.Close)

  }
 }
}
