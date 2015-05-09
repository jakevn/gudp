package gudp

import "net"

type conn struct {
	udp *net.UDPAddr
}

func newConn(udp *net.UDPAddr) *conn {
	return &conn{
		udp: udp,
	}
}

func (c *conn) receive(b []byte) {

}
