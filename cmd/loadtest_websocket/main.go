package main

import (
	"encoding/json"
	"fmt"
	"sync/atomic"
	"time"

	"golang.org/x/net/websocket"
)

////////////////////////////////////////////////////////////////////////////////////////
// Config
////////////////////////////////////////////////////////////////////////////////////////

const (
	Concurrency = 1000
	URL         = "ws://localhost:8080/v2/websocket"
	Origin      = "http://localhost:8080"
	Request     = `{"method": "subscribePriceV1", "assets":["BTC.BTC", "ETH.ETH", "LTC.LTC"]}`
)

var Running int32 = 0
var Read int32 = 0

////////////////////////////////////////////////////////////////////////////////////////
// Spawn
////////////////////////////////////////////////////////////////////////////////////////

func Spawn() {
	ws, err := websocket.Dial(URL, "", Origin)
	if err != nil {
		fmt.Println("failed to dial websocket")
		atomic.AddInt32(&Running, -1)
		return
	}
	defer ws.Close()
	if _, err := ws.Write([]byte(Request)); err != nil {
		fmt.Println("failed to write request")
		atomic.AddInt32(&Running, -1)
		return
	}

	m := map[string]interface{}{}
	dec := json.NewDecoder(ws)
	for {
		err := dec.Decode(&m)
		if err != nil {
			fmt.Println("failed to read message")
			atomic.AddInt32(&Running, -1)
			return
		} else {
			atomic.AddInt32(&Read, 1)
		}
	}
}

////////////////////////////////////////////////////////////////////////////////////////
// Main
////////////////////////////////////////////////////////////////////////////////////////

func main() {

	for i := 1; i <= Concurrency; i++ {
		time.Sleep(1 * time.Millisecond)
		go Spawn()
		atomic.AddInt32(&Running, 1)
	}

	for {
		fmt.Println("Open Websockets: ", atomic.LoadInt32(&Running))
		fmt.Println("Read Messages: ", atomic.LoadInt32(&Read))
		time.Sleep(10 * time.Second)
	}
}
