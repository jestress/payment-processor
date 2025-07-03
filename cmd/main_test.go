package cmd

import (
	"bufio"
	"errors"
	"fmt"
	"log"
	"math/rand/v2"
	"net"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/jestress/payment-processor/config"
	"github.com/jestress/payment-processor/server"
	"github.com/jestress/payment-processor/validator"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/net/context"
)

type TestScheme struct {
	name           string
	amount         int
	input          string
	expectedOutput string
	minDuration    time.Duration
	maxDuration    time.Duration
}

// Global variables for server shutdown synchronization during concurrent request/connection execution
var shutdownSynchronizationMutex sync.Mutex
var shutdownInitiated bool
var serverShutdownInitiationTime time.Time
var serverShutdownTime time.Time

func setupServer(t *testing.T) (*server.TcpServer, context.CancelFunc) {
	ctx, cn := context.WithCancel(t.Context())
	activeTest := t.Name()
	validator := validator.NewAmountValidator()
	requestHandler := server.NewRequestHandler(ctx, validator)
	tcpServer, err := server.NewTcpServer(requestHandler, ctx, cn)
	shutdownSynchronizationMutex = sync.Mutex{}
	if err != nil {
		panic(err)
	}

	// Register cleanup function
	t.Cleanup(func() {
		log.Printf("%s|Cleaning up.\n", activeTest)
		cn()
	})

	log.Printf("%s|Starting server\n", activeTest)
	go tcpServer.Start()

	// Wait for the server to get ready
	time.Sleep(time.Second)

	return tcpServer, cn
}

func Test_PaymentProcessing_BasicScenarios(t *testing.T) {
	setupServer(t)
	tests := []TestScheme{
		{
			name:           "Valid Request",
			input:          "PAYMENT|10",
			expectedOutput: "RESPONSE|ACCEPTED|Transaction processed",
			maxDuration:    50 * time.Millisecond,
		},
		{
			name:           "Valid Request with Delay",
			input:          "PAYMENT|101",
			expectedOutput: "RESPONSE|ACCEPTED|Transaction processed",
			minDuration:    101 * time.Millisecond,
			maxDuration:    151 * time.Millisecond,
		},

		{
			name:           "Invalid Request Format",
			input:          "INVALID|100",
			expectedOutput: "RESPONSE|REJECTED|Invalid request",
			maxDuration:    10 * time.Millisecond,
		},
		{
			name:           "Large Amount",
			input:          "PAYMENT|10000",
			expectedOutput: "RESPONSE|ACCEPTED|Transaction processed",
			minDuration:    10 * time.Second,
			maxDuration:    10*time.Second + 50*time.Millisecond,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			conn, err := net.Dial("tcp", ":8080")
			require.NoError(t, err, "Failed to connect to server")
			defer conn.Close()

			_, err = fmt.Fprintf(conn, "%s", tt.input+"\n")
			require.NoError(t, err, "Failed to send request")

			start := time.Now()

			response, err := bufio.NewReader(conn).ReadString('\n')
			require.NoError(t, err, "Failed to read response")
			duration := time.Since(start)

			response = strings.TrimSpace(response)

			assert.Equal(t, tt.expectedOutput, response, "Unexpected response")

			if tt.minDuration > 0 {
				assert.GreaterOrEqual(t, duration, tt.minDuration, "Response time was shorter than expected")
			}

			if tt.maxDuration > 0 {
				assert.LessOrEqual(t, duration, tt.maxDuration, "Response time was longer than expected")
			}
		})
	}
}

func Test_PaymentProcessing_InvalidRequest_RequestRejected(t *testing.T) {
	setupServer(t)

	log.Println("Connecting to server")
	conn, err := net.Dial("tcp", ":8080")
	if err != nil {
		log.Println("Error connecting to server: ", err)
	}
	require.NoError(t, err, "Failed to connect to server")
	defer conn.Close()

	log.Printf("C:%s|S:%s| Sending request.\n", conn.LocalAddr(), conn.RemoteAddr())
	_, err = fmt.Fprintf(conn, "INVALID\n")
	require.NoError(t, err, "Failed to send request")
	response, err := bufio.NewReader(conn).ReadString('\n')
	require.NoError(t, err, "Failed to read response")
	response = strings.TrimSpace(response)

	assert.Equal(t, "RESPONSE|REJECTED|Invalid request", response, "Unexpected response")
}

