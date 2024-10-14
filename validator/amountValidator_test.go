package validator

import (
	"testing"

	"github.com/form3tech-oss/interview-simulator/messages"
)

func Test_AmountValidator_Validate(t *testing.T) {
	tests := []struct {
		name           string
		request        string
		expectedOutput int
		errorExpected  bool
		expectedError  string
	}{
		{
			name:           "Test_Validate_ValidRequest_ReturnsAmount",
			request:        messages.PaymentMarker + messages.MessageBodySeperator + "100",
			expectedOutput: 100,
			errorExpected:  false,
			expectedError:  "",
		},
		{
			name:           "Test_Validate_InvalidRequest_ReturnsError",
			request:        "invalid_request",
			expectedOutput: -1,
			errorExpected:  true,
			expectedError:  messages.InvalidRequestErrorMessage,
		},
		{
			name:           "Test_Validate_InvalidAmount_ReturnsError",
			request:        messages.PaymentMarker + messages.MessageBodySeperator + "invalid_amount",
			expectedOutput: -1,
			errorExpected:  true,
			expectedError:  messages.InvalidAmountErrorMessage,
		},
		{
			name:           "Test_Validate_ZeroAmount_ReturnsError",
			request:        messages.PaymentMarker + messages.MessageBodySeperator + "0",
			expectedOutput: -1,
			errorExpected:  true,
			expectedError:  messages.InvalidAmountErrorMessage,
		},
		{
			name:           "Test_Validate_MissingAmount_ReturnsError",
			request:        messages.PaymentMarker + messages.MessageBodySeperator,
			expectedOutput: -1,
			errorExpected:  true,
			expectedError:  messages.InvalidAmountErrorMessage,
		},
		{
			name:           "Test_Validate_NoPaymentMarker_ReturnsError",
			request:        messages.MessageBodySeperator + "100",
			expectedOutput: -1,
			errorExpected:  true,
			expectedError:  messages.InvalidRequestErrorMessage,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			validator := NewAmountValidator()
			result, error := validator.Validate(tt.request)
			if (error != nil) != tt.errorExpected {
				t.Errorf("Unexpected error during validation: %v, expected: %v", error, tt.errorExpected)
				return
			}
			if error != nil && error.Error() != tt.expectedError {
				t.Errorf("Unexpected error during validation: %v, expected: %v", error, tt.expectedError)
			}
			if result != tt.expectedOutput {
				t.Errorf("Unexpected result: %v, expected: %v", result, tt.expectedOutput)
			}
		})
	}
}
