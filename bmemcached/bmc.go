package bmemcached

import (
	"../cachemap"
	"fmt"
	"net"
	"os"
	"time"
)

const (
	CONN_PORT = "11211"
	CONN_TYPE = "tcp"
)

type Server struct {
	listener net.Listener
	cm *cachemap.CacheMap
}

func NewServer() *Server {
	cm := cachemap.New()
	server := Server{ cm: cm }

	server.listen()

	return &server
}

func (s *Server)listen() {
	// create listener socket
	l, err := net.Listen(CONN_TYPE, ":"+CONN_PORT)
	if err != nil {
		fmt.Println("Error listening:", err.Error())
		os.Exit(1)
	}

	fmt.Println(" --- Listening on port " + CONN_PORT + " ---")

	s.listener = l
}

func (s *Server)MainLoop() {
	defer s.listener.Close()

	// main accept() loop
	for {
		conn, err := s.listener.Accept()
		if err != nil {
			fmt.Println("accept() error: ", err.Error())
			os.Exit(1)
		}

		go s.clientConnected(conn)
	}
}

func (s *Server)clientConnected(conn net.Conn) {
	// Disable read timeouts. This allows clients to stay connected indefinitely
	// but could possibly lead to a DoS. 
	err := conn.SetDeadline(*new(time.Time)) // "zero" time (forever)
	if err != nil {
		fmt.Println("Failed to disable connection timeout: ", err.Error())
		os.Exit(1)
	}

	for {
		// Make a buffer to hold incoming data.
		buf := make([]byte, 1024)
		// Read the incoming connection into the buffer.
		reqLen, err := conn.Read(buf)
		if err != nil {
			fmt.Println("Error reading:", err.Error())
		}

		fmt.Println("Read ", reqLen, " bytes")

		// Send a response back to person contacting us.
		conn.Write([]byte("Message received."))
		// Close the connection when you're done with it.
		
	}

	conn.Close()
}
