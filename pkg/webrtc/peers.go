package webrtc

import (
	"encoding/json"
	"log"
	"sync"
	"time"

	"github.com/gofiber/websocket/v2"
	"github.com/pion/rtcp"
	"github.com/pion/webrtc/v3"

	"github.com/amitamrutiya/videocall-project/pkg/chat"
)

// RoomsLock is a mutex for rooms concurrency control
var RoomsLock sync.RWMutex

// Rooms stores rooms by UUID
var Rooms map[string]*Room

// Streams stores streams by SUUID
var Streams map[string]*Room

// turnConfig specifies the TURN server configuration
var turnConfig = webrtc.Configuration{
	ICETransportPolicy: webrtc.ICETransportPolicyRelay,
	ICEServers: []webrtc.ICEServer{
		{
			URLs: []string{"stun:turn.localhost:3478"},
		},
		{
			URLs:           []string{"turn:turn.localhost:3478"},
			Username:       "amit",
			Credential:     "amrutiya",
			CredentialType: webrtc.ICECredentialTypePassword,
		},
	},
}

// Room represents a WebRTC room
type Room struct {
	Peers *Peers      // Peers in the room
	Hub   *chat.Hub   // Chat hub associated with the room
}

// Peers represents peers in a room
type Peers struct {
	ListLock     sync.RWMutex              // Mutex for peers list
	Connections  []PeerConnectionState     // Peer connections
	TrackLocals  map[string]*webrtc.TrackLocalStaticRTP // Local tracks
}

// PeerConnectionState represents the state of a peer connection
type PeerConnectionState struct {
	PeerConnection *webrtc.PeerConnection // WebRTC peer connection
	Websocket      *ThreadSafeWriter       // Thread-safe writer for WebSocket
}

// ThreadSafeWriter is a thread-safe writer for WebSocket
type ThreadSafeWriter struct {
	Conn  *websocket.Conn // WebSocket connection
	Mutex sync.Mutex      // Mutex for thread safety
}

// WriteJSON writes JSON data to WebSocket in a thread-safe manner
func (t *ThreadSafeWriter) WriteJSON(v interface{}) error {
	t.Mutex.Lock()
	defer t.Mutex.Unlock()
	return t.Conn.WriteJSON(v)
}

// AddTrack adds a track to the peers list
func (p *Peers) AddTrack(t *webrtc.TrackRemote) *webrtc.TrackLocalStaticRTP {
	p.ListLock.Lock()
	defer func() {
		p.ListLock.Unlock()
		p.SignalPeerConnections()
	}()

	trackLocal, err := webrtc.NewTrackLocalStaticRTP(t.Codec().RTPCodecCapability, t.ID(), t.StreamID())
	if err != nil {
		log.Println(err.Error())
		return nil
	}

	p.TrackLocals[t.ID()] = trackLocal
	return trackLocal
}

// RemoveTrack removes a track from the peers list
func (p *Peers) RemoveTrack(t *webrtc.TrackLocalStaticRTP) {
	p.ListLock.Lock()
	defer func() {
		p.ListLock.Unlock()
		p.SignalPeerConnections()
	}()

	delete(p.TrackLocals, t.ID())
}

// SignalPeerConnections signals peer connections to establish and update tracks.
// This method is responsible for managing the signaling process between peers to establish and update tracks.
// It first acquires a lock on the list of peers to ensure concurrent-safe access.
// Then, it attempts to synchronize the peer connections by iterating over each connection.
// If a connection is closed, it removes it from the list of connections.
// For each connection, it checks for existing senders and receivers.
// It ensures that all local tracks are added to the connection and removes any tracks not present locally.
// After ensuring track consistency, it creates an offer for the peer connection and sends it to the client over WebSocket.
// If any error occurs during the process, it returns true to indicate that synchronization should be retried.
// It retries synchronization up to 25 times before sleeping for 3 seconds and retrying again.
// The method returns once synchronization is successful or after the maximum number of attempts.
func (p *Peers) SignalPeerConnections() {
	// Lock the list of peers to ensure concurrent-safe access
	p.ListLock.Lock()
	defer func() {
		// Unlock the list of peers when done
		p.ListLock.Unlock()
		// Dispatch key frame after signaling peer connections
		p.DispatchKeyFrame()
	}()

	// Function to attempt synchronization
	attemptSync := func() (tryAgain bool) {
		// Iterate over each connection
		for i := range p.Connections {
			// If connection is closed, remove it from the list
			if p.Connections[i].PeerConnection.ConnectionState() == webrtc.PeerConnectionStateClosed {
				p.Connections = append(p.Connections[:i], p.Connections[i+1:]...)
				log.Println("a", p.Connections)
				return true
			}

			// Map to track existing senders
			existingSenders := map[string]bool{}
			// Iterate over senders and add to existing senders map
			for _, sender := range p.Connections[i].PeerConnection.GetSenders() {
				if sender.Track() == nil {
					continue
				}
				existingSenders[sender.Track().ID()] = true

				// If track not found locally, remove it from the connection
				if _, ok := p.TrackLocals[sender.Track().ID()]; !ok {
					if err := p.Connections[i].PeerConnection.RemoveTrack(sender); err != nil {
						return true
					}
				}
			}

			// Iterate over receivers and add to existing senders map
			for _, receiver := range p.Connections[i].PeerConnection.GetReceivers() {
				if receiver.Track() == nil {
					continue
				}
				existingSenders[receiver.Track().ID()] = true
			}

			// Add local tracks to the connection if not already present
			for trackID := range p.TrackLocals {
				if _, ok := existingSenders[trackID]; !ok {
					if _, err := p.Connections[i].PeerConnection.AddTrack(p.TrackLocals[trackID]); err != nil {
						return true
					}
				}
			}

			// Create offer for the connection
			offer, err := p.Connections[i].PeerConnection.CreateOffer(nil)
			if err != nil {
				return true
			}

			// Set local description for the connection
			if err = p.Connections[i].PeerConnection.SetLocalDescription(offer); err != nil {
				return true
			}

			// Marshal offer to JSON
			offerString, err := json.Marshal(offer)
			if err != nil {
				return true
			}

			// Send offer to client over WebSocket
			if err = p.Connections[i].Websocket.WriteJSON(&websocketMessage{
				Event: "offer",
				Data:  string(offerString),
			}); err != nil {
				return true
			}
		}

		return
	}

	// Attempt synchronization in a loop
	for syncAttempt := 0; ; syncAttempt++ {
		// If sync attempt reaches maximum retries, retry after a delay
		if syncAttempt == 25 {
			go func() {
				time.Sleep(time.Second * 3)
				p.SignalPeerConnections() // Retry signaling peer connections
			}()
			return
		}

		// If synchronization fails, retry or exit loop
		if !attemptSync() {
			break
		}
	}
}

// DispatchKeyFrame sends key frame requests to peer connections
func (p *Peers) DispatchKeyFrame() {
	p.ListLock.Lock()
	defer p.ListLock.Unlock()

	for i := range p.Connections {
		for _, receiver := range p.Connections[i].PeerConnection.GetReceivers() {
			if receiver.Track() == nil {
				continue
			}

			_ = p.Connections[i].PeerConnection.WriteRTCP([]rtcp.Packet{
				&rtcp.PictureLossIndication{
					MediaSSRC: uint32(receiver.Track().SSRC()),
				},
			})
		}
	}
}

// websocketMessage represents a message exchanged over WebSocket
type websocketMessage struct {
	Event string `json:"event"`
	Data  string `json:"data"`
}
