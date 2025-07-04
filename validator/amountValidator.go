package validator

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/jestress/payment-processor/messages"
)

// AmountValidator takes a raw request and validates it to ensure it is a valid payment request.
type AmountValidator struct{}

func NewAmountValidator() *AmountValidator {
	return &AmountValidator{}
}

// Validates the request and returns the amount if the request is valid.
// Returns -1 if the request is invalid.
// Returns an error if the request is invalid.
func (v *AmountValidator) Validate(request string) (int, error) {
	parts := strings.Split(request, messages.MessageBodySeperator) // Split the request into parts
	if len(parts) != 2 || parts[0] != messages.PaymentMarker {     // Check if the request is formed correctly
		return -1, fmt.Errorf(messages.InvalidRequestErrorMessage)
	}

	amount, err := strconv.Atoi(parts[1]) // Convert the amount to an integer
	if err != nil || amount <= 0 {        // Check if the amount is a valid integer and is greater than 0
		return -1, fmt.Errorf(messages.InvalidAmountErrorMessage)
	}

	return amount, nil
}
