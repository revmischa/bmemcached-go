package main

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"sync" // for RWMutex
	"time"
)

// Start up a server and run it in a mainloop
func main() {
	server := NewServer()
	log.Fatal(server.Run())
}

// Guts below

const (
	serverPort = "11211"

	// bmemcached opcodes
	opGet    = 0x00
	opSet    = 0x01
	opDelete = 0x04

	// bmemcached status codes
	statusKeyNotFound uint16 = 0x0001
)

// Represents a memcache server with its own listener and hash table
type Server struct {
	listener net.Listener
	cm       *CacheMap
}

func NewServer() *Server {
	s := &Server{cm: NewCacheMap()}

	return s
}

// TCP listener
func (s *Server) listen() error {
	// create listener socket
	l, err := net.Listen("tcp", ":"+serverPort)
	if err != nil {
		fmt.Println("Error listening:", err.Error())
		os.Exit(1)
	}

	log.Println(" --- Listening on port " + serverPort + " ---")

	s.listener = l

	return nil
}

// Runs forever, acceping and processing clients. Can return a fatal error.
func (s *Server) Run() error {
	err := s.listen()
	if err != nil {
		return err
	}

	defer s.listener.Close()

	// main accept() loop
	for {
		conn, err := s.listener.Accept()
		if err != nil {
			return err
		}

		go s.serve(conn)
	}
}

// client connected, read packets, parse them, process commands, respond
func (s *Server) serve(conn net.Conn) {
	// Disable read timeouts. This allows clients to stay connected indefinitely
	// but could possibly lead to a DoS.
	err := conn.SetDeadline(*new(time.Time)) // "zero" time (forever)
	if err != nil {
		fmt.Println("Failed to disable connection timeout: ", err.Error())
		os.Exit(1)
	}

	defer conn.Close()

	err = nil
	for {
		err = s.handleClientCommand(conn)

		if err != nil {
			// probably EOF ...
			fmt.Println("Read error: ", err.Error())
			break
		}
	}
}

// handles a single client command and returning response. returns error on failure
func (s *Server) handleClientCommand(conn net.Conn) error {
	const (
		// minimum size of bmemcached packet
		minHeaderSize = 24
	)

	// our packet buffer
	pkt := make([]byte, 2048)
	readCount, err := io.ReadAtLeast(conn, pkt, minHeaderSize)
	if err != nil {
		return err
	}
	if readCount < minHeaderSize {
		return errors.New("Got incomplete header")
	}

	// should now have a full 24-byte header read now that we can look at

	// check magic
	magic := pkt[0]
	if magic != 0x80 {
		return errors.New("Got invalid magic in packet header")
	}

	// parse lengths of variable-length data
	keyLen := binary.BigEndian.Uint16(pkt[2:4])
	extraLen := uint8(pkt[4])
	totalLen := int(binary.BigEndian.Uint32(pkt[8:12]))
	valLen := int(totalLen) - int(keyLen) - int(extraLen) // payload tacked on the end

	// read the rest of the packet now that we know how big the variable-length bits are
	packetTotalLen := minHeaderSize + totalLen

	toReadLen := packetTotalLen - readCount
	// fmt.Println("toReadLen", toReadLen)
	if toReadLen > 0 {
		// this needs testing... test client should try sending mega sized packets

		remainingBuf := pkt[readCount:]    // store the rest of the data into the rest of pkt
		if len(remainingBuf) > toReadLen { // big enough?
			// resize packet buffer
			newBuf := make([]byte, packetTotalLen)
			copy(newBuf, pkt)
			remainingBuf = newBuf[readCount:]
		}
		// readCount, pkt, err = readAtLeast(totalLen, conn)
		readCount, err := io.ReadAtLeast(conn, remainingBuf, totalLen)
		if readCount != totalLen {
			err = errors.New("Failed to read expected packet length")
		}
		if err != nil {
			return err
		}
		pkt = append(pkt, remainingBuf...)
	}

	// extract variable-length fields (extra, key, val)
	pktVarFields := pkt[minHeaderSize:]
	extra := pktVarFields[:extraLen]
	keyEnd := int(extraLen) + int(keyLen)
	key := string(pktVarFields[extraLen:keyEnd])
	valEnd := keyEnd + int(valLen)
	val := pktVarFields[keyEnd:valEnd] // or just pkt[keyEnd:] ?

	// switch on requested operation (get/set/delete)
	opcode := pkt[1]
	err = nil // return code
	switch opcode {
	case opGet:
		val, flags, exists := s.cm.Get(key)
		sendGetResponse(conn, key, val, flags, exists)

	case opSet:
		// get extra, flags subfield
		flags := binary.BigEndian.Uint32(extra[:4]) // first 4 bytes
		s.cm.Set(key, val, Flags(flags))
		err = sendResponse(conn, 0x00, "", make([]byte, 0), 0, make([]byte, 0))

	case opDelete:
		exists := s.cm.Delete(key)
		err = sendDeleteResponse(conn, key, exists)

	default:
		errMsg := "Got unimplemented opcode " + string(opcode)
		fmt.Println(errMsg)
		err = sendErrorResponse(conn, errMsg)
	}

	return err
}