func Test_PaymentProcessing_NonEscapedRequest_ServerTimesOutRequest(t *testing.T) {
	setupServer(t)
	conn, err := net.Dial("tcp", ":8080")
	require.NoError(t, err, "Failed to connect to server")
	defer conn.Close()

	_, err = fmt.Fprintf(conn, "PAYMENT|100")
	require.NoError(t, err, "Failed to send request")
	_, err = bufio.NewReader(conn).ReadString('\n')
	assert.Error(t, err, "Expected error when sending non-escaped request")
}

func Test_PaymentProcessing_InvalidAmount_RequestRejected(t *testing.T) {
	setupServer(t)
	conn, err := net.Dial("tcp", ":8080")
	require.NoError(t, err, "Failed to connect to server")
	defer conn.Close()

	_, err = fmt.Fprintf(conn, "PAYMENT|INVALID\n")
	require.NoError(t, err, "Failed to send request")
	response, err := bufio.NewReader(conn).ReadString('\n')
	require.NoError(t, err, "Failed to read response")
	response = strings.TrimSpace(response)

	assert.Equal(t, "RESPONSE|REJECTED|Invalid amount", response, "Unexpected response")
}

func Test_PaymentProcessing_InvalidConnectionToTheServer_ConnectionReturnsError(t *testing.T) {
	setupServer(t)
	conn, err := net.Dial("tcp", ":8080")
	require.NoError(t, err, "Failed to connect to server")
	defer conn.Close()
	_, err = fmt.Fprintf(conn, "PAYMENT|10\n")
	require.NoError(t, err, "Failed to send request")
	conn.Close()

	_, err = fmt.Fprintf(conn, "PAYMENT|10\n")
	assert.Error(t, err, "Expected error when sending request to closed connection")
}

func Test_PaymentProcessing_SendRequestToInvalidPort_ConnectionFails(t *testing.T) {
	setupServer(t)
	conn, err := net.Dial("tcp", ":8081")
	assert.Error(t, err, "Expected error when connecting to invalid port")
	assert.Nil(t, conn, "Expected connection to be nil")
}

func Test_PaymentProcessing_ConnectDuringServerShutdown_ServerReturnsResponseWhileGracefullyShuttingDown(t *testing.T) {
	testScheme := createTestScheme(1500) // Should take 1500 milliseconds to process, which is within the servers shutdown grace period
	_, cn := setupServer(t)

	conn, err := net.Dial("tcp", ":8080")
	require.NoError(t, err, "Failed to connect to server")
	defer conn.Close()

	cn()

	time.Sleep(time.Second) // Wait for shutdown to count one second down

	_, err = fmt.Fprintf(conn, "%s", testScheme.input)
	require.NoError(t, err, "Failed to send request")

	start := time.Now()
	response, err := bufio.NewReader(conn).ReadString('\n')
	require.NoError(t, err, "Failed to read response")
	duration := time.Since(start)
	response = strings.TrimSpace(response)

	assertSuccessfulResult(t, testScheme, duration, response)
}

func Test_PaymentProcessing_ConnectAfterServerShutdown_ServerConnectionReturnsError(t *testing.T) {
	_, cn := setupServer(t)
	cn()

	time.Sleep(5 * time.Second) // Wait for servers shutdown time-frame to expire

	conn, err := net.Dial("tcp", ":8080")

	assert.Error(t, err, "Expected error when connecting to closed server")
	assert.Nil(t, conn, "Expected connection to be nil")
}

func Test_PaymentProcessing_ActiveRequestDuringServerShutdown_RequestIsProcessedBeforeServerShutsDown_ServerReturnsResponse(t *testing.T) {
	_, cn := setupServer(t)

	conn, err := net.Dial("tcp", ":8080")
	require.NoError(t, err, "Failed to connect to server")
	defer conn.Close()
	cn()

	_, err = fmt.Fprintf(conn, "PAYMENT|10\n")
	require.NoError(t, err, "Failed to send request")
	response, err := bufio.NewReader(conn).ReadString('\n')
	require.NoError(t, err, "Failed to read response")
	response = strings.TrimSpace(response)

	assert.Equal(t, "RESPONSE|ACCEPTED|Transaction processed", response, "Unexpected response")
}

