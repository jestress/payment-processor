package server

import (
	"bufio"
	"context"
	"fmt"
	"log"
	"net"
	"sync"
	"time"

	"github.com/jestress/payment-processor/config"
	"github.com/jestress/payment-processor/contracts"
	"github.com/jestress/payment-processor/validator"
)

type TcpServer struct {
	ctx     context.Context
	cnFn    context.CancelFunc
	inChan  chan net.Conn // Channel to signal the server to handle incoming connection
	rqtChan chan struct{} // Channel to signal the server to gracefully terminate active requests
	conns   map[net.Conn]struct{}
	l       net.Listener
	rh      contracts.RequestHandler
	mut     sync.Mutex
	svWg    sync.WaitGroup
}

func NewTcpServer(rh contracts.RequestHandler, ctx context.Context, cn context.CancelFunc) (*TcpServer, error) {
	listener, err := net.Listen("tcp", fmt.Sprintf("localhost:%d", config.ListenerPortnumber))
	if err != nil {
		return nil, err
	}

	return &TcpServer{
		ctx:     ctx,
		cnFn:    cn,
		inChan:  make(chan net.Conn),
		conns:   make(map[net.Conn]struct{}),
		rh:      rh,
		rqtChan: make(chan struct{}),
		l:       listener,
	}, nil
}

func (s *TcpServer) Start() error {
	s.svWg.Add(2)
	go s.accept()
	go s.handle()

	go func() {
		<-s.ctx.Done()
		log.Println("Server|CTX_DONE|Context cancelled, triggering server shutdown...")
		s.Stop()
	}()

	return nil
}

func (s *TcpServer) Stop() {
	log.Println("Server|CLOSE_LISTENER|Server closing the listener port.")
	err := s.l.Close()
	if err != nil {
		log.Printf("Server|Error closing listener: %v\n", err)
	}

	log.Println("Server|INVOKE_SHUTDOWN|Server invoking the shutdown channel.")
	s.cnFn()

	done := make(chan struct{})
	go func() {
		log.Println("Server|WAIT_COMPLETE|Waiting for server to finish up.")
		s.svWg.Wait()

		log.Println("Server|FINISHED|Server has finished up.")
		close(done)
	}()

	activeConns := s.getConns()
	if len(s.conns) == 0 {
		log.Println("Server|NO_WAIT|No active connections. Shutting down.")
		return
	}

	log.Printf("Server|WAIT_ACTIVE_CONNECTIONServer|Allowing following %v connection(s) to complete before shutting down.\n", activeConns)
	s.activeConns()
	select {
	case <-done:
		return
	case <-time.After(config.TimeoutDurationForActiveRequests): // Allow active connections to complete within a certain duration before shutting down
		log.Println("Server|Terminating active requests.")
		close(s.rqtChan)
	}
}

func (s *TcpServer) accept() {
	defer s.svWg.Done() // Signal the server WaitGroup that the main server loop is closed
	log.Printf("Server|Listener: %s|Server started accepting connections.\n", s.l.Addr())

	for {
		select {
		case <-s.ctx.Done():
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

func (s *TcpServer) handle() {
	defer s.svWg.Done()

	for {
		select {
		case <-s.rqtChan:
			log.Println("Server|Signal received on shutdown channel.")
			return
		case conn := <-s.inChan:
			log.Printf("Server|C:%s|An incoming connection signaled the connection listener channel.\n", conn.RemoteAddr())
			s.add(conn)
			go s.handleConn(conn)
		}
	}
}

func (s *TcpServer) add(conn net.Conn) {
	s.mut.Lock()
	defer s.mut.Unlock()
	s.conns[conn] = struct{}{} // Add the connection to the connections table of the server
}

func (s *TcpServer) remove(conn net.Conn) {
	s.mut.Lock()
	defer s.mut.Unlock()
	delete(s.conns, conn) // Remove the connection from the connections table of the server
}

func (s *TcpServer) getConns() int {
	s.mut.Lock()
	defer s.mut.Unlock()
	return len(s.conns)
}

func (s *TcpServer) handleConn(conn net.Conn) {
	remoteAddr := conn.RemoteAddr()
	log.Printf("Server|C:%s|Handling incoming connection.\n", remoteAddr)

	defer func() {
		s.remove(conn)
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
		rh := NewRequestHandler(s.ctx, validator.NewAmountValidator())
		res := rh.HandleRequest(req)
		duration := time.Since(start)
		log.Printf("Server||C:%s||Request: %s||Writing response: %s||Duration:%v\n\n", remoteAddr, req, res, duration)
		_, err := fmt.Fprintf(conn, "%s\n", res)
		if err != nil {
			log.Printf("Server|C:%s|Error writing response: %s.\n", remoteAddr, err.Error())
		}
	}
}

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
