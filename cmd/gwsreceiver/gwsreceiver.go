package main

import (
	"encoding/binary"
	"fmt"
	"github.com/lxzan/gws"
	"go-relay/cmd/conf"
	"log"
	"runtime"
	"time"
)

type MessageLatency struct {
	msg        []byte
	recvNanoTS uint64
}

func main() {
	ws := &WebSocket{
		messageChan: make(chan MessageLatency, conf.MessageChanSize),
	}

	socket, _, err := gws.NewClient(ws, &gws.ClientOption{
		Addr: "ws://127.0.0.1:8081/relay",
		PermessageDeflate: gws.PermessageDeflate{
			Enabled:               true,
			ServerContextTakeover: true,
			ClientContextTakeover: true,
		},
	})
	if err != nil {
		log.Printf(err.Error())
		return
	}

	go func() {
		socket.ReadLoop()
	}()

	time.Sleep(100 * time.Millisecond)

	socket.WriteString("ready")

	go func() {
		for {
			if conf.UseGosched {
				runtime.Gosched()
			}
		}
	}()

	// run forever
	select {}
}

type WebSocket struct {
	messageChan chan MessageLatency
}

func (c *WebSocket) OnClose(socket *gws.Conn, err error) {
	fmt.Printf("onerror: err=%s\n", err.Error())
}

func (c *WebSocket) OnPong(socket *gws.Conn, payload []byte) {
}

func (c *WebSocket) OnOpen(socket *gws.Conn) {
	_ = socket.WriteString("hello, there is client")

	go c.TimeMessages()
}

func (c *WebSocket) OnPing(socket *gws.Conn, payload []byte) {
	_ = socket.WritePong(payload)
}

func (c *WebSocket) OnMessage(socket *gws.Conn, message *gws.Message) {
	defer message.Close()

	recvNanoTS := time.Now().UnixNano()

	ml := MessageLatency{
		msg:        message.Data.Bytes(),
		recvNanoTS: uint64(recvNanoTS),
	}

	select {
	case c.messageChan <- ml:
	default:
		fmt.Println("receiver chan full")
	}
}

func (c *WebSocket) TimeMessages() {
	if conf.LockOSThread {
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()
	}

	var (
		minLatency  uint64
		maxLatency  uint64
		total       uint64
		lastLatency uint64
		count       uint64
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
				time.Duration(lastLatency),
				time.Duration(minLatency),
				time.Duration(maxLatency),
				time.Duration(total/count),
				count,
				conf.UseGosched,
			)
		case ml := <-c.messageChan:
			count++
			if count < conf.IgnoreInitialMessageCount {
				continue
			}

			msg := ml.msg

			ts := binary.LittleEndian.Uint64(msg[:conf.TimestampBytes])

			latencyNanos := ml.recvNanoTS - ts

			// TODO fix overflow - threshold one hour
			if latencyNanos > 3_600_000_000_000 {
				latencyNanos = lastLatency
			}

			// Update metrics
			if minLatency == 0 || latencyNanos < minLatency {
				minLatency = latencyNanos
			}
			if latencyNanos > maxLatency {
				maxLatency = latencyNanos
			}
			total += latencyNanos
			lastLatency = latencyNanos
		}
	}
}
