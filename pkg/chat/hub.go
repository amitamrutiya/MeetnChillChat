package chat

// Hub represents a chat hub that manages clients
type Hub struct {
	clients    map[*Client]bool   // Map to store connected clients
	broadcast  chan []byte        // Channel to broadcast messages to clients
	register   chan *Client       // Channel to register new clients
	unregister chan *Client       // Channel to unregister clients
}

// NewHub creates a new instance of Hub
func NewHub() *Hub {
	return &Hub{
		broadcast:  make(chan []byte),  // Buffered channel for broadcasting messages
		register:   make(chan *Client), // Channel for registering new clients
		unregister: make(chan *Client), // Channel for unregistering clients
		clients:    make(map[*Client]bool), // Map to store connected clients
	}
}

// Run starts the chat hub to manage clients
func (h *Hub) Run() {
	for {
		select {
		case client := <-h.register:
			// Register new client
			h.clients[client] = true
		case client := <-h.unregister:
			// Unregister client
			if _, ok := h.clients[client]; ok {
				delete(h.clients, client)
				close(client.Send)
			}
		case message := <-h.broadcast:
			// Broadcast message to all clients
			for client := range h.clients {
				select {
				case client.Send <- message:
					// Send message to client
				default:
					// If unable to send, close connection and unregister client
					close(client.Send)
					delete(h.clients, client)
				}
			}
		}
	}
}
