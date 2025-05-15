package main

import (
	"flag"
	"fmt"
	"github.com/Allenxuxu/gev"
	"github.com/Allenxuxu/gev/plugins/websocket"
	"github.com/Allenxuxu/gev/plugins/websocket/ws"
	"github.com/Allenxuxu/gev/plugins/websocket/ws/util"
	gws "github.com/gorilla/websocket"
	"go-relay/cmd/conf"
	"log"
	"net/http"
	"runtime"
	"strconv"
	"sync"
)

var (
	keyRequestHeader = "requestHeader"
	keyUri           = "uri"
)

type example struct {
	sync.Mutex
	sessions    map[*gev.Connection]*Session
	messageChan chan []byte
}

type Session struct {
	first  bool
	header http.Header
	conn   *gev.Connection
}

func (s *example) OnConnect(c *gev.Connection) {
	log.Println("OnConnect: ", c.PeerAddr())

	s.Lock()
	defer s.Unlock()

	s.sessions[c] = &Session{
		first: true,
		conn:  c,
	}
}

func (s *example) OnMessage(c *gev.Connection, data []byte) (messageType ws.MessageType, out []byte) {
	log.Println("OnMessage: ", string(data))

	messageType = ws.MessageBinary
	out = data

	return messageType, out
}

func (s *example) OnClose(c *gev.Connection) {
	log.Println("OnClose")

	s.Lock()
	defer s.Unlock()

	delete(s.sessions, c)
}

func (s *example) ForwardMessages() {
	// Forward messages from Source to Dest
	go func() {
		// Connect to Source
		senderWS, _, _ := gws.DefaultDialer.Dial("ws://localhost:8080/sender", nil)
		defer senderWS.Close()

		if conf.LockOSThread {
			runtime.LockOSThread()
			defer runtime.UnlockOSThread()
		}

		// Read message, add to channel
		for {
			_, msg, err := senderWS.ReadMessage()
			if err != nil {
				break
			}

			select {
			case s.messageChan <- msg:
			default:
				fmt.Println("relay chan full")
			}
		}
	}()
}

func loopRelay(serv *example) {
	for {
		//serv.Lock()

		for _, session := range serv.sessions {
			if session == nil {
				continue
			}

			if len(serv.messageChan) == 0 {
				continue
			}

			msgBytes := <-serv.messageChan

			msg, err := util.PackData(ws.MessageBinary, msgBytes)
			if err != nil {
				continue
			}
			_ = session.conn.Send(msg)
		}
	}
}

// NewWebSocketServer 创建 WebSocket Server
func NewWebSocketServer(handler websocket.WSHandler, u *ws.Upgrader, opts ...gev.Option) (server *gev.Server, err error) {
	opts = append(opts, gev.CustomProtocol(websocket.New(u)))
	return gev.NewServer(websocket.NewHandlerWrap(u, handler), opts...)
}

func main() {
	var (
		port  int
		loops int
	)

	flag.IntVar(&port, "port", 8081, "server port")
	flag.IntVar(&loops, "loops", -1, "num loops")
	flag.Parse()

	handler := &example{
		sessions:    make(map[*gev.Connection]*Session, 10),
		messageChan: make(chan []byte, conf.MessageChanSize),
	}

	wsUpgrader := &ws.Upgrader{}
	wsUpgrader.OnHeader = func(c *gev.Connection, key, value []byte) error {
		log.Println("OnHeader: ", string(key), string(value))

		var header http.Header
		_header, ok := c.Get("requestHeader")
		if ok {
			header = _header.(http.Header)
		} else {
			header = make(http.Header)
		}
		header.Set(string(key), string(value))

		c.Set(keyRequestHeader, header)
		return nil
	}

	wsUpgrader.OnRequest = func(c *gev.Connection, uri []byte) error {
		log.Println("OnRequest: ", string(uri))

		c.Set(keyUri, string(uri))

		handler.ForwardMessages()

		return nil
	}

	go loopRelay(handler)

	s, err := NewWebSocketServer(handler, wsUpgrader,
		gev.Network("tcp"),
		gev.Address(":"+strconv.Itoa(port)),
		gev.NumLoops(loops))
	if err != nil {
		panic(err)
	}

	s.Start()
}
