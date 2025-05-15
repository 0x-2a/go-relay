package main

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"github.com/gorilla/websocket"
	"go-relay/cmd/conf"
	"runtime"
	"time"
)

type MessageLatency struct {
	msg        []byte
	recvNanoTS int64
}

func main() {
	//dialer := websocket.Dialer{
	//	ReadBufferSize:  conf.ReadBufferSize,
	//	WriteBufferSize: conf.WriteBufferSize,
	//}

	ws, _, _ := websocket.DefaultDialer.Dial("ws://localhost:8080/sender", nil)
	defer func() {
		if ws != nil {
			ws.Close()
		}
	}()

	messageChan := make(chan MessageLatency, conf.MessageChanSize)

	go func() {
		if conf.LockOSThread {
			runtime.LockOSThread()
			defer runtime.UnlockOSThread()
		}

		var (
			minLatency  = time.Duration(1<<63 - 1)
			maxLatency  time.Duration
			total       time.Duration
			lastLatency time.Duration
			count       int
		)

		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				nowTimeStr := time.Now().Format(time.DateTime)
				if count < conf.IgnoreInitialMessageCount {
					fmt.Printf(
						"%v: Ignoring initial messages, count=%v/%v\n",
						nowTimeStr,
						count,
						conf.IgnoreInitialMessageCount,
					)
					continue
				}

				fmt.Printf(
					"%v:  SampleLatency: %v | Min: %v | Max: %v | Avg: %v | Count: %v| UseGosched: %v\n",
					nowTimeStr,
					lastLatency,
					minLatency,
					maxLatency,
					total/time.Duration(count),
					count,
					conf.UseGosched,
				)
			case ml := <-messageChan:
				count++
				if count < conf.IgnoreInitialMessageCount {
					continue
				}

				msg := ml.msg
				// Extract timestamp (first 8 bytes)
				var ts int64
				binary.Read(bytes.NewReader(msg[:conf.TimestampBytes]), binary.BigEndian, &ts)
				latency := time.Duration(ml.recvNanoTS - ts)

				// Update metrics
				if latency < minLatency {
					minLatency = latency
				}
				if latency > maxLatency {
					maxLatency = latency
				}
				total += latency
				lastLatency = latency
			default:
				if conf.UseGosched {
					runtime.Gosched()
				}
			}
		}
	}()
	
	ws.WriteMessage(websocket.BinaryMessage, []byte("ready"))

	for {
		_, msg, err := ws.ReadMessage()
		if err != nil {
			break
		}
		if err != nil || len(msg) < 8 {
			continue
		}

		recvNanoTS := time.Now().UnixNano()

		ml := MessageLatency{
			msg:        msg,
			recvNanoTS: recvNanoTS,
		}

		select {
		case messageChan <- ml:
		default:
			fmt.Println("receiver chan full")
		}

	}
}
