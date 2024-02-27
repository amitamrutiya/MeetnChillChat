package webrtc

import (
	"encoding/json"
	"log"
	"os"
	"sync"

	"github.com/gofiber/websocket/v2"
	"github.com/pion/webrtc/v3"
)

// RoomConn establishes a new WebRTC connection for a room
func RoomConn(c *websocket.Conn, p *Peers) {
	// Configuration for the WebRTC connection
	var config webrtc.Configuration
	if os.Getenv("ENVIRONMENT") == "PRODUCTION" {
		config = turnConfig // Use TURN server in production
	}

	// Create a new peer connection
	peerConnection, err := webrtc.NewPeerConnection(config)
	if err != nil {
		log.Print(err)
		return
	}
	defer peerConnection.Close() // Close the peer connection when the function exits

	// Add transceivers for video and audio
	for _, typ := range []webrtc.RTPCodecType{webrtc.RTPCodecTypeVideo, webrtc.RTPCodecTypeAudio} {
		if _, err := peerConnection.AddTransceiverFromKind(typ, webrtc.RTPTransceiverInit{
			Direction: webrtc.RTPTransceiverDirectionRecvonly,
		}); err != nil {
			log.Print("error adding transceiver:", err)
			return
		}
	}

	// Create a new PeerConnectionState
	newPeer := PeerConnectionState{
		PeerConnection: peerConnection,
		Websocket: &ThreadSafeWriter{
			Conn:  c,
			Mutex: sync.Mutex{},
		},
	}

	// Add the new PeerConnection to the global list
	p.ListLock.Lock()
	p.Connections = append(p.Connections, newPeer)
	p.ListLock.Unlock()

	log.Println("New peer connection established: ", p.Connections)

	// Handle ICE candidate messages from the client
	peerConnection.OnICECandidate(func(i *webrtc.ICECandidate) {
		if i == nil {
			log.Println("nil ICE candidate")
			return
		}

		candidateString, err := json.Marshal(i.ToJSON())
		if err != nil {
			log.Println("error marshalling ICE candidate:", err)
			return
		}

		if writeErr := newPeer.Websocket.WriteJSON(&websocketMessage{
			Event: "candidate",
			Data:  string(candidateString),
		}); writeErr != nil {
			log.Println("error writing ICE candidate:", writeErr)
		}
	})

	// Handle changes in connection state
	peerConnection.OnConnectionStateChange(func(pp webrtc.PeerConnectionState) {
		switch pp {
		case webrtc.PeerConnectionStateFailed:
			if err := peerConnection.Close(); err != nil {
				log.Print("error closing peer connection:", err)
			}
		case webrtc.PeerConnectionStateClosed:
			p.SignalPeerConnections() // Signal peer connections when closed
		}
	})

	// Handle incoming tracks
	peerConnection.OnTrack(func(t *webrtc.TrackRemote, _ *webrtc.RTPReceiver) {
		// Add the track to the peer's track list
		trackLocal := p.AddTrack(t)
		if trackLocal == nil {
			log.Println("error adding track")
			return
		}
		defer p.RemoveTrack(trackLocal)

		// Read and write incoming video stream
		buf := make([]byte, 1500)
		for {
			i, _, err := t.Read(buf)
			if err != nil {
				log.Println("error reading from track:", err)
				return
			}

			if _, err = trackLocal.Write(buf[:i]); err != nil {
				log.Println("error writing to track:", err)
				return
			}
		}
	})

	p.SignalPeerConnections() // Signal peer connections upon successful setup

	message := &websocketMessage{}
	for {
		// Read and handle messages from the client
		_, raw, err := c.ReadMessage()
		if err != nil {
			log.Println(err)
			return
		} else if err := json.Unmarshal(raw, &message); err != nil {
			log.Println(err)
			return
		}

		switch message.Event {
		case "candidate":
			// Handle ICE candidate message
			candidate := webrtc.ICECandidateInit{}
			if err := json.Unmarshal([]byte(message.Data), &candidate); err != nil {
				log.Println(err)
				return
			}

			if err := peerConnection.AddICECandidate(candidate); err != nil {
				log.Println(err)
				return
			}
		case "answer":
			// Handle SDP answer message
			answer := webrtc.SessionDescription{}
			if err := json.Unmarshal([]byte(message.Data), &answer); err != nil {
				log.Println(err)
				return
			}

			if err := peerConnection.SetRemoteDescription(answer); err != nil {
				log.Println(err)
				return
			}
		}
	}
}
