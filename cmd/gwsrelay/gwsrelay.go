package main

import (
	"fmt"
	"github.com/lxzan/gws"
	"go-relay/cmd/conf"
	"log"
	"net/http"
	"runtime"
	"time"
)

func main() {
	upgrader := gws.NewUpgrader(&Relay{}, &gws.ServerOption{
		ParallelEnabled:   true,                                 // Parallel message processing
		Recovery:          gws.Recovery,                         // Exception recovery
		PermessageDeflate: gws.PermessageDeflate{Enabled: true}, // Enable compression
	})
	http.HandleFunc("/relay", func(writer http.ResponseWriter, request *http.Request) {
		socket, err := upgrader.Upgrade(writer, request)
		if err != nil {
			return
		}
		go func() {
			socket.ReadLoop() // Blocking prevents the context from being GC.
		}()
	})

	go func() {
		for {
			if conf.UseGosched {
				runtime.Gosched()
			}
		}
	}()

	http.ListenAndServe(":8081", nil)
}

type Relay struct {
}

func (r *Relay) OnOpen(socket *gws.Conn) {}

func (r *Relay) OnClose(socket *gws.Conn, err error) {}

func (r *Relay) OnPing(socket *gws.Conn, payload []byte) {}

func (r *Relay) OnPong(socket *gws.Conn, payload []byte) {}

func (r *Relay) OnMessage(socket *gws.Conn, message *gws.Message) {
	defer message.Close()

	//fmt.Printf("onMessage: %v\n", message.Data)

	// Forward from receiver to sender
	if message.Data.String() == "ready" {
		fmt.Println("Client sent ready")

		connectSender(socket)
	}
}

func connectSender(receiver *gws.Conn) {
	ws := &Sender{
		messageChan: make(chan []byte, conf.MessageChanSize),
		receiver:    receiver,
	}

	socket, _, err := gws.NewClient(ws, &gws.ClientOption{
		Addr: "ws://127.0.0.1:8080/sender",
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
		ws.ForwardMessageLoop()
	}()

	go func() {
		socket.ReadLoop()
	}()

	time.Sleep(100 * time.Millisecond)

	socket.WriteString("ready")
}

type Sender struct {
	messageChan chan []byte
	receiver    *gws.Conn
}

func (c *Sender) OnClose(socket *gws.Conn, err error) {}

func (c *Sender) OnPong(socket *gws.Conn, payload []byte) {}

func (c *Sender) OnOpen(socket *gws.Conn) {}

func (c *Sender) OnPing(socket *gws.Conn, payload []byte) {}

func (c *Sender) OnMessage(socket *gws.Conn, message *gws.Message) {
	defer message.Close()

	select {
	case c.messageChan <- message.Data.Bytes():
	default:
		fmt.Println("message chan full")
	}

	//c.receiver.WriteMessage(gws.OpcodeBinary, message.Data.Bytes())

}

func (c *Sender) ForwardMessageLoop() {
	if conf.LockOSThread {
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()
	}

	for {
		msg := <-c.messageChan
		c.receiver.WriteMessage(gws.OpcodeBinary, msg)
	}
}
