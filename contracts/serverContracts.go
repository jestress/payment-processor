package contracts

import "os"

// Defines methods for a TCP server.
type TcpServer interface {

	// Returns the channel that can be used to signal the server to clean-up and terminate.
	GetExitChannel() chan os.Signal

	// Starts the TCP server.
	Start() error

	// Stops the TCP server, closing the listener and stopping accepting new connections.
	Stop()
}

// Defines methods for a request handler.
type RequestHandler interface {

	// Returns the channel that the handler listens for receiving termination signals for the active request.
	GetRequestTerminateChannel() chan struct{}

	// Processes the incoming request and returns response.
	HandleRequest(request string) string
}
