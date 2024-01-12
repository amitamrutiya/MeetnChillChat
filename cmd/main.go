package main

import (
	"log"

	"github.com/amitamrutiya/videocall-project/internal/server"
)

func main() {
	if err := server.Run(); err != nil {
		log.Fatalln(err.Error())
	}
}
