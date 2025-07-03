package cmd

import (
	"context"
	"log"
	"os"
	"os/signal"

	"github.com/jestress/payment-processor/server"
	"github.com/jestress/payment-processor/validator"
)

func init() {
	log.SetFlags(log.Ltime | log.Lmicroseconds)
}

func main() {
	validator := validator.NewAmountValidator()
	requestHandler := server.NewRequestHandler(validator, make(chan struct{}))

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, os.Kill)
	s, err := server.NewTcpServer(requestHandler, stop)
	defer stop()
	if err != nil {
		log.Printf("error creating server: %v\n", err)
		return
	}

	go func() {
		if err := s.Start(); err != nil {
			log.Printf("Server error: %v\n", err)
			stop()
		}
	}()

	if err := s.Start(); err != nil {
		log.Printf("Error starting server: %v\n", err)
	}

	<-ctx.Done() // Wait for the context to be done, which will happen on exit signal
	log.Println("Received exit signal, stopping server...")
	stop()
}
