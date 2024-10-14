package config

import "time"

const (
	AmountLimitForAsyncProcessing    int           = 100                            // Request that contain amount greater than this value will be processed asynchronously
	ListenerPortnumber               int           = 8080                           // Port number that the server listens on
	MaximumProcessingTime            int           = 10000                          // Maximum processing time in milliseconds for a request
	ReadTimeoutForActiveRequest      time.Duration = time.Duration(4) * time.Second // Amount of time that the server will wait for a request to be read from the connection
	TimeoutDurationForActiveRequests time.Duration = time.Duration(3) * time.Second // Amount of time that the server will allow active requests to complete before shutting down
)
