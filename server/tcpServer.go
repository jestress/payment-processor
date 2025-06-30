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

	"github.com/jestress/payment-processor/config"
	"github.com/jestress/payment-processor/contracts"
	"github.com/jestress/payment-processor/validator"
)

type TcpServer struct {
	inChan       chan net.Conn  // Channel to signal the server to handle incoming connection
	exitChan     chan os.Signal // Channel to listen for exit signals such as Ctrl+C
	rqtChan      chan struct{}  // Channel to signal the server to gracefully terminate active requests
	shutdownChan chan struct{}  // Channel to signal the server to cleanup and shutdown
	conns        map[net.Conn]struct{}
	l            net.Listener
	rh           contracts.RequestHandler
	mut          sync.Mutex
	svWg         sync.WaitGroup
}

func NewTcpServer(
	rh contracts.RequestHandler,
) (*TcpServer, error) {
	exitChannel := make(chan os.Signal, 2)
	signal.Notify(exitChannel, syscall.SIGINT, syscall.SIGTERM) // Modify the flags for signaling the exit channel with further commands

	listener, err := net.Listen("tcp", fmt.Sprintf("localhost:%d", config.ListenerPortnumber))
	if err != nil {
		return nil, err
	}

	return &TcpServer{
		inChan:       make(chan net.Conn),
		conns:        make(map[net.Conn]struct{}),
		exitChan:     exitChannel,
		rh:           rh,
		rqtChan:      make(chan struct{}),
		shutdownChan: make(chan struct{}),
		l:            listener,
	}, nil
}

// Returns the channel that the server listens for receiving exit signals. Returned channel can be invoked for sending exit signals to the server.
func (s *TcpServer) GetExitChannel() chan os.Signal {
	return s.exitChan
}

func (s *TcpServer) Start() error {
	s.svWg.Add(2)            // Add two goroutines to the server WaitGroup. One for accepting connections and one for handling connections
	go s.acceptConnections() // Start accepting connections
	go s.handleConnections() // Start handling connections
	return nil
}

func (s *TcpServer) Stop() {
	log.Println("Server|CLOSE_LISTENER|Server closing the listener port.")
	err := s.l.Close()
	if err != nil {
		log.Printf("Server|Error closing listener: %v\n", err)
	}

	log.Println("Server|INVOKE_SHUTDOWN|Server invoking the shutdown channel.")
	close(s.shutdownChan)

	done := make(chan struct{})
	go func() {
		log.Println("Server|WAIT_COMPLETE|Waiting for server to finish up.")
		s.svWg.Wait()

		log.Println("Server|FINISHED|Server has finished up.")
		close(done)
	}()

	activeConnectionCount := s.getActiveConnectionCount()
	if len(s.conns) == 0 {
		log.Println("Server|NO_WAIT|No active connections. Shutting down.")
		return
	}

	log.Printf("Server|WAIT_ACTIVE_CONNECTIONServer|Allowing following %v connection(s) to complete before shutting down.\n", activeConnectionCount)
	s.activeConns()
	select {
	case <-done:
		return
	case <-time.After(config.TimeoutDurationForActiveRequests): // Allow active connections to complete within a certain duration before shutting down
		log.Println("Server|Terminating active requests.")
		close(s.rqtChan)
	}
}

func (s *TcpServer) acceptConnections() {
	defer s.svWg.Done() // Signal the server WaitGroup that the main server loop is closed
	log.Printf("Server|Listener: %s|Server started accepting connections.\n", s.l.Addr())

	for {
		select {
		case <-s.shutdownChan:
			log.Println("Server|Signal received on shutdown channel. Exiting main server loop.")
			return
		default:
			conn, err := s.l.Accept() // Wait for incoming connection
			if err != nil {
				continue
			}

			s.inChan <- conn // Signal the active connection channel to handle the incoming connection
		}
	}
}

func (s *TcpServer) handleConnections() {
	defer s.svWg.Done()

	for {
		select {
		case <-s.rqtChan:
			log.Println("Server|Signal received on shutdown channel.")
			return
		case conn := <-s.inChan:
			log.Printf("Server|C:%s|An incoming connection signaled the connection listener channel.\n", conn.RemoteAddr())
			s.addConnection(conn)
			go s.handleConnection(conn)
		}
	}
}

func (s *TcpServer) addConnection(conn net.Conn) {
	s.mut.Lock()
	defer s.mut.Unlock()
	s.conns[conn] = struct{}{} // Add the connection to the connections table of the server
}

func (s *TcpServer) removeConnection(conn net.Conn) {
	s.mut.Lock()
	defer s.mut.Unlock()
	delete(s.conns, conn) // Remove the connection from the connections table of the server
}

func (s *TcpServer) getActiveConnectionCount() int {
	s.mut.Lock()
	defer s.mut.Unlock()
	return len(s.conns)
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

	refresh(conn)
	scanner := bufio.NewScanner(conn)
	for scanner.Scan() { // Wait for client to send a request into the request stream
		if err := scanner.Err(); err != nil {
			log.Printf("Server||C:%s|Error reading from connection: %s", remoteAddr, err)
			break
		}

		req := scanner.Text()
		log.Printf("Server||C:%s||Request received: %s\n", remoteAddr, req)
		refresh(conn) // As soon as a request is received, refresh the read deadline on the connection
		start := time.Now()
		rh := NewRequestHandler(validator.NewAmountValidator(), s.rqtChan)
		res := rh.HandleRequest(req)
		duration := time.Since(start)
		log.Printf("Server||C:%s||Request: %s||Writing response: %s||Duration:%v\n\n", remoteAddr, req, res, duration)
		_, err := fmt.Fprintf(conn, "%s\n", res)
		if err != nil {
			log.Printf("Server|C:%s|Error writing response: %s.\n", remoteAddr, err.Error())
		}
	}
}

// Refresh the read deadline on the connection to ensure that the server waits for the client to send a request.
func refresh(conn net.Conn) {
	readErr := conn.SetReadDeadline(time.Now().Add(config.ReadTimeoutForActiveRequest))
	if readErr != nil {
		log.Printf("Server|C:%s|Error setting read deadline: %v\n", conn.RemoteAddr(), readErr)
	}
}

func (s *TcpServer) activeConns() {
	for conn := range s.conns {
		log.Printf("Server|Active connection: %s\n", conn.RemoteAddr())
	}
}
