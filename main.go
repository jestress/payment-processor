package main

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
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, os.Kill)
	validator := validator.NewAmountValidator()
	requestHandler := server.NewRequestHandler(ctx, validator)

	s, err := server.NewTcpServer(requestHandler, ctx, stop)
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

	<-ctx.Done()
	log.Println("Received exit signal, stopping server...")
	stop()
}
