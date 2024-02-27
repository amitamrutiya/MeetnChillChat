package main

import (
	"flag"
	"log"
	"net"
	"os"
	"os/signal"
	"regexp"
	"strconv"
	"syscall"

	"github.com/pion/turn/v2"
)

func main() {
	// Command-line flags
	publicIP := flag.String("public-ip", "", "Public IP address of the TURN server")
	port := flag.Int("port", 3478, "Port number for the TURN server")
	users := flag.String("users", "", "List of users and passwords in the format 'user=password,user=password'")
	realm := flag.String("realm", "v.amit.sh", "Realm for the TURN server")
	flag.Parse()

	// Validate command-line flags
	if len(*publicIP) == 0 {
		log.Fatalf("public-ip is required")
	}
	if len(*users) == 0 {
		log.Fatalf("'users' is required")
	}

	// Create a UDP listener for TURN server
	udpListener, err := net.ListenPacket("udp4", "0.0.0.0:"+strconv.Itoa(*port))
	if err != nil {
		log.Panicf("failed to create TURN server listener: %s", err)
	}

	// Parse user credentials and generate authentication keys
	usersMap := map[string][]byte{}
	for _, kv := range regexp.MustCompile(`(\w+)=(\w+)`).FindAllStringSubmatch(*users, -1) {
		usersMap[kv[1]] = turn.GenerateAuthKey(kv[1], *realm, kv[2])
	}

	// Configure and start the TURN server
	s, err := turn.NewServer(turn.ServerConfig{
		Realm: *realm,
		AuthHandler: func(username string, realm string, srcAddr net.Addr) ([]byte, bool) {
			if key, ok := usersMap[username]; ok {
				return key, true
			}
			return nil, false
		},
		PacketConnConfigs: []turn.PacketConnConfig{
			{
				PacketConn: udpListener,
				RelayAddressGenerator: &turn.RelayAddressGeneratorPortRange{
					RelayAddress: net.ParseIP(*publicIP),
					Address:      "0.0.0.0",
					MinPort:      50000,
					MaxPort:      55000,
				},
			},
		},
	})
	if err != nil {
		log.Panic(err)
	}

	// Wait for termination signals
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)
	<-sigs

	// Close the TURN server
	if err = s.Close(); err != nil {
		log.Panic(err)
	}
}
