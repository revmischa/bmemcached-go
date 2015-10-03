package bmemcached

import (
	"../cachemap"
	"encoding/binary"
	"fmt"
	"net"
	"os"
	"time"
)

const (
	PORT = "11211"

	OP_GET    byte = 0x00
	OP_SET    byte = 0x01
	OP_DELETE byte = 0x04

	STATUS_KEY_NOT_FOUND = 0x0001
)

type Server struct {
	listener net.Listener
	cm       *cachemap.CacheMap
}

func NewServer() *Server {
	cm := cachemap.New()
	server := Server{cm: cm}

	server.listen()

	return &server
}

// TCP listener
func (s *Server) listen() {
	// create listener socket
	l, err := net.Listen("tcp", ":"+PORT)
	if err != nil {
		fmt.Println("Error listening:", err.Error())
		os.Exit(1)
	}

	fmt.Println(" --- Listening on port " + PORT + " ---")

	s.listener = l
}

// accept() loop
func (s *Server) MainLoop() {
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

// client connected, read packets, parse them, process commands, respond
func (s *Server) clientConnected(conn net.Conn) {
	// Disable read timeouts. This allows clients to stay connected indefinitely
	// but could possibly lead to a DoS.
	err := conn.SetDeadline(*new(time.Time)) // "zero" time (forever)
	if err != nil {
		fmt.Println("Failed to disable connection timeout: ", err.Error())
		os.Exit(1)
	}

	defer conn.Close()

	done := false
	for !done {
		done = !s.handleClientCommand(conn)
	}
}

func (s *Server) handleClientCommand(conn net.Conn) bool {
	readCount, pkt, err := readAtLeast(24, conn)

	if err != nil {
		// probably EOF
		// fmt.Println("Read error: ", err.Error())
		return false
	}

	// should now have a full 24-byte header read now that we can look at

	// check magic
	magic := pkt[0]
	if magic != 0x80 {
		fmt.Println("Got invalid magic in packet header")
		return false
	}

	// parse lengths
	keyLen := binary.BigEndian.Uint16(pkt[2:4])
	extraLen := uint8(pkt[4])
	totalLen := int(binary.BigEndian.Uint32(pkt[8:12]))
	valLen := int(totalLen) - int(keyLen) - int(extraLen)

	// read the rest of the packet now that we know how big the variable-length bits are
	payload := make([]byte, totalLen)
	readCount, payload, err = readAtLeast(totalLen, conn)
	if err != nil || readCount != totalLen {
		fmt.Println("Read error: ", err.Error())
		return false
	}

	// extract variable-length fields (extra, key, val)
	extra := payload[:extraLen]
	keyEnd := int(extraLen) + int(keyLen)
	key := string(payload[extraLen:keyEnd])
	valEnd := keyEnd + int(valLen)
	val := payload[keyEnd:valEnd]

	// switch on requested operation (get/set/delete)
	opcode := pkt[1]
	switch opcode {
	case OP_GET:
		val, flags, exists := s.cm.Get(key)
		sendGetResponse(conn, key, val, flags, exists)

	case OP_SET:
		// get extra, flags subfield
		flags := extra[:4] // first 4 bytes
		s.cm.Set(key, val, flags)
		sendResponse(conn, 0x00, "", make([]byte, 0), 0, make([]byte, 0))

	case OP_DELETE:
		exists := s.cm.Delete(key)
		sendDeleteResponse(conn, key, exists)

	default:
		errMsg := "Got unimplemented opcode " + string(opcode)
		fmt.Println(errMsg)
		sendErrorResponse(conn, errMsg)
		return true
	}

	return true
}

func sendGetResponse(conn net.Conn, key string, val []byte, flags []byte, exists bool) {
	extra := make([]byte, 0)
	extra = append(extra, flags...)
	// extra = append(extra, 0, 0, 0, 0) // expiry

	var status uint16
	if exists {
		status = 0x0000
	} else {
		status = STATUS_KEY_NOT_FOUND
	}
	sendResponse(conn, 0x00, "", val, status, extra)
}

func sendDeleteResponse(conn net.Conn, key string, exists bool) {
	var status uint16
	if exists {
		status = 0x0000
	} else {
		status = STATUS_KEY_NOT_FOUND
	}
	sendResponse(conn, OP_DELETE, key, make([]byte, 0), status, make([]byte, 0))
}

func sendResponse(conn net.Conn, opcode uint8, key string, val []byte, status uint16, extra []byte) {
	totalLen := make([]byte, 4)
	binary.BigEndian.PutUint32(totalLen, uint32(len(val)+len(extra)+len(key)))

	keyLen := make([]byte, 2)
	binary.BigEndian.PutUint16(keyLen, uint16(len(key)))

	statusBytes := make([]byte, 2)
	binary.BigEndian.PutUint16(statusBytes, status)

	extraLen := byte(len(extra))

	resp := []byte{0x81, opcode, keyLen[0], keyLen[1], extraLen, 0}
	resp = append(resp, statusBytes...)
	resp = append(resp, totalLen...)
	resp = append(resp, make([]byte, 12)...) // opaque + CAS

	resp = append(resp, extra...)
	resp = append(resp, key...)
	resp = append(resp, val...)

	conn.Write(resp)
}

// attempts to read a given number of bytes from conn
func readAtLeast(count int, conn net.Conn) (int, []byte, error) {
	if count == 0 {
		return 0, nil, nil
	}

	buf := make([]byte, count)
	read := make([]byte, 0)

	// initial read
	readCount, err := conn.Read(buf)
	if err != nil {
		// fmt.Println("Error reading (", len(buf), "):", err.Error())
		return 0, nil, err
	}

	// slice buffer at read count
	read = append(read, buf[:readCount]...)
	buf = buf[readCount:]

	for readCount < count {
		var readMoreCount int
		readMoreCount, err = conn.Read(buf)
		if err != nil {
			// fmt.Println("Error reading (", len(buf), "):", err.Error())
			return 0, nil, err
		}
		readCount += readMoreCount
		read = append(read, buf[:readCount]...)
		buf = buf[readCount:]
	}

	// I think this returns a copy of read, which is less than ideal
	return readCount, read, nil
}

func sendErrorResponse(conn net.Conn, msg string) {
	// Field        (offset) (value)
	// Magic        (0)    : 0x81
	// Opcode       (1)    : 0x00
	// Key length   (2,3)  : 0x0000
	// Extra length (4)    : 0x00
	// Data type    (5)    : 0x00
	// Status       (6,7)  : 0x0001
	// Total body   (8-11) : len(msg)
	// Opaque       (12-15): 0x00000000
	// CAS          (16-23): 0x0000000000000000
	// Extras              : None
	// Key                 : None
	// Value        (24-x) : msg

	bodyLen := make([]byte, 4)
	binary.BigEndian.PutUint32(bodyLen, uint32(len(msg)))

	resp := []byte{0x81, 0, 0, 0, 0, 0, 0, 1} // magic etc
	resp = append(resp, bodyLen...)           // body length
	resp = append(resp, make([]byte, 12)...)  // opaque + CAS

	resp = append(resp, []byte(msg)...) // stick error string on the end

	conn.Write(resp)
}
