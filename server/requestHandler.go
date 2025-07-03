package server

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/jestress/payment-processor/config"
	"github.com/jestress/payment-processor/contracts"
	"github.com/jestress/payment-processor/messages"
)

type RequestHandler struct {
	ctx       context.Context
	validator contracts.Validator
}

func NewRequestHandler(ctx context.Context, validator contracts.Validator) *RequestHandler {
	return &RequestHandler{
		ctx:       ctx,
		validator: validator,
	}
}

func (rh *RequestHandler) HandleRequest(request string) string {
	response := messages.ResponseBodyPattern
	amount, err := rh.validator.Validate(request)
	if err != nil {
		return fmt.Sprintf(response, messages.ResponseRejectedMarker, err.Error())
	}

	if amount > config.AmountLimitForAsyncProcessing {
		processingTime := amount
		if amount > config.MaximumProcessingTime {
			processingTime = config.MaximumProcessingTime
		}
		select {
		case <-rh.ctx.Done():
			log.Printf("Server||Active Request: %s||Terminating request due to external signal.\n", request)
			return fmt.Sprintf(response, messages.ResponseRejectedMarker, messages.RequestCancelledResponse)
		case <-time.After(time.Duration(processingTime) * time.Millisecond): // Simulate a moderately long processing time
			return fmt.Sprintf(response,
				messages.RequestAcceptedMarker,
				messages.TransactionProcessedResponse,
			)
		}
	}

	return fmt.Sprintf(response,
		messages.RequestAcceptedMarker,
		messages.TransactionProcessedResponse,
	)
}
