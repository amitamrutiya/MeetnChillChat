package handlers

import (
	"fmt"
	"log"
	"os"
	"time"

	w "github.com/amitamrutiya/videocall-project/pkg/webrtc"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/websocket/v2"
)

// Stream handles streaming logic
func Stream(c *fiber.Ctx) error {
	suuid := c.Params("suuid")
	if suuid == "" {
		c.Status(400) // Bad request if SUUID is empty
		return nil
	}

	// Determine WebSocket protocol based on environment
	ws := "ws"
	if os.Getenv("ENVIRONMENT") == "PRODUCTION" {
		ws = "wss"
	}

	// Lock rooms map to ensure concurrent safety
	w.RoomsLock.Lock()
	defer w.RoomsLock.Unlock()

	// Check if stream exists, if yes, render stream page, else render page with no stream
	if _, ok := w.Streams[suuid]; ok {
		log.Println("Stream exists")
		return c.Render("stream", fiber.Map{
			"StreamWebsocketAddr": fmt.Sprintf("%s://%s/stream/%s/websocket", ws, c.Hostname(), suuid),
			"ChatWebsocketAddr":   fmt.Sprintf("%s://%s/stream/%s/chat/websocket", ws, c.Hostname(), suuid),
			"ViewerWebsocketAddr": fmt.Sprintf("%s://%s/stream/%s/viewer/websocket", ws, c.Hostname(), suuid),
			"Type":                "stream",
		}, "layouts/main")
	}
	log.Println("Stream does not exist")
	return c.Render("stream", fiber.Map{
		"NoStream": "true",
		"Leave":    "true",
	}, "layouts/main")
}

// StreamWebsocket handles websocket connections for streaming
func StreamWebsocket(c *websocket.Conn) {
	suuid := c.Params("suuid")
	if suuid == "" {
		return
	}

	// Lock rooms map to ensure concurrent safety
	w.RoomsLock.Lock()
	defer w.RoomsLock.Unlock()

	// Check if stream exists, if yes, establish connection
	if stream, ok := w.Streams[suuid]; ok {
		w.StreamConn(c, stream.Peers)
	} else {
		log.Println("Stream does not exist")
	}
}

// StreamViewerWebsocket handles websocket connections for viewers in a stream
func StreamViewerWebsocket(c *websocket.Conn) {
	suuid := c.Params("suuid")
	if suuid == "" {
		return
	}

	// Lock rooms map to ensure concurrent safety
	w.RoomsLock.Lock()
	defer w.RoomsLock.Unlock()

	// Check if stream exists, if yes, establish viewer connection
	if stream, ok := w.Streams[suuid]; ok {
		viewerConn(c, stream.Peers)
	} else {
		log.Println("Stream does not exist")
	}
}

// viewerConn handles viewer websocket connections
func viewerConn(c *websocket.Conn, p *w.Peers) {
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()
	defer c.Close()

	// Periodically send the number of connections to the viewer
	for range ticker.C {
		w, err := c.Conn.NextWriter(websocket.TextMessage)
		log.Println("error getting next writer:", err)
		if err != nil {
			return
		}
		w.Write([]byte(fmt.Sprintf("%d", len(p.Connections))))
	}
}
