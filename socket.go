package gudp

import (
	"errors"
	"net"
	"sync"
	"time"
)

type socket struct {
	udp       *net.UDPConn
	cfg       SocketConfig
	conns     map[*net.UDPAddr]*conn
	connsLock *sync.RWMutex
	inConn    map[*net.UDPAddr]time.Time   // Pending inbound connections awaiting approve/deny
	outConn   map[*net.UDPAddr]connAttempt // Pending outbound connections awaiting response
	inOutLock *sync.Mutex
	Events    *events
}

type packet struct {
	from *conn
	data []byte
}

type unconnPacket struct {
	from *net.UDPAddr
	data []byte
}

type connAttempt struct {
	addr    *net.UDPAddr
	retries int
	data    []byte
	time    time.Time
}

type events struct {
	newConn  chan<- *conn
	NewConn  <-chan *conn
	disconn  chan<- *conn
	Disconn  <-chan *conn
	recv     chan<- *packet
	Recv     <-chan *packet
	approve  chan<- *unconnPacket
	Approve  <-chan *unconnPacket
	connFail chan<- *net.UDPAddr
	ConnFail <-chan *net.UDPAddr
}

type SocketConfig struct {
	Address           *net.UDPAddr
	DenyIncoming      bool
	AutoAccept        bool
	MaxConn           int
	Timeout           time.Duration
	ConnTimeout       time.Duration
	ConnRetries       int
	HeartbeatInterval time.Duration
	MTU               int
}

var DefaultConfig = SocketConfig{
	MaxConn:           512,
	Timeout:           time.Second * 2,
	ConnTimeout:       time.Second,
	ConnRetries:       5,
	HeartbeatInterval: time.Millisecond * 333,
	MTU:               1400,
}

func mergeDefaultConfig(cfg SocketConfig) SocketConfig {
	if cfg.Address == nil {
		addr, err := net.ResolveUDPAddr("udp4", ":0")
		if err != nil {
			panic("Unable to resolve a UDP4 address.")
		}
		cfg.Address = addr
	}
	if cfg.MaxConn == 0 {
		cfg.MaxConn = DefaultConfig.MaxConn
	}
	if cfg.Timeout == 0 {
		cfg.Timeout = DefaultConfig.Timeout
	}
	if cfg.ConnTimeout == 0 {
		cfg.ConnTimeout = DefaultConfig.ConnTimeout
	}
	if cfg.HeartbeatInterval == 0 {
		cfg.HeartbeatInterval = DefaultConfig.ConnTimeout
	}
	if cfg.MTU < 1 {
		cfg.MTU = DefaultConfig.MTU
	}
	return cfg
}

func NewSocket(cfg *SocketConfig) (*socket, error) {
	var config SocketConfig
	if cfg == nil {
		config = DefaultConfig
	} else {
		config = mergeDefaultConfig(*cfg)
	}

	udpSock, err := net.ListenUDP("udp4", config.Address)
	if err != nil {
		return nil, err
	}

	s := &socket{
		udp:       udpSock,
		cfg:       config,
		conns:     map[*net.UDPAddr]*conn{},
		connsLock: &sync.RWMutex{},
		inConn:    map[*net.UDPAddr]time.Time{},
		outConn:   map[*net.UDPAddr]connAttempt{},
		inOutLock: &sync.Mutex{},
		Events:    newEvents(),
	}
	go s.poll()
	go s.checkForTimeouts()

	return s, err
}

func newEvents() *events {
	newConnChan := make(chan *conn, 100)
	disconnChan := make(chan *conn, 100)
	recvChan := make(chan *packet, 2000)
	approveChan := make(chan *unconnPacket, 100)
	connFailChan := make(chan *net.UDPAddr, 100)
	return &events{
		newConn:  newConnChan,
		NewConn:  newConnChan,
		disconn:  disconnChan,
		Disconn:  disconnChan,
		recv:     recvChan,
		Recv:     recvChan,
		approve:  approveChan,
		Approve:  approveChan,
		connFail: connFailChan,
		ConnFail: connFailChan,
	}
}

