package main

import (
	"fmt"
	"github.com/gorilla/websocket"
	"go-relay/cmd/conf"
	"net/http"
	"runtime"
)

var (
	upgrader = websocket.Upgrader{
		ReadBufferSize:  conf.ReadBufferSize,
		WriteBufferSize: conf.WriteBufferSize,
	}
)

func main() {
	// Connect to Source
	senderWS, _, _ := websocket.DefaultDialer.Dial("ws://localhost:8080/sender", nil)
	defer senderWS.Close()

	var receiverWS *websocket.Conn

	messageChan := make(chan []byte, conf.MessageChanSize)

	// Forward messages from Source to Dest
	go func() {
		if conf.LockOSThread {
			runtime.LockOSThread()
			defer runtime.UnlockOSThread()
		}

		for {
			select {
			case msg := <-messageChan:
				if receiverWS == nil {
					continue
				}
				
				receiverWS.WriteMessage(websocket.BinaryMessage, msg)
			default:
				if conf.UseGosched {
					runtime.Gosched()
				}
			}
		}
	}()

	// Accept Dest connections
	http.HandleFunc("/relay", func(w http.ResponseWriter, r *http.Request) {
		receiverWS, _ = upgrader.Upgrade(w, r, nil)
		defer receiverWS.Close()

		// Read message, add to channel
		for {
			_, msg, err := senderWS.ReadMessage()
			if err != nil {
				break
			}

			select {
			case messageChan <- msg:
			default:
				fmt.Println("relay chan full")
			}

		}
	})
	http.ListenAndServe(":8081", nil)
}