func Test_PaymentProcessing_ActiveRequestDuringServerShutdown_RequestIsNotProcessedBeforeServerShutsDown_ServerReturnsRejection(t *testing.T) {
	_, cn := setupServer(t)
	conn, err := net.Dial("tcp", ":8080")
	require.NoError(t, err, "Failed to connect to server")
	defer conn.Close()
	_, err = fmt.Fprintf(conn, "PAYMENT|10000\n")
	require.NoError(t, err, "Failed to send request")

	cn()

	response, err := bufio.NewReader(conn).ReadString('\n')
	require.NoError(t, err, "Failed to read response")
	response = strings.TrimSpace(response)

	assert.Equal(t, "RESPONSE|REJECTED|Request cancelled", response, "Unexpected response")
}

func Test_PaymentProcessing_SendConsecutiveRequestsOnTheSameConnection_ServerReturnsResponse(t *testing.T) {
	setupServer(t)
	conn, err := net.Dial("tcp", ":8080")
	require.NoError(t, err, "Failed to connect to server")
	defer conn.Close()

	for i := 0; i < 10; i++ {
		testScheme := createRandomizedTestScheme(500)
		_, err = fmt.Fprintf(conn, "%s", testScheme.input)
		require.NoError(t, err, "Failed to send request")

		start := time.Now()
		response, err := bufio.NewReader(conn).ReadString('\n')
		require.NoError(t, err, "Failed to read response")
		duration := time.Since(start)
		response = strings.TrimSpace(response)

		assertSuccessfulResult(t, testScheme, duration, response)
	}
}

func Test_PaymentProcessing_SendConsecutiveRequestsOnTheSameConnection_ShutdownTriggered_ServerGracefullyShutsdown(t *testing.T) {
	_, cn := setupServer(t)
	conn, err := net.Dial("tcp", ":8080")
	require.NoError(t, err, "Failed to connect to server")
	defer conn.Close()

	shutdownInitiated := false
	var serverShutdownTime time.Time
	amountOfRequestsToSpawn := 50                      // Change this value to as many requests as you want to spawn
	iterationToShutdown := amountOfRequestsToSpawn / 2 // Change this value to the iteration you want to trigger the shutdown

	for i := 1; i < amountOfRequestsToSpawn; i++ {
		testScheme := createRandomizedTestScheme(1000)
		if i == iterationToShutdown {
			shutdownInitiated = true
			shutdownInitiationTime := time.Now()
			cn()
			serverShutdownTime = shutdownInitiationTime.Add(config.TimeoutDurationForActiveRequests)
		}

		_, err = fmt.Fprintf(conn, "%s", testScheme.input)
		require.NoError(t, err, "Failed to send request")

		requestStart := time.Now()
		response, err := bufio.NewReader(conn).ReadString('\n')
		if err != nil {
			fmt.Printf("Error reading response: %v\n", err)
		}

		start := time.Now()
		require.NoError(t, err, "Failed to read response")
		duration := time.Since(requestStart)
		response = strings.TrimSpace(response)

		if shutdownInitiated && start.After(serverShutdownTime) {
			// Allow bleeding edge scenarios where timer precisions are in the boundary of 50 milliseconds
			if response != "RESPONSE|REJECTED|Request cancelled" && (start.Sub(serverShutdownTime) < 50*time.Millisecond) {
				assertSuccessfulResult(t, testScheme, duration, response)
				continue
			}
			assert.Equal(t, "RESPONSE|REJECTED|Request cancelled", response, "Unexpected response")
		} else {
			assertSuccessfulResult(t, testScheme, duration, response)
		}
	}
}

func Test_PaymentProcessing_SendFiftyConcurrentValidRequests_ProcessesSuccessfully(t *testing.T) {
	setupServer(t)
	var wg sync.WaitGroup
	wg.Add(50)

	for i := 0; i < 50; i++ {
		go func() {
			defer wg.Done()

			conn, err := net.Dial("tcp", ":8080")
			require.NoError(t, err, "Failed to connect to server")
			defer conn.Close()

			_, err = fmt.Fprintf(conn, "PAYMENT|10\n")
			require.NoError(t, err, "Failed to send request")
			response, err := bufio.NewReader(conn).ReadString('\n')
			require.NoError(t, err, "Failed to read response")
			response = strings.TrimSpace(response)

			assert.Equal(t, "RESPONSE|ACCEPTED|Transaction processed", response, "Unexpected response")
		}()
	}

	wg.Wait()
}

