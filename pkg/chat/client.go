package chat

import (
	"bytes"
	"log"
	"time"

	"github.com/fasthttp/websocket"
)

const (
	writeWait      = 10 * time.Second
	pongWait       = 60 * time.Second    // 60s
	pingPeriod     = (pongWait * 9) / 10 // 54s
	maxMessageSize = 512
)

// Byte slices used for message processing
var (
	newline = []byte{'\n'}
	space   = []byte{' '}
)

// WebSocket upgrader configuration
// var upgrader = websocket.FastHTTPUpgrader{
// 	ReadBufferSize:  1024,
// 	WriteBufferSize: 1024,
// }

// Client represents a chat client connected to the hub
type Client struct {
	Hub  *Hub            // Reference to the hub this client is connected to
	Conn *websocket.Conn // WebSocket connection of the client
	Send chan []byte     // Channel for sending messages to the client
}

// readPump listens for messages from the WebSocket connection and sends them to the hub
func (c *Client) readPump() {
	defer func() {
		c.Hub.unregister <- c // Unregister client from the hub when done
		c.Conn.Close()        // Close the WebSocket connection
	}()
	c.Conn.SetReadLimit(maxMessageSize)              // Set maximum message size
	c.Conn.SetReadDeadline(time.Now().Add(pongWait)) // Set read deadline
	c.Conn.SetPongHandler(func(string) error {       // Set pong handler to update read deadline
		c.Conn.SetReadDeadline(time.Now().Add(pongWait))
		return nil
	})
	for {
		_, message, err := c.Conn.ReadMessage() // Read message from WebSocket connection
		if err != nil {
			// Handle WebSocket read errors
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Printf("error: %v", err)
			}
			log.Printf("other error: %v", err)
			break
		}
		// Trim leading and trailing whitespace, replace newlines with spaces
		message = bytes.TrimSpace(bytes.Replace(message, newline, space, -1))
		c.Hub.broadcast <- message // Send message to the hub for broadcasting
	}
}

// writePump listens for messages from the hub and writes them to the WebSocket connection
func (c *Client) writePump() {
	ticker := time.NewTicker(pingPeriod) // Create ticker for sending ping messages
	defer func() {
		ticker.Stop()
		c.Conn.Close() // Close WebSocket connection when done
	}()
	for {
		select {
		case message, ok := <-c.Send: // Receive message from the client's send channel
			c.Conn.SetWriteDeadline(time.Now().Add(writeWait)) // Set write deadline
			if !ok {
				return
			}

			w, err := c.Conn.NextWriter(websocket.TextMessage) // Get writer for writing message
			if err != nil {
				return
			}
			w.Write(message) // Write message to the WebSocket connection

			// Write any buffered messages to the WebSocket connection
			n := len(c.Send)
			for i := 0; i < n; i++ {
				w.Write(newline)
				w.Write(<-c.Send)
			}

			// Close the writer
			if err := w.Close(); err != nil {
				return
			}
		case <-ticker.C: // Periodically send ping messages to the client
			c.Conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err := c.Conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

// PeerChatConn creates a new client and manages its lifecycle
func PeerChatConn(c *websocket.Conn, hub *Hub) {
	// Create a new client instance
	client := &Client{Hub: hub, Conn: c, Send: make(chan []byte, 256)}
	// Register the client with the hub
	client.Hub.register <- client

	// Start the client's write and read pumps concurrently
	go client.writePump()
	client.readPump()
}
