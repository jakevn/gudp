package gudp

import (
	"time"

	"github.com/jakevn/gobit"
)

type header struct {
	objSequence uint16
	ackSequence uint16
	ackHistory  uint64
	ackTime     uint16
	sendTime    uint64
}

type reliable struct {
	openSendBuf       *gobit.Buf
	sendWindow        []uint16
	sendWindowTime    []uint32
	sendWindowCurr    uint16
	sendWindowMax     uint16
	sendPackets       map[uint16]*gobit.Buf
	recvBuf           []*gobit.Buf
	recvBufSeqDist    []int32
	recvBuffCurr      uint16
	recvBuffMax       uint16
	newestRemoteSeq   uint16
	localSeq          uint16
	ackHistory        uint64
	lastSent          time.Time
	lastRecv          time.Time
	lastAcceptedSeq   uint16
	recvSinceLastSend uint32
}

func (r *reliable) SendWindowFull() bool {
	return r.sendWindowCurr == r.sendWindowMax
}

func (r *reliable) ShouldForceAck(currTime time.Time) bool {
	return r.recvSinceLastSend > 16 || (r.recvSinceLastSend > 0 && time.Since(r.lastSent) > 33)
}

func (r *reliable) ForceAck() {
	if r.openSendBuf != nil {
		return
	}
}
