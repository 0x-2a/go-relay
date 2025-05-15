package main

import (
	"encoding/binary"
	"fmt"
	"github.com/lxzan/gws"
	"go-relay/cmd/conf"
	"math/rand"
	"net/http"
	"time"
)

const (
	PingInterval = 5 * time.Second
	PingWait     = 10 * time.Second
)

func main() {
	upgrader := gws.NewUpgrader(&Handler{}, &gws.ServerOption{
		ParallelEnabled:   true,                                 // Parallel message processing
		Recovery:          gws.Recovery,                         // Exception recovery
		PermessageDeflate: gws.PermessageDeflate{Enabled: true}, // Enable compression
	})
	http.HandleFunc("/sender", func(writer http.ResponseWriter, request *http.Request) {
		socket, err := upgrader.Upgrade(writer, request)
		if err != nil {
			return
		}
		go func() {
			socket.ReadLoop() // Blocking prevents the context from being GC.
		}()
	})

	http.ListenAndServe(":8080", nil)
}

type Handler struct{}

func (c *Handler) OnOpen(socket *gws.Conn) {
	//_ = socket.SetDeadline(time.Now().Add(PingInterval + PingWait))
}

func (c *Handler) OnClose(socket *gws.Conn, err error) {}

func (c *Handler) OnPing(socket *gws.Conn, payload []byte) {
	_ = socket.SetDeadline(time.Now().Add(PingInterval + PingWait))
	_ = socket.WritePong(nil)
}

func (c *Handler) OnPong(socket *gws.Conn, payload []byte) {}

func (c *Handler) OnMessage(socket *gws.Conn, message *gws.Message) {
	defer message.Close()
	
	
	if message.Data.String() == "ready" {
		fmt.Println("Client sent ready")
		
		loopBroadcast(socket)
	}
}

func loopBroadcast(socket *gws.Conn) {
	messageChan := make(chan []byte, conf.MessageChanSize)

	// Create messages
	go func() {
		prng := rand.New(rand.NewSource(conf.RandSeed))

		for {
			if conf.SenderThrottleMillis > 0 {
				time.Sleep(conf.SenderThrottleMillis * time.Millisecond)
			}

			
			randomLength := prng.Intn(conf.PayloadMaxBytes-conf.PayloadMinBytes) + conf.PayloadMinBytes
			byteArray := make([]byte, 8+randomLength)
			_, err := prng.Read(byteArray[8:])
			if err != nil {
				panic(err)
			}

			select {
			case messageChan <- byteArray:
			}
		}
	}()

	time.Sleep(100 * time.Millisecond)

	for {
		if len(messageChan) == 0 {
			panic("message chan is empty")
		}

		msgBytes := <-messageChan

		timestamp := time.Now().UnixNano()
		binary.LittleEndian.PutUint64(msgBytes[:8], uint64(timestamp))

		socket.WriteMessage(gws.OpcodeBinary, msgBytes)

		if conf.SenderThrottleMillis > 0 {
			time.Sleep(conf.SenderThrottleMillis * time.Millisecond)
		}
	}
}