func Test_PaymentProcessing_SendFiftyConcurrentRequestsWithRandomAmounts_ShutdownNotTriggered_ServerProcessesEveryRequest(t *testing.T) {
	setupServer(t)

	amountOfRequestsToSpawn := 50
	var wg sync.WaitGroup
	wg.Add(amountOfRequestsToSpawn)
	for i := 0; i < amountOfRequestsToSpawn; i++ {
		go func() { // Spawn each connection in a seperate goroutine to simulate concurrent connections
			defer wg.Done()
			testScheme := createRandomizedTestScheme(10000)

			conn, err := net.Dial("tcp", ":8080")
			if err != nil {
				log.Printf("Error connecting to server: %v\n", err)
				return
			}
			defer conn.Close()
			require.NoError(t, err, "Failed to connect to server")
			_, err = fmt.Fprintf(conn, "%s", testScheme.input)
			require.NoError(t, err, "Failed to send request")
			start := time.Now()
			response, err := bufio.NewReader(conn).ReadString('\n')

			require.NoError(t, err, fmt.Sprintf("Client|%s|Error reading response: %v. Iteration: %v. Request: %s", conn.LocalAddr(), err, i, testScheme.input))
			duration := time.Since(start)

			response = strings.TrimSpace(response)
			assertSuccessfulResult(t, testScheme, duration, response)
		}()
	}

	wg.Wait()
}

func Test_PaymentProcessing_SendFiftyConcurrentRequestsWithRandomAmounts_TriggerDuringExecution_ServerGracefullyShutsdown(t *testing.T) {
	_, cn := setupServer(t)

	amountOfConnectionsToSpawn := 500                     // Change this value to as many requests as you want to spawn
	iterationToShutdown := amountOfConnectionsToSpawn / 2 // Change this value to the iteration you want to trigger the shutdown
	var wg sync.WaitGroup
	wg.Add(amountOfConnectionsToSpawn)

	for i := 0; i < amountOfConnectionsToSpawn; i++ {
		go func() { // Spawn each connection in a seperate goroutine to simulate concurrent connections
			defer wg.Done()
			testScheme := createRandomizedTestScheme(10000)

			if i == iterationToShutdown {
				initiateServerShutdown()
				cn()
				log.Printf("Iteration:%d|Request:%s|Exit channel has been signaled.\n", i, testScheme.input)
			}

			startConnection := time.Now()
			conn, err := net.Dial("tcp", ":8080")
			defer func() {
				// For cases where the net.Dial returns as connection but the OS puts it into the TCP Backlog queue
				// it is possible that this conn object is terminated. This is a safety measure to ensure that the connection is not empty.
				if conn != nil {
					conn.Close()
				}
			}()

			subroutineIsShutdownInitiated := getIsShutdownInitiated()
			subroutineServerShutdownInitiationTime := getServerShutdownInitiationTime()
			if !subroutineIsShutdownInitiated {
				// If shutdown has not been initiated, then we should not have an error connecting to the server
				require.NoError(t, err, fmt.Sprintf("Client|Iteration: %v|Error connecting to server: %v\n", i, err))
			} else if startConnection.After(getServerShutdownInitiationTime()) {
				// If shutdown has been initiated, and the server has already shut down, then we should expect an error
				require.Error(t, err,
					fmt.Sprintf("Client|Shutdown initiated: %v| serverShutdownTime: %v. startConnection: %v. Iteration: %v. Request: %s",
						subroutineIsShutdownInitiated,
						subroutineServerShutdownInitiationTime,
						startConnection, i, testScheme.input))
				return
			}

			// For the scenarios where the margin between test start time and shutdown initiation time is less than 10 milliseconds
			// if the connection is terminated by the network stack (e.g. Ghost session), then we should not proceed with the test.
			if err != nil && startConnection.Sub(subroutineServerShutdownInitiationTime) < 10*time.Millisecond {
				assert.True(t, errors.Is(err, syscall.ECONNREFUSED) || errors.Is(err, syscall.ECONNRESET), fmt.Sprintf(
					"Client|Connection was terminated by the network stack: %v. Iteration: %v. Request: %s\n",
					err, i, testScheme.input))
				return
			}

			require.NoError(t, err, fmt.Sprintf(
				"Client|Error connecting to server: %v. Iteration: %v. Request: %s", err, i, testScheme.input))
			// Send the request
			_, err = fmt.Fprintf(conn, "%s", testScheme.input)
			if !subroutineIsShutdownInitiated {
				require.NoError(t, err, "Failed to send request")
			}

			// Start reading the response
			start := time.Now()
			response, err := bufio.NewReader(conn).ReadString('\n')
			response = strings.TrimSpace(response)
			duration := time.Since(start)

			// In case of ghost sessions, response reader might return an error
			if err != nil && getIsShutdownInitiated() {
				assert.ErrorIs(t, err, syscall.ECONNRESET, fmt.Sprintf(
					"Client|%s|Connection was terminated by the network stack: %v. Iteration: %v. Request: %s\n",
					conn.LocalAddr(), err, i, testScheme.input))
				return
			}
			require.NoError(t, err, fmt.Sprintf(
				"Client|Error reading response: %v. Iteration: %v. Request: %s", err, i, testScheme.input))

			// Expect a rejection if the server has shut down, allow 50 milliseconds of margin
			if duration > config.TimeoutDurationForActiveRequests+50 {
				assert.Equal(t,
					"RESPONSE|REJECTED|Request cancelled",
					response,
					fmt.Sprintf("Client|Unexpected response. Iteration: %v. Request: %s. Duration: %v", i, testScheme.input, duration),
				)
			} else {
				assertSuccessfulResult(t, testScheme, duration, response)
			}
		}()
	}

	wg.Wait()
}

