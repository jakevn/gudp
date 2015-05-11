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

func (c *conn) receiveReliable(b []byte) {

}

func (c *conn) Send(b []byte) error {
	return nil
}

func (c *conn) SendReliable(b []byte) error {
	return nil
}
