package handlers

import (
	"fmt"
	"log"
	"os"
	"time"

	"github.com/amitamrutiya/videocall-project/pkg/chat"
	w "github.com/amitamrutiya/videocall-project/pkg/webrtc"

	"crypto/sha256"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/websocket/v2"
	"github.com/google/uuid"
	"github.com/pion/webrtc/v3"
)

// RoomCreate creates a new room and redirects to it
func RoomCreate(c *fiber.Ctx) error {
	return c.Redirect(fmt.Sprintf("/room/%s", uuid.New().String()))
}

// Room handles the room logic
func Room(c *fiber.Ctx) error {
	uuid := c.Params("uuid")
	if uuid == "" {
		c.Status(400) // Bad request if UUID is empty
		return nil
	}

	// Determine WebSocket protocol based on environment
	ws := "ws"
	if os.Getenv("ENVIRONMENT") == "PRODUCTION" {
		ws = "wss"
	}

	uuid, suuid, _ := createOrGetRoom(uuid) // Create or get existing room
	fmt.Println("host name", c.Protocol())
	return c.Render("peer", fiber.Map{
		"RoomWebsocketAddr":   fmt.Sprintf("%s://%s/room/%s/websocket", ws, c.Hostname(), uuid),
		"RoomLink":            fmt.Sprintf("%s://%s/room/%s", c.Protocol(), c.Hostname(), uuid),
		"ChatWebsocketAddr":   fmt.Sprintf("%s://%s/room/%s/chat/websocket", ws, c.Hostname(), uuid),
		"ViewerWebsocketAddr": fmt.Sprintf("%s://%s/room/%s/viewer/websocket", ws, c.Hostname(), uuid),
		"StreamLink":          fmt.Sprintf("%s://%s/stream/%s", c.Protocol(), c.Hostname(), suuid),
		"Type":                "room",
	}, "layouts/main")
}

// RoomWebsocket handles websocket connections for a room
func RoomWebsocket(c *websocket.Conn) {
	uuid := c.Params("uuid")
	if uuid == "" {
		return
	}

	_, _, room := createOrGetRoom(uuid)
	w.RoomConn(c, room.Peers)
}

// createOrGetRoom creates or retrieves an existing room
func createOrGetRoom(uuid string) (string, string, *w.Room) {
	w.RoomsLock.Lock()
	defer w.RoomsLock.Unlock()

	// Generate unique ID for the room
	h := sha256.New()
	h.Write([]byte(uuid))
	suuid := fmt.Sprintf("%x", h.Sum(nil))

	// Check if the room already exists, if not, create a new one
	if room := w.Rooms[uuid]; room != nil {
		if _, ok := w.Streams[suuid]; !ok {
			w.Streams[suuid] = room
		}
		return uuid, suuid, room
	}

	// Create a new chat hub and peers object for the room
	hub := chat.NewHub()
	p := &w.Peers{}
	p.TrackLocals = make(map[string]*webrtc.TrackLocalStaticRTP)
	room := &w.Room{
		Peers: p,
		Hub:   hub,
	}

	// Add the room to the rooms map and the streams map
	w.Rooms[uuid] = room
	w.Streams[suuid] = room

	// Start the chat hub
	go hub.Run()
	return uuid, suuid, room
}

// RoomViewerWebsocket handles websocket connections for viewers in a room
func RoomViewerWebsocket(c *websocket.Conn) {
	uuid := c.Params("uuid")
	if uuid == "" {
		return
	}

	w.RoomsLock.Lock()
	if peer, ok := w.Rooms[uuid]; ok {
		w.RoomsLock.Unlock()
		roomViewerConn(c, peer.Peers)
		return
	}
	w.RoomsLock.Unlock()
}

// roomViewerConn handles websocket connections for viewers
func roomViewerConn(c *websocket.Conn, p *w.Peers) {
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()
	defer c.Close()

	// Periodically send the number of connections to the viewer
	for range ticker.C {
		w, err := c.Conn.NextWriter(websocket.TextMessage)
		if err != nil {
			log.Println("error getting next writer:", err)
			return
		}
		w.Write([]byte(fmt.Sprintf("%d", len(p.Connections))))
	}
}