// Thread-safe mechanism for getting the server shutdown time
func getServerShutdownInitiationTime() time.Time {
	shutdownSynchronizationMutex.Lock()
	defer shutdownSynchronizationMutex.Unlock()
	return serverShutdownInitiationTime
}

// Thread-safe mechanism for checking if server shutdown has been initiated
func getIsShutdownInitiated() bool {
	shutdownSynchronizationMutex.Lock()
	defer shutdownSynchronizationMutex.Unlock()
	return shutdownInitiated
}

// Thread-safe mechanism for initiating server shutdown
func initiateServerShutdown() {
	shutdownSynchronizationMutex.Lock()
	defer shutdownSynchronizationMutex.Unlock()
	shutdownInitiated = true
	serverShutdownInitiationTime = time.Now()
	// Set the server shutdown time to the current time plus the timeout duration for active requests
	serverShutdownTime = serverShutdownInitiationTime.Add(config.TimeoutDurationForActiveRequests)
}

// Creates a new payment request scenario with the specified amount as input.
func createTestScheme(amountToBeProcessed int) TestScheme {
	amount := amountToBeProcessed + 1 // Change this value to the maximum amount you want to test
	minDurationExpected := 0 * time.Millisecond
	if amount >= config.AmountLimitForAsyncProcessing {
		minDurationExpected = time.Duration(amount) * time.Millisecond
	}
	maxDurationExpected := time.Duration(amount+config.AmountLimitForAsyncProcessing) * time.Millisecond
	if amount > config.MaximumProcessingTime {
		maxDurationExpected = time.Duration(config.MaximumProcessingTime) * time.Millisecond
	}
	return TestScheme{
		amount:         amount,
		input:          fmt.Sprintf("PAYMENT|%d\n", amount),
		expectedOutput: "RESPONSE|ACCEPTED|Transaction processed",
		minDuration:    minDurationExpected,
		maxDuration:    maxDurationExpected,
	}
}

// Creates a new payment request scenario with a random amount between 1 and the maximum amount to be processed.
func createRandomizedTestScheme(maximumPaymentAmountToBeProcessed int) TestScheme {
	return createTestScheme(
		rand.IntN(maximumPaymentAmountToBeProcessed) + 1,
	)
}

func assertSuccessfulResult(t *testing.T, testScheme TestScheme, duration time.Duration, response string) {
	if testScheme.amount <= 0 {
		assert.Equal(t, "RESPONSE|REJECTED|Invalid amount", response, "Unexpected response")
	} else {
		assert.Equal(t, testScheme.expectedOutput, response, "Unexpected response")

		if testScheme.minDuration > 0 {
			assert.GreaterOrEqual(t, duration, testScheme.minDuration, "Response time was shorter than expected")
		}

		if testScheme.maxDuration > 0 {
			assert.LessOrEqual(t, duration, testScheme.maxDuration, "Response time was longer than expected")
		}
	}
}
