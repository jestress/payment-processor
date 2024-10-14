package main

import (
	"log"

	"github.com/form3tech-oss/interview-simulator/contracts"
	"github.com/form3tech-oss/interview-simulator/server"
	"github.com/form3tech-oss/interview-simulator/validator"
)

func init() {
	log.SetFlags(log.Ltime | log.Lmicroseconds)
}

func Start(tcpServer contracts.TcpServer) error {
	go tcpServer.Start()

	log.Println("Server started. Press Ctrl+C to exit")
	exitChannel := tcpServer.GetExitChannel() // Get the exit channel to listen for exit signals
	log.Println("Exit channel configured.")
	<-exitChannel // Wait for an exit signal to be received
	log.Println("Received signal to exit.")

	tcpServer.Stop()
	return nil
}

func Stop(tcpServer contracts.TcpServer) {
	tcpServer.Stop()
}

func main() {
	validator := validator.NewAmountValidator()
	requestHandler := server.NewRequestHandler(validator, make(chan struct{}))
	tcpServer, err := server.NewTcpServer(requestHandler)
	if err != nil {
		log.Printf("Error creating server: %v\n", err)
		return
	}

	if err := Start(tcpServer); err != nil {
		log.Printf("Error starting server: %v\n", err)
	}
}
