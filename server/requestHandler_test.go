package server

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/jestress/payment-processor/config"
	"github.com/jestress/payment-processor/messages"
	"github.com/jestress/payment-processor/mock"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
)

func Test_RequestHandler_NewRequestHandler_ReturnsRequestHandler(t *testing.T) {
	// Arrange
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	// Mocks
	mock_validator := mock.NewMockValidator(ctrl)

	// Act
	result := NewRequestHandler(t.Context(), mock_validator)

	// Assert
	assert.NotNil(t, result)
	assert.Equal(t, mock_validator, result.validator)
}

func Test_RequestHandler_HandleRequest_InvalidRequest_ReturnsError(t *testing.T) {
	// Arrange
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	expectedError := fmt.Sprintf(messages.ResponseBodyPattern, messages.ResponseRejectedMarker, messages.InvalidRequestErrorMessage)
	validationError := errors.New(messages.InvalidRequestErrorMessage)
	invalidRequest := "invalid_request"

	// Mocks
	mock_validator := mock.NewMockValidator(ctrl)
	mock_validator.EXPECT().Validate(invalidRequest).Return(-1, validationError)

	// Act
	requestHandler := NewRequestHandler(t.Context(), mock_validator)
	result := requestHandler.HandleRequest(invalidRequest)

	// Assert
	assert.Equal(t, expectedError, result)
}

func Test_RequestHandler_HandleRequest_RequestWithLowerAmountThanLimit_ReturnsResultInstantly(t *testing.T) {
	// Arrange
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	expectedResponse := fmt.Sprintf(messages.ResponseBodyPattern, messages.RequestAcceptedMarker, messages.TransactionProcessedResponse)
	request := "payment|100"

	// Mocks
	mock_validator := mock.NewMockValidator(ctrl)
	mock_validator.EXPECT().Validate(request).Return(100, nil)

	// Act
	requestHandler := NewRequestHandler(t.Context(), mock_validator)
	result := requestHandler.HandleRequest(request)

	// Assert
	assert.Equal(t, expectedResponse, result)
}

func Test_RequestHandler_HandleRequest_RequestWithHigherAmountThanLimit_ReturnsResultAfterProcessing(t *testing.T) {
	// Arrange
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	amount := 1000
	minDuration := time.Duration(amount) * time.Millisecond
	maxDuration := time.Duration(amount+50) * time.Millisecond
	expectedResponse := fmt.Sprintf(messages.ResponseBodyPattern, messages.RequestAcceptedMarker, messages.TransactionProcessedResponse)
	request := fmt.Sprintf("payment|%v", amount)

	// Mocks
	mock_validator := mock.NewMockValidator(ctrl)
	mock_validator.EXPECT().Validate(request).Return(amount, nil)

	// Act
	requestHandler := NewRequestHandler(t.Context(), mock_validator)

	start := time.Now()
	result := requestHandler.HandleRequest(request)
	elapsed := time.Since(start)

	// Assert
	assert.Equal(t, expectedResponse, result)
	assert.GreaterOrEqual(t, elapsed, minDuration, "Response time was shorter than expected")
	assert.LessOrEqual(t, elapsed, maxDuration, "Response time was longer than expected")
}

func Test_RequestHandler_HandleRequest_RequestWithHigherAmountThanTenThousand_ReturnsResultAfterProcessing(t *testing.T) {
	// Arrange
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	amount := 15000
	minDuration := time.Duration(10000) * time.Millisecond
	maxDuration := time.Duration(10050) * time.Millisecond
	expectedResponse := fmt.Sprintf(messages.ResponseBodyPattern, messages.RequestAcceptedMarker, messages.TransactionProcessedResponse)
	request := fmt.Sprintf("payment|%v", amount)

	// Mocks
	mock_validator := mock.NewMockValidator(ctrl)
	mock_validator.EXPECT().Validate(request).Return(amount, nil)

	// Act
	requestHandler := NewRequestHandler(t.Context(), mock_validator)

	start := time.Now()
	result := requestHandler.HandleRequest(request)
	elapsed := time.Since(start)

	// Assert
	assert.Equal(t, expectedResponse, result)
	assert.GreaterOrEqual(t, elapsed, minDuration, "Response time was shorter than expected")
	assert.LessOrEqual(t, elapsed, maxDuration, "Response time was longer than expected")
}

func Test_RequestHandler_HandleRequest_TerminationInvokedWhileProcessing_ReturnsResponse(t *testing.T) {
	// Arrange
	ctx, cn := context.WithCancel(t.Context())
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	amount := 1000
	maxDuration := time.Duration(amount+50) * time.Millisecond
	expectedResponse := fmt.Sprintf(messages.ResponseBodyPattern, messages.RequestAcceptedMarker, messages.TransactionProcessedResponse)
	request := fmt.Sprintf(messages.PaymentRequestBodyPattern, amount)

	// Mocks
	mock_validator := mock.NewMockValidator(ctrl)
	mock_validator.EXPECT().Validate(request).Return(amount, nil)

	// Act
	requestHandler := NewRequestHandler(ctx, mock_validator)

	start := time.Now()
	result := requestHandler.HandleRequest(request)
	cn()
	elapsed := time.Since(start)

	// Assert
	assert.Equal(t, expectedResponse, result)
	assert.LessOrEqual(t, elapsed, maxDuration, "Response time was longer than expected")
}

func Test_RequestHandler_HandleRequest_TerminationInvokedWhileProcessing_RequestTerminates(t *testing.T) {
	// Arrange
	ctx, cn := context.WithCancel(t.Context())
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	amount := 5000
	timeoutDurationWithBuffer := config.TimeoutDurationForActiveRequests + 50*time.Millisecond
	expectedResponse := fmt.Sprintf(messages.ResponseBodyPattern, messages.ResponseRejectedMarker, messages.RequestCancelledResponse)
	request := fmt.Sprintf(messages.PaymentRequestBodyPattern, amount)

	// Mocks
	mock_validator := mock.NewMockValidator(ctrl)
	mock_validator.EXPECT().Validate(request).Return(amount, nil)

	// Act
	requestHandler := NewRequestHandler(ctx, mock_validator)

	go func() {
		cn()
	}()

	start := time.Now()
	result := requestHandler.HandleRequest(request)
	elapsed := time.Since(start)

	// Assert
	assert.Equal(t, expectedResponse, result)
	assert.LessOrEqual(t, elapsed, timeoutDurationWithBuffer, "Response time was longer than expected")
}
