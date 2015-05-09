package gudp

import (
	"net"
	"sync"
	"time"
)

type socket struct {
	udp       *net.UDPConn
	cfg       SocketConfig
	conns     map[*net.UDPAddr]*conn
	connsLock *sync.RWMutex
	inConn    map[*net.UDPAddr]time.Duration // Pending inbound connections awaiting approve/deny
	outConn   map[*net.UDPAddr]time.Duration // Pending outbound connections awaiting response
	Events    *events
}

type packet struct {
	from *conn
	data []byte
}

type events struct {
	newConn chan<- *conn
	NewConn <-chan *conn
	disconn chan<- *conn
	Disconn <-chan *conn
	recv    chan<- *packet
	Recv    <-chan *packet
	approve chan<- *packet
	Approve <-chan *packet
}

type SocketConfig struct {
	Address   net.UDPAddr
	MaxConnIn int
}

func NewSocket(cfg SocketConfig) (*socket, error) {
	addr, err := net.ResolveUDPAddr("udp4", ":0")
	if err != nil {
		return nil, err
	}

	udpSock, err := net.ListenUDP("udp4", addr)
	if err != nil {
		return nil, err
	}

	s := &socket{
		udp:       udpSock,
		cfg:       cfg,
		conns:     map[*net.UDPAddr]*conn{},
		connsLock: &sync.RWMutex{},
		inConn:    map[*net.UDPAddr]time.Duration{},
		outConn:   map[*net.UDPAddr]time.Duration{},
		Events:    newEvents(),
	}
	go s.poll()

	return s, err
}

func newEvents() *events {
	newConnChan := make(chan *conn)
	disconnChan := make(chan *conn)
	recvChan := make(chan *packet)
	approveChan := make(chan *packet)
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

func (s *socket) Connect(addr net.UDPAddr) error {
	return nil
}

func (s *socket) ApproveConnection(addr net.UDPAddr) error {
	return nil
}

func (s *socket) Close() error {
	return s.udp.Close()
}

func (s *socket) poll() {
	readBytes := make([]byte, 1400)
	for {
		bCount, fromAddr, err := s.udp.ReadFromUDP(readBytes)
		if err != nil {
			continue
		}

		s.connsLock.Lock()
		defer s.connsLock.Unlock()

		if conn, ok := s.conns[fromAddr]; ok {
			conn.receive(readBytes[:bCount])
		} else {
			s.conns[fromAddr] = newConn(fromAddr)
		}
	}
}
