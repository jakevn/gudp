package gudp

import (
	"net"
	"sync"
	"time"
)

type conn struct {
	udp          *net.UDPAddr
	lastSent     time.Time
	lastSentLock *sync.Mutex
}

func newConn(udp *net.UDPAddr) *conn {
	return &conn{
		udp: udp,
	}
}

func (c *conn) updateLastSent(t time.Time) {
	c.lastSentLock.Lock()
	c.lastSent = t
	c.lastSentLock.Unlock()
}

func (c *conn) receive(b []byte) {

}

func (c *conn) receiveReliable(data []byte, header []byte) {

}

func (c *conn) Send(b []byte) error {
	return nil
}

func (c *conn) SendReliable(b []byte) error {
	return nil
}