func (s *socket) Connect(addr *net.UDPAddr, approvalData []byte) error {
	s.connsLock.Lock()
	defer s.connsLock.Unlock()

	s.outConn[addr] = connAttempt{
		addr:    addr,
		retries: s.cfg.ConnRetries,
		data:    approvalData,
		time:    time.Now(),
	}
	_, _, err := s.udp.WriteMsgUDP(approvalData, []byte{2}, addr)
	if err != nil {
		delete(s.outConn, addr)
	}

	return err
}

func (s *socket) ApproveConnection(addr *net.UDPAddr) error {
	s.connsLock.Lock()
	defer s.connsLock.Unlock()
	s.inOutLock.Lock()
	defer s.inOutLock.Unlock()

	if _, ok := s.conns[addr]; ok {
		return errors.New("Cannot create new connection, already exists: " + addr.String())
	}

	if len(s.conns) > s.cfg.MaxConn {
		return errors.New("Cannot create new connection, would exceed limit: " + addr.String())
	}

	if _, ok := s.inConn[addr]; !ok {
		return errors.New("Cannot create new connection, not present in incoming: " + addr.String())
	}

	connTime := s.inConn[addr]

	if time.Since(connTime) > (s.cfg.ConnTimeout / 2) {
		return errors.New("Cannot create new connection, not approved within time window: " + addr.String())
	}

	delete(s.inConn, addr)

	conn := newConn(addr)
	s.conns[addr] = conn
	s.Events.newConn <- conn

	return nil
}

func (s *socket) disconnect(addr *net.UDPAddr) {
	s.connsLock.Lock()
	defer s.connsLock.Unlock()

	if conn, ok := s.conns[addr]; ok {
		delete(s.conns, addr)
		s.Events.disconn <- conn
	}
}

func (s *socket) Close() error {
	return s.udp.Close()
}

func (s *socket) send(to *net.UDPAddr, data, header []byte) {
	_, _, err := s.udp.WriteMsgUDP(data, header, to)
	if err != nil {
		// TODO: Error handling story
	}
}

func (s *socket) poll() {
	readBytes := make([]byte, s.cfg.MTU)
	oobBytes := make([]byte, s.cfg.MTU)
	for {
		bCount, oobCount, _, fromAddr, err := s.udp.ReadMsgUDP(readBytes, oobBytes)
		if err != nil {
			continue
		}

		s.connsLock.Lock()
		defer s.connsLock.Unlock()

		if conn, ok := s.conns[fromAddr]; ok {
			conn.receive(readBytes[:bCount])
		} else if oobCount == 1 && oobBytes[0] == 2 {
			s.incomingConn(fromAddr, readBytes[:bCount])
		}
	}
}

func (s *socket) retryConn(addr *net.UDPAddr) {
	if attempt, ok := s.outConn[addr]; ok {
		if attempt.retries+1 <= s.cfg.ConnRetries {
			attempt.retries++
			_, _, err := s.udp.WriteMsgUDP(attempt.data, []byte{2}, addr)
			if err != nil {
				s.Events.connFail <- addr
				delete(s.outConn, addr)
			}
		} else {
			s.Events.connFail <- addr
			delete(s.outConn, addr)
		}
	}
}

func (s *socket) checkForTimeouts() {
	for {
		s.inOutLock.Lock()
		for addr, attempt := range s.outConn {
			if time.Since(attempt.time) > s.cfg.ConnTimeout {
				s.retryConn(addr)
			}
		}
		for addr, timeRecv := range s.inConn {
			if time.Since(timeRecv) > (s.cfg.ConnTimeout / 2) {
				delete(s.inConn, addr)
			}
		}
		s.inOutLock.Unlock()
		time.Sleep(time.Millisecond * 100)
	}
}

func (s *socket) incomingConn(addr *net.UDPAddr, approvalData []byte) {
	if s.cfg.DenyIncoming || s.cfg.MaxConn > len(s.conns) {
		return
	}

	s.inOutLock.Lock()
	s.inConn[addr] = time.Now()
	s.inOutLock.Unlock()

	if s.cfg.AutoAccept {
		s.ApproveConnection(addr)
	} else {
		s.Events.approve <- &unconnPacket{
			from: addr,
			data: approvalData,
		}
	}
}
