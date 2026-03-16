package ws

import (
	"fmt"
	"log"
	"net/url"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

type Client struct {
	url       string
	conn      *websocket.Conn
	mu        sync.Mutex
	done      chan struct{}
	OnMessage func([]byte)
}

func NewClient(wsURL, token string) *Client {
	u, err := url.Parse(wsURL)
	if err != nil {
		log.Fatalf("Invalid WebSocket URL: %v", err)
	}
	q := u.Query()
	q.Set("token", token)
	u.RawQuery = q.Encode()

	return &Client{
		url:  u.String(),
		done: make(chan struct{}),
	}
}

func (c *Client) Connect() error {
	dialer := websocket.DefaultDialer
	dialer.HandshakeTimeout = 15 * time.Second

	conn, _, err := dialer.Dial(c.url, nil)
	if err != nil {
		return fmt.Errorf("websocket dial: %w", err)
	}
	c.mu.Lock()
	c.conn = conn
	c.mu.Unlock()
	log.Println("[ws] Connected to Console")
	return nil
}

func (c *Client) ConnectWithRetry() {
	backoff := 2 * time.Second
	maxBackoff := 5 * time.Minute

	for {
		err := c.Connect()
		if err == nil {
			return
		}
		log.Printf("[ws] Connection failed: %v, retrying in %v", err, backoff)
		time.Sleep(backoff)
		backoff *= 2
		if backoff > maxBackoff {
			backoff = maxBackoff
		}
	}
}

func (c *Client) ReadLoop() {
	defer func() {
		c.mu.Lock()
		if c.conn != nil {
			c.conn.Close()
			c.conn = nil
		}
		c.mu.Unlock()
	}()

	for {
		_, msg, err := c.conn.ReadMessage()
		if err != nil {
			log.Printf("[ws] Read error: %v", err)
			return
		}
		if c.OnMessage != nil {
			c.OnMessage(msg)
		}
	}
}

func (c *Client) SendJSON(v interface{}) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.conn == nil {
		return fmt.Errorf("not connected")
	}
	return c.conn.WriteJSON(v)
}

func (c *Client) Close() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.conn != nil {
		c.conn.WriteMessage(websocket.CloseMessage,
			websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""))
		c.conn.Close()
		c.conn = nil
	}
}
