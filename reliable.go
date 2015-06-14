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
	cfg               ReliableCfg
	openSendBuf       *gobit.Buf
	sendWindow        []uint16
	sendWindowTime    []uint64
	sendPackets       map[uint16]*gobit.Buf
	recvBuf           []*gobit.Buf
	recvBufSeqDist    []int32
	newestRemoteSeq   uint16
	localSeq          uint16
	ackHistory        uint64
	lastSent          time.Time
	lastRecv          time.Time
	lastAcceptedSeq   uint16
	recvSinceLastSend uint32
	createTime        time.Time
}

type ReliableCfg struct {
	BufSize        uint32
	SendWindowSize uint32
	RecvBufSize    uint32
}

func NewReliable(cfg ReliableCfg) *reliable {
	return &reliable{
		cfg:            cfg,
		sendWindow:     make([]uint16, cfg.SendWindowSize),
		sendWindowTime: make([]uint64, cfg.SendWindowSize),
		sendPackets:    map[uint16]*gobit.Buf{},
		recvBuf:        make([]*gobit.Buf, cfg.RecvBufSize),
		recvBufSeqDist: make([]int32, cfg.RecvBufSize),
		lastSent:       time.Now(),
		createTime:     time.Now(),
	}
}

func (r *reliable) SendWindowFull() bool {
	return len(r.sendWindow) == cap(r.sendWindow)
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

func (r *reliable) FlushBuf() *gobit.Buf {
	if r.openSendBuf == nil {
		return nil
	}
	r.lastSent = time.Now()
	defer func() { r.openSendBuf = nil }()
	return r.openSendBuf
}

func (r *reliable) initializeBuf() {
	r.openSendBuf = gobit.NewBuf(r.cfg.BufSize)
	r.advanceLocalSeq()

	h := r.createHeader()
	h.WriteToBitbuf(r.openSendBuf)

	r.sendWindow[len(r.sendWindow)] = h.objSequence
	r.sendWindowTime[len(r.sendWindowTime)] = h.sendTime
	r.sendPackets[h.objSequence] = r.openSendBuf
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
		sendTime:    uint64((r.createTime.UnixNano() % 1e6) / 1e3),
	}
}

func (r *reliable) AddToBuf(data []byte) {
	if r.SendWindowFull() {
		return
	}

	if r.openSendBuf == nil {
		r.initializeBuf()
	}

	if !r.openSendBuf.CanWrite(uint32(len(data) * 8)) {
		if uint32(len(data)) > r.cfg.BufSize {
			return
		}
		r.initializeBuf()
	}
	r.openSendBuf.WriteByteArray(data)
}

func (r *reliable) RouteIncoming(packet *gobit.Buf) {
	h := HeaderFromBitbuf(packet)
	dist := seqDist(h.objSequence, r.lastAcceptedSeq)

	if packet.BitSize() <= 120 {
		r.ackDelivered(h)
		// release
	} else if !seqValid(dist) {
		// release
	} else if dist != 1 {
		// bufferOutOfOrder(dist, packet, h)
	} else {
		// ackReceived(h)
		// ackDelivered(h)
		// deliver(packet)
	}
}

func (r *reliable) ackDelivered(h header) {
	for i, sendSeq := range r.sendWindow {
		dist := seqDist(sendSeq, h.ackSequence)
		if dist > 0 {
			return
		}
		if dist <= -64 {
			// disconnect
		} else if acked(h.ackHistory, dist) {
			// messageDelivered(i, h)
		} else if uint64((time.Since(r.createTime).Nanoseconds()%1e6)/1e3)-r.sendWindowTime[i] > 333 {
			// messageLost(i)
		}
	}
}

func acked(history uint64, seqDist int32) bool {
	return (history & (uint64(1) << uint64(-seqDist))) != uint64(0)
}

func seqDist(from, to uint16) int32 {
	from <<= 1
	to <<= 1
	return int32(from-to) >> 1
}

func seqValid(dist int32) bool {
	return dist > 0 && dist <= 512
}
