package messages

const (
	RequestAcceptedMarker        string = "ACCEPTED"
	RequestCancelledResponse     string = "Request cancelled"
	ResponseBodyPattern          string = "RESPONSE|%s|%s"
	ResponseRejectedMarker       string = "REJECTED"
	InvalidRequestErrorMessage   string = "Invalid request"
	InvalidAmountErrorMessage    string = "Invalid amount"
	MessageBodySeperator         string = "|"
	PaymentRequestBodyPattern    string = "PAYMENT|%v"
	PaymentMarker                string = "PAYMENT"
	TransactionProcessedResponse string = "Transaction processed"
)
