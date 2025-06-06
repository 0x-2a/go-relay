package main

import (
	"bytes"
	"encoding/binary"
	"flag"
	"go-relay/cmd/conf"
	"log"
	"math/rand"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/Allenxuxu/gev"
	"github.com/Allenxuxu/gev/plugins/websocket"
	"github.com/Allenxuxu/gev/plugins/websocket/ws"
	"github.com/Allenxuxu/gev/plugins/websocket/ws/util"
)

var (
	keyRequestHeader = "requestHeader"
	keyUri           = "uri"
)

type example struct {
	sync.Mutex
	sessions map[*gev.Connection]*Session
}

type Session struct {
	first  bool
	header http.Header
	conn   *gev.Connection
}

// connection lifecycle
// OnConnect() -> OnRequest() -> OnHeader() -> OnMessage() -> OnClose()

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

	s.Lock()
	session, ok := s.sessions[c]
	if !ok {
		s.Unlock()
		return
	}
	s.Unlock()

	if session.first {
		session.first = false

		_header, ok := c.Get(keyRequestHeader)
		if ok {
			header := _header.(http.Header)
			session.header = header
			log.Printf("request header header: %+v \n", header)
		}

		_uri, ok := c.Get(keyUri)
		if ok {
			uri := _uri.(string)
			log.Printf("request uri: %v \n", uri)
		}
	}

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

func loopBroadcast(serv *example) {
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

			select {
			case messageChan <- data:
			}
		}
	}()
	
	time.Sleep(100 * time.Millisecond)

	for {
		//serv.Lock()

		for _, session := range serv.sessions {
			if session == nil {
				continue
			}

			if len(messageChan) == 0 {
				panic("message chan is empty")
			}

			msgBytes := <-messageChan

			buf := bytes.NewBuffer(make([]byte, 0, conf.TimestampBytes+len(msgBytes)))

			// Prepend timestamp (int64, 8 bytes)
			binary.Write(buf, binary.BigEndian, time.Now().UnixNano())
			buf.Write(msgBytes)

			payloadBytes := buf.Bytes()

			msg, err := util.PackData(ws.MessageBinary, payloadBytes)
			if err != nil {
				continue
			}
			_ = session.conn.Send(msg)
		}

		//serv.Unlock()

		if conf.SenderThrottleMillis > 0 {
			time.Sleep(conf.SenderThrottleMillis * time.Millisecond)
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

	flag.IntVar(&port, "port", 8080, "server port")
	flag.IntVar(&loops, "loops", -1, "num loops")
	flag.Parse()

	handler := &example{
		sessions: make(map[*gev.Connection]*Session, 10),
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
		return nil
	}

	go loopBroadcast(handler)

	s, err := NewWebSocketServer(
		handler,
		wsUpgrader,
		gev.Network("tcp"),
		gev.Address(":"+strconv.Itoa(port)),
		gev.NumLoops(loops),
	)
	if err != nil {
		panic(err)
	}

	s.Start()
}
