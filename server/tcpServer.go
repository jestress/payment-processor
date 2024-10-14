package server

import (
	"bufio"
	"fmt"
	"log"
	"net"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/form3tech-oss/interview-simulator/config"
	"github.com/form3tech-oss/interview-simulator/contracts"
	"github.com/form3tech-oss/interview-simulator/validator"
)

type TcpServer struct {
	incomingConnectionChannel chan net.Conn  // Channel to signal the server to handle incoming connection
	exitChannel               chan os.Signal // Channel to listen for exit signals such as Ctrl+C
	requestTerminationChannel chan struct{}  // Channel to signal the server to gracefully terminate active requests
	shutdownChannel           chan struct{}  // Channel to signal the server to cleanup and shutdown
	connectionsTable          map[net.Conn]struct{}
	listener                  net.Listener
	requestHandler            contracts.RequestHandler
	connectionManagerMutex    sync.Mutex
	serverWaitGroup           sync.WaitGroup
}

func NewTcpServer(
	requestHandler contracts.RequestHandler,
) (*TcpServer, error) {
	exitChannel := make(chan os.Signal, 2)
	shutdownChannel := make(chan struct{})
	requestTerminationChannel := make(chan struct{})
	signal.Notify(exitChannel, syscall.SIGINT, syscall.SIGTERM) // Modify the flags for signaling the exit channel with further commands

	listener, err := net.Listen("tcp", fmt.Sprintf("localhost:%d", config.ListenerPortnumber))
	if err != nil {
		return nil, err
	}

	return &TcpServer{
		incomingConnectionChannel: make(chan net.Conn),
		connectionsTable:          make(map[net.Conn]struct{}),
		exitChannel:               exitChannel,
		requestHandler:            requestHandler,
		requestTerminationChannel: requestTerminationChannel,
		shutdownChannel:           shutdownChannel,
		listener:                  listener,
	}, nil
}

// Returns the channel that the server listens for receiving exit signals. Returned channel can be invoked for sending exit signals to the server.
func (s *TcpServer) GetExitChannel() chan os.Signal {
	return s.exitChannel
}

func (s *TcpServer) Start() error {
	s.serverWaitGroup.Add(2) // Add two goroutines to the server WaitGroup. One for accepting connections and one for handling connections
	go s.acceptConnections() // Start accepting connections
	go s.handleConnections() // Start handling connections
	return nil
}

func (s *TcpServer) Stop() {
	log.Println("Server|CLOSE_LISTENER|Server closing the listener port.")
	err := s.listener.Close()
	if err != nil {
		log.Printf("Server|Error closing listener: %v\n", err)
	}

	log.Println("Server|INVOKE_SHUTDOWN|Server invoking the shutdown channel.")
	close(s.shutdownChannel)

	done := make(chan struct{})
	go func() {
		log.Println("Server|WAIT_COMPLETE|Waiting for server to finish up.")
		s.serverWaitGroup.Wait()

		log.Println("Server|FINISHED|Server has finished up.")
		close(done)
	}()

	activeConnectionCount := s.getActiveConnectionCount()
	if len(s.connectionsTable) == 0 {
		log.Println("Server|NO_WAIT|No active connections. Shutting down.")
		return
	}

	log.Printf("Server|WAIT_ACTIVE_CONNECTIONServer|Allowing following %v connection(s) to complete before shutting down.\n", activeConnectionCount)
	s.reportActiveConnections()
	select {
	case <-done:
		return
	case <-time.After(config.TimeoutDurationForActiveRequests): // Allow active connections to complete within a certain duration before shutting down
		log.Println("Server|Terminating active requests.")
		close(s.requestTerminationChannel)
	}
}

func (s *TcpServer) acceptConnections() {
	defer s.serverWaitGroup.Done() // Signal the server WaitGroup that the main server loop is closed
	log.Printf("Server|Listener: %s|Server started accepting connections.\n", s.listener.Addr())

	for {
		select {
		case <-s.shutdownChannel:
			log.Println("Server|Signal received on shutdown channel. Exiting main server loop.")
			return
		default:
			conn, err := s.listener.Accept() // Wait for incoming connection
			if err != nil {
				continue
			}

			s.incomingConnectionChannel <- conn // Signal the active connection channel to handle the incoming connection
		}
	}
}

func (s *TcpServer) handleConnections() {
	defer s.serverWaitGroup.Done()

	for {
		select {
		case <-s.requestTerminationChannel:
			log.Println("Server|Signal received on shutdown channel.")
			return
		case conn := <-s.incomingConnectionChannel:
			log.Printf("Server|C:%s|An incoming connection signaled the connection listener channel.\n", conn.RemoteAddr())
			s.addConnection(conn)
			go s.handleConnection(conn)
		}
	}
}

func (s *TcpServer) addConnection(conn net.Conn) {
	s.connectionManagerMutex.Lock()
	defer s.connectionManagerMutex.Unlock()
	s.connectionsTable[conn] = struct{}{} // Add the connection to the connections table of the server
}

func (s *TcpServer) removeConnection(conn net.Conn) {
	s.connectionManagerMutex.Lock()
	defer s.connectionManagerMutex.Unlock()
	delete(s.connectionsTable, conn) // Remove the connection from the connections table of the server
}

func (s *TcpServer) getActiveConnectionCount() int {
	s.connectionManagerMutex.Lock()
	defer s.connectionManagerMutex.Unlock()
	return len(s.connectionsTable)
}

func (s *TcpServer) handleConnection(conn net.Conn) {
	remoteAddr := conn.RemoteAddr()
	log.Printf("Server|C:%s|Handling incoming connection.\n", remoteAddr)

	defer func() {
		s.removeConnection(conn)
		err := conn.Close() // Close the connection when the function completes its work and returns
		if err != nil {
			log.Printf("Server|C:%s|Error closing connection: %v\n", remoteAddr, err)
		}
	}()

	refreshReadDeadlineOnConnection(conn)
	scanner := bufio.NewScanner(conn)
	for scanner.Scan() { // Wait for client to send a request into the request stream
		if err := scanner.Err(); err != nil {
			log.Printf("Server||C:%s|Error reading from connection: %s", remoteAddr, err)
			break
		}

		request := scanner.Text()
		log.Printf("Server||C:%s||Request received: %s\n", remoteAddr, request)
		refreshReadDeadlineOnConnection(conn) // As soon as a request is received, refresh the read deadline on the connection
		start := time.Now()
		requestHandler := NewRequestHandler(validator.NewAmountValidator(), s.requestTerminationChannel)
		response := requestHandler.HandleRequest(request)
		duration := time.Since(start)
		log.Printf("Server||C:%s||Request: %s||Writing response: %s||Duration:%v\n\n", remoteAddr, request, response, duration)
		_, err := fmt.Fprintf(conn, "%s\n", response)
		if err != nil {
			log.Printf("Server|C:%s|Error writing response: %s.\n", remoteAddr, err.Error())
		}
	}
}

// Refresh the read deadline on the connection to ensure that the server waits for the client to send a request.
func refreshReadDeadlineOnConnection(conn net.Conn) {
	readErr := conn.SetReadDeadline(time.Now().Add(config.ReadTimeoutForActiveRequest))
	if readErr != nil {
		log.Printf("Server|C:%s|Error setting read deadline: %v\n", conn.RemoteAddr(), readErr)
	}
}

func (s *TcpServer) reportActiveConnections() {
	for conn := range s.connectionsTable {
		log.Printf("Server|Active connection: %s\n", conn.RemoteAddr())
	}
}
