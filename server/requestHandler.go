package server

import (
	"fmt"
	"log"
	"time"

	"github.com/form3tech-oss/interview-simulator/config"
	"github.com/form3tech-oss/interview-simulator/contracts"
	"github.com/form3tech-oss/interview-simulator/messages"
)

type RequestHandler struct {
	validator                 contracts.Validator
	requestTerminationChannel chan struct{}
}

func NewRequestHandler(validator contracts.Validator, requestTerminationChannel chan struct{}) *RequestHandler {
	return &RequestHandler{
		requestTerminationChannel: requestTerminationChannel,
		validator:                 validator,
	}
}

// Returns the channel that the handler listens for receiving termination signals for the active request.
func (rh *RequestHandler) GetRequestTerminateChannel() chan struct{} {
	return rh.requestTerminationChannel
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
		case <-rh.requestTerminationChannel:
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
