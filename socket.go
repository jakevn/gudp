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
	newConn chan<- *conn
	NewConn <-chan *conn
	disconn chan<- *conn
	Disconn <-chan *conn
	recv    chan<- *packet
	Recv    <-chan *packet
	approve chan<- *unconnPacket
	Approve <-chan *unconnPacket
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
		Events:    newEvents(),
	}
	go s.poll()

	return s, err
}

func newEvents() *events {
	newConnChan := make(chan *conn)
	disconnChan := make(chan *conn)
	recvChan := make(chan *packet)
	approveChan := make(chan *unconnPacket)
	return &events{
		newConn: newConnChan,
		NewConn: newConnChan,
		disconn: disconnChan,
		Disconn: disconnChan,
		recv:    recvChan,
		Recv:    recvChan,
		approve: approveChan,
		Approve: approveChan,
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

func (s *socket) Close() error {
	return s.udp.Close()
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

func (s *socket) incomingConn(addr *net.UDPAddr, approvalData []byte) {
	s.inConn[addr] = time.Now()
	s.Events.approve <- &unconnPacket{
		from: addr,
		data: approvalData,
	}
}
