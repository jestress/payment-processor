package contracts

// Defines methods for validating a request.
type Validator interface {

	// Validates a request and returns the amount.
	Validate(request string) (int, error)
}
