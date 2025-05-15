package main

import (
	"bytes"
	"encoding/binary"
	"github.com/gorilla/websocket"
	"go-relay/cmd/conf"
	"log"
	"math/rand"
	"net/http"
	"runtime"
	"time"
)

var (
	upgrader = websocket.Upgrader{
		ReadBufferSize:  conf.ReadBufferSize,
		WriteBufferSize: conf.WriteBufferSize,
	}
)

func main() {
	if conf.PayloadMaxBytes < conf.PayloadMinBytes {
		log.Fatal("PayloadMaxBytes must be greater or equal to PayloadMinBytes")
	}

	messageChan := make(chan []byte, conf.MessageChanSize)

	// Create messages
	go func() {
		prng := rand.New(rand.NewSource(conf.RandSeed))

		for {
			if conf.SenderThrottleMillis > 0 {
				time.Sleep(conf.SenderThrottleMillis * time.Millisecond)
			}

			//// Generate random payload (2-4096 bytes)
			length := prng.Intn(conf.PayloadMaxBytes-conf.PayloadMinBytes) + conf.PayloadMinBytes
			data := make([]byte, length)
			prng.Read(data)

			buf := bytes.NewBuffer(make([]byte, 0, conf.TimestampBytes+length))

			// Prepend timestamp (int64, 8 bytes)
			binary.Write(buf, binary.BigEndian, time.Now().UnixNano())
			buf.Write(data)

			payloadBytes := buf.Bytes()

			select {
			case messageChan <- payloadBytes:
			}
		}
	}()

	http.HandleFunc("/sender", func(w http.ResponseWriter, r *http.Request) {
		if conf.LockOSThread {
			runtime.LockOSThread()
			defer runtime.UnlockOSThread()
		}

		conn, _ := upgrader.Upgrade(w, r, nil)
		defer conn.Close()

		for {
			select {
			case msg := <-messageChan:
				// Send message
				conn.WriteMessage(websocket.BinaryMessage, msg)
			default:
				if conf.UseGosched {
					runtime.Gosched()
				} 
			}
		}
	})
	http.ListenAndServe(":8080", nil)
}
