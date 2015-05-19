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

func (h header) WriteToBitbuf(b *gobit.Buf) {
	b.WriteUint16(h.objSequence)
	b.WriteUint16(h.ackSequence)
	b.WriteUint64(h.ackHistory)
	b.WriteUint16(h.ackTime)
	b.WriteUint64(h.sendTime)
}

func HeaderFromBitbuf(b *gobit.Buf) header {
	return header{
		objSequence: b.ReadUint16(),
		ackSequence: b.ReadUint16(),
		ackHistory:  b.ReadUint64(),
		ackTime:     b.ReadUint16(),
		sendTime:    b.ReadUint64(),
	}
}

type reliable struct {
	cfg               *ReliableCfg
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

type ReliableCfg struct {
	BufSize        uint32
	SendWindowSize uint32
	RecvBufSize    uint32
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
	r.openSendBuf = gobit.NewBuf(r.cfg.BufSize)
	h := r.createHeader()
	h.WriteToBitbuf(r.openSendBuf)
}

func (r *reliable) initializeBuf() {
	r.openSendBuf = gobit.NewBuf(r.cfg.BufSize)
	r.advanceLocalSeq()

	h := r.createHeader()
	h.WriteToBitbuf(r.openSendBuf)
	// ...
}

func (r *reliable) advanceLocalSeq() {
	r.localSeq++
	r.localSeq &= 32767
}

func (r *reliable) createHeader() header {
	return header{
		objSequence: r.localSeq,
		ackSequence: r.newestRemoteSeq,
		ackHistory:  r.ackHistory,
		ackTime:     uint16(time.Since(r.lastRecv)),
		sendTime:    uint64(time.Now().Unix()),
	}
}
