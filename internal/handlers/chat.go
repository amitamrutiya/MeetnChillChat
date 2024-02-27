package handlers

import (
	"github.com/amitamrutiya/videocall-project/pkg/chat"
	w "github.com/amitamrutiya/videocall-project/pkg/webrtc"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/websocket/v2"
)

// RoomChat renders the chat room view
func RoomChat(c *fiber.Ctx) error {
	return c.Render("chat", fiber.Map{}, "layouts/main")
}

// RoomChatWebsocket handles websocket connections for the chat room
func RoomChatWebsocket(c *websocket.Conn) {
	uuid := c.Params("uuid")
	if uuid == "" {
		return
	}

	// Lock rooms map to ensure concurrent safety
	w.RoomsLock.Lock()
	defer w.RoomsLock.Unlock()

	room := w.Rooms[uuid]
	if room == nil {
		return
	}
	if room.Hub == nil {
		return
	}

	// Establish chat connection for the peer
	chat.PeerChatConn(c.Conn, room.Hub)
}

// StreamChatWebsocket handles websocket connections for chat in a stream
func StreamChatWebsocket(c *websocket.Conn) {
	suuid := c.Params("suuid")
	if suuid == "" {
		return
	}

	// Lock rooms map to ensure concurrent safety
	w.RoomsLock.Lock()
	defer w.RoomsLock.Unlock()

	// Check if stream exists, if not, return
	if stream, ok := w.Streams[suuid]; ok {
		if stream.Hub == nil {
			// If hub is nil, create a new hub and start it
			hub := chat.NewHub()
			stream.Hub = hub
			go hub.Run()
		}

		// Establish chat connection for the peer
		chat.PeerChatConn(c.Conn, stream.Hub)
		return
	}
}