func sendGetResponse(conn net.Conn, key string, val []byte, flags Flags, exists bool) error {
	extra := make([]byte, 0)

	flagsBytes := make([]byte, 4)
	binary.BigEndian.PutUint32(flagsBytes, uint32(flags))
	extra = append(extra, flagsBytes...)
	// extra = append(extra, 0, 0, 0, 0) // expiry

	var status uint16
	if exists {
		status = 0x0000
	} else {
		status = statusKeyNotFound
	}
	return sendResponse(conn, 0x00, "", val, status, extra)
}

func sendDeleteResponse(conn net.Conn, key string, exists bool) error {
	var status uint16
	if exists {
		status = 0x0000
	} else {
		status = statusKeyNotFound
	}
	return sendResponse(conn, opDelete, key, make([]byte, 0), status, make([]byte, 0))
}

func sendResponse(conn net.Conn, opcode uint8, key string, val []byte, status uint16, extra []byte) error {
	const (
		headerBaseLen = 24
		opaqueAndCASLen = 12
	)
	// allocate response
	varLen := len(val)+len(extra)+len(key)
	totalLen := headerBaseLen + varLen
	resp := make([]byte, totalLen)

	// convert lengths to network byte order ints
	totalLenBytes := make([]byte, 4)
	binary.BigEndian.PutUint32(totalLenBytes, uint32(varLen))

	keyLenBytes := make([]byte, 2)
	binary.BigEndian.PutUint16(keyLenBytes, uint16(len(key)))

	statusBytes := make([]byte, 2)
	binary.BigEndian.PutUint16(statusBytes, status)

	extraLen := byte(len(extra))

	// header
	head := []byte{0x81, opcode, keyLenBytes[0], keyLenBytes[1], extraLen, 0}
	i := len(head)
	copy(resp[0:i], head)
	copy(resp[i:i+len(statusBytes)], statusBytes)
	i += len(statusBytes)
	copy(resp[i:i+len(totalLenBytes)], totalLenBytes)
	i += len(totalLenBytes)
	copy(resp[i:i+opaqueAndCASLen], make([]byte, opaqueAndCASLen)) // opaque + CAS
	i += opaqueAndCASLen

	// variable-length sections
	copy(resp[i:i+len(extra)], extra)
	i += len(extra)
	copy(resp[i:i+len(key)], key)
	i += len(key)
	copy(resp[i:i+len(val)], val)
	i += len(val)

	_, err := conn.Write(resp)
	return err
}

func sendErrorResponse(conn net.Conn, msg string) error {
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

	_, err := conn.Write(resp)
	return err
}


// CACHEMAP

// The actual storage of our objects. Safe for concurrent accesses.
// TODO: LRU + expiry so that memory doesn't grow forever

// store the raw flag data that was passed in from the client for a given cache item
type Flags uint32

type CacheItem struct {
	flags Flags
	val   []byte
}

type CacheMap struct {
	mu sync.RWMutex
	m  map[string]CacheItem
}

func NewCacheMap() *CacheMap {
	return &CacheMap{m: make(map[string]CacheItem)}
}

/// Mediate access with a read/write mutex

func (cm *CacheMap) Get(key string) (val []byte, fl Flags, ok bool) {
	cm.mu.RLock()
	defer cm.mu.RUnlock()
	ival, ok := cm.m[key]

	return ival.val, ival.flags, ok
}

func (cm *CacheMap) Set(key string, val []byte, flags Flags) {
	cm.mu.Lock()
	defer cm.mu.Unlock()
	cm.m[key] = CacheItem{flags: flags, val: val}
}

func (cm *CacheMap) Delete(key string) (ok bool) {
	cm.mu.Lock()
	defer cm.mu.Unlock()
	_, ok = cm.m[key]
	delete(cm.m, key)

	return ok
}
