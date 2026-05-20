package realtime

import (
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

const (
	defaultWriteWait      = 10 * time.Second
	defaultPongWait       = 60 * time.Second
	defaultPingPeriod     = 30 * time.Second
	defaultClientSendSize = 64
)

type WebSocketServer struct {
	hub      *Hub
	upgrader websocket.Upgrader
}

type websocketClient struct {
	hub           *Hub
	conn          *websocket.Conn
	send          chan ServerMessage
	subscriptions map[string]Subscription
	mu            sync.Mutex
	closed        chan struct{}
	closeOnce     sync.Once
}

func NewWebSocketServer(hub *Hub) *WebSocketServer {
	return &WebSocketServer{
		hub: hub,
		upgrader: websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool {
				return true
			},
		},
	}
}

func (s *WebSocketServer) Serve(c *gin.Context) {
	if s == nil || s.hub == nil {
		common.ApiErrorMsg(c, "realtime hub is not initialized")
		return
	}
	conn, err := s.upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		return
	}
	client := &websocketClient{
		hub:           s.hub,
		conn:          conn,
		send:          make(chan ServerMessage, defaultClientSendSize),
		subscriptions: map[string]Subscription{},
		closed:        make(chan struct{}),
	}
	go client.writePump()
	client.readPump()
}

func (c *websocketClient) Send(message ServerMessage) bool {
	if c == nil {
		return false
	}
	select {
	case <-c.closed:
		return false
	default:
	}
	message = c.hub.Prepare(message)
	select {
	case c.send <- message:
		return true
	case <-c.closed:
		return false
	default:
		c.Close()
		return false
	}
}

func (c *websocketClient) readPump() {
	defer c.Close()
	_ = c.conn.SetReadDeadline(time.Now().Add(defaultPongWait))
	c.conn.SetPongHandler(func(string) error {
		return c.conn.SetReadDeadline(time.Now().Add(defaultPongWait))
	})
	for {
		var message ClientMessage
		messageType, data, err := c.conn.ReadMessage()
		if err != nil {
			return
		}
		if messageType != websocket.TextMessage && messageType != websocket.BinaryMessage {
			return
		}
		if err := common.Unmarshal(data, &message); err != nil {
			return
		}
		c.handleMessage(message)
	}
}

func (c *websocketClient) writePump() {
	ticker := time.NewTicker(defaultPingPeriod)
	defer func() {
		ticker.Stop()
		c.Close()
	}()
	for {
		select {
		case message, ok := <-c.send:
			if !ok {
				_ = c.writeControl(websocket.CloseMessage, []byte{})
				return
			}
			if err := c.writeJSON(message); err != nil {
				return
			}
		case <-ticker.C:
			if err := c.writeControl(websocket.PingMessage, nil); err != nil {
				return
			}
		case <-c.closed:
			return
		}
	}
}

func (c *websocketClient) handleMessage(message ClientMessage) {
	switch strings.TrimSpace(message.Type) {
	case MessageTypeSubscribe:
		c.subscribe(message)
	case MessageTypeUnsubscribe:
		c.unsubscribe(message.ID)
	case MessageTypePing:
		c.Send(ServerMessage{Type: MessageTypePong, ID: message.ID, Topic: message.Topic})
	default:
		c.Send(ServerMessage{Type: MessageTypeError, ID: message.ID, Topic: message.Topic, Message: "unsupported realtime message type"})
	}
}

func (c *websocketClient) subscribe(message ClientMessage) {
	subscription := Subscription{
		ID:     strings.TrimSpace(message.ID),
		Topic:  strings.TrimSpace(message.Topic),
		Params: message.Params,
	}
	if subscription.ID == "" || subscription.Topic == "" {
		c.Send(ServerMessage{Type: MessageTypeError, ID: message.ID, Topic: message.Topic, Message: "missing subscription id or topic"})
		return
	}
	if c.hub == nil || !c.hub.Subscribe(c, subscription) {
		c.Send(ServerMessage{Type: MessageTypeError, ID: subscription.ID, Topic: subscription.Topic, Message: "unknown realtime topic"})
		return
	}
	c.mu.Lock()
	c.subscriptions[subscription.ID] = subscription
	c.mu.Unlock()
	c.Send(ServerMessage{
		Type:  MessageTypeStatus,
		ID:    subscription.ID,
		Topic: subscription.Topic,
		Data: map[string]any{
			"subscribed": true,
		},
	})
}

func (c *websocketClient) unsubscribe(subscriptionID string) {
	subscriptionID = strings.TrimSpace(subscriptionID)
	if subscriptionID == "" {
		return
	}
	c.mu.Lock()
	subscription, ok := c.subscriptions[subscriptionID]
	if ok {
		delete(c.subscriptions, subscriptionID)
	}
	c.mu.Unlock()
	if ok && c.hub != nil {
		c.hub.Unsubscribe(c, subscription)
	}
}

func (c *websocketClient) Close() {
	if c == nil {
		return
	}
	c.closeOnce.Do(func() {
		close(c.closed)
		c.mu.Lock()
		subscriptions := make([]Subscription, 0, len(c.subscriptions))
		for _, subscription := range c.subscriptions {
			subscriptions = append(subscriptions, subscription)
		}
		c.subscriptions = map[string]Subscription{}
		c.mu.Unlock()
		for _, subscription := range subscriptions {
			if c.hub != nil {
				c.hub.Unsubscribe(c, subscription)
			}
		}
		_ = c.conn.Close()
	})
}

func (c *websocketClient) writeJSON(message ServerMessage) error {
	_ = c.conn.SetWriteDeadline(time.Now().Add(defaultWriteWait))
	data, err := common.Marshal(message)
	if err != nil {
		return err
	}
	return c.conn.WriteMessage(websocket.TextMessage, data)
}

func (c *websocketClient) writeControl(messageType int, data []byte) error {
	_ = c.conn.SetWriteDeadline(time.Now().Add(defaultWriteWait))
	return c.conn.WriteControl(messageType, data, time.Now().Add(defaultWriteWait))
}
