package westworld3

import (
	"github.com/michaelquigley/dilithium/util"
	"github.com/pkg/errors"
)

type wireMessage struct {
	seq    int32
	mt     messageType
	data   []byte
	buffer *buffer
}

type messageType uint8

const (
	// 0x0 ... 0x7
	HELLO messageType = iota
	ACK
	DATA
	KEEPALIVE
	CLOSE
)

const messageTypeMask = byte(0x7)

type messageFlag uint8

const (
	// 0x8 ... 0x80
	RTT        messageFlag = 0x8
	INLINE_ACK messageFlag = 0x10
)

const dataStart = 7

func newHello(seq int32, h hello, a *ack, p *pool) (wm *wireMessage, err error) {
	wm = &wireMessage{
		seq:    seq,
		mt:     HELLO,
		buffer: p.get(),
	}
	var acksSz uint32
	var helloSz uint32
	if a != nil {
		wm.setFlag(INLINE_ACK)
		acksSz, err = encodeAcks([]ack{*a}, wm.buffer.data[dataStart:])
		if err != nil {
			return nil, errors.Wrap(err, "error encoding hello ack")
		}
	}
	helloSz, err = encodeHello(h, wm.buffer.data[dataStart+acksSz:])
	return wm.encodeHeader(uint16(acksSz + helloSz))
}

func (self *wireMessage) asHello() (h hello, a []ack, err error) {
	if self.messageType() != HELLO {
		return hello{}, nil, errors.Errorf("unexpected message type [%d], expected HELLO", self.messageType())
	}
	i := uint32(0)
	if self.hasFlag(INLINE_ACK) {
		a, i, err = decodeAcks(self.data)
		if err != nil {
			return hello{}, nil, errors.Wrap(err, "error decoding acks")
		}
	}
	h, _, err = decodeHello(self.data[i:])
	if err != nil {
		return hello{}, nil, errors.Wrap(err, "error decoding hello")
	}
	return
}

func newAck(acks []ack, rxPortalSz int32, rtt *uint16, p *pool) (wm *wireMessage, err error) {
	wm = &wireMessage{
		seq:    -1,
		mt:     ACK,
		buffer: p.get(),
	}
	var rttSz uint32
	if rtt != nil {
		if wm.buffer.sz < dataStart+2 {
			return nil, errors.Errorf("short buffer for ack [%d < %d]", wm.buffer.sz, dataStart+2)
		}
		wm.setFlag(RTT)
		util.WriteUint16(wm.buffer.data[dataStart:], *rtt)
		rttSz = 2
	}
	var acksSz uint32
	if len(acks) > 0 {
		acksSz, err = encodeAcks(acks, wm.buffer.data[dataStart+rttSz:])
		if err != nil {
			return nil, errors.Wrap(err, "error encoding acks")
		}
	}
	if dataStart+rttSz+acksSz > wm.buffer.sz {
		return nil, errors.Errorf("short buffer for ack [%d < %d]", wm.buffer.sz, dataStart+acksSz)
	}
	util.WriteInt32(wm.buffer.data[dataStart+acksSz:], int32(rxPortalSz))
	return wm.encodeHeader(uint16(acksSz + 4))
}

func (self *wireMessage) asAck() (a []ack, rxPortalSz int32, rtt *uint16, err error) {
	if self.messageType() != ACK {
		return nil, 0, nil, errors.Errorf("unexpected message type [%d], expected ACK", self.messageType())
	}
	i := uint32(0)
	if self.hasFlag(RTT) {
		if self.buffer.uz < dataStart+2 {
			return nil, 0, nil, errors.Errorf("short buffer for ack decode [%d < %d]", self.buffer.uz, dataStart+2)
		}
		util.ReadUint16(self.buffer.data[dataStart:])
		i += 2
	}
	var acksSz uint32
	a, acksSz, err = decodeAcks(self.buffer.data[i:])
	if err != nil {
		return nil, 0, nil, errors.Wrap(err, "error decoding acks")
	}
	i += acksSz
	if self.buffer.uz < i+4 {
		return nil, 0, nil, errors.Errorf("short buffer for rxPortalSz decode [%d < %d]", self.buffer.uz, i+4)
	}
	rxPortalSz = util.ReadInt32(self.buffer.data[i:])
	return
}

func (self *wireMessage) encodeHeader(dataSz uint16) (*wireMessage, error) {
	if self.buffer.sz < uint32(dataStart+dataSz) {
		return nil, errors.Errorf("short buffer for encode [%d < %d]", self.buffer.sz, dataStart+dataSz)
	}
	util.WriteInt32(self.buffer.data[0:4], self.seq)
	self.buffer.data[4] = byte(self.mt)
	util.WriteUint16(self.buffer.data[5:dataStart], dataSz)
	self.buffer.uz = uint32(dataStart + dataSz)
	return self, nil
}

func decodeHeader(buffer *buffer) (*wireMessage, error) {
	sz := util.ReadUint16(buffer.data[5:dataStart])
	if uint32(dataStart+sz) > buffer.uz {
		return nil, errors.Errorf("short buffer read [%d != %d]", buffer.sz, dataStart+sz)
	}
	wm := &wireMessage{
		seq:    util.ReadInt32(buffer.data[0:4]),
		mt:     messageType(buffer.data[4]),
		data:   buffer.data[dataStart : dataStart+sz],
		buffer: buffer,
	}
	return wm, nil
}

func (self *wireMessage) insertData(data []byte) error {
	dataSz := uint16(len(data))
	if self.buffer.sz < self.buffer.uz+uint32(dataSz) {
		return errors.Errorf("short buffer for insert [%d < %d]", self.buffer.sz, self.buffer.uz+uint32(dataSz))
	}
	for i := self.buffer.uz - 1; i >= dataStart; i-- {
		self.buffer.data[i+uint32(dataSz)] = self.buffer.data[i]
	}
	for i := 0; i < int(dataSz); i++ {
		self.buffer.data[dataStart+i] = data[i]
	}
	self.buffer.uz = self.buffer.uz + uint32(dataSz)
	return nil
}

func (self *wireMessage) appendData(data []byte) error {
	dataSz := uint16(len(data))
	if self.buffer.sz < self.buffer.uz+uint32(dataSz) {
		return errors.Errorf("short buffer for append [%d < %d]", self.buffer.sz, self.buffer.uz+uint32(dataSz))
	}
	for i := 0; i < int(dataSz); i++ {
		self.buffer.data[self.buffer.uz+uint32(i)] = data[i]
	}
	self.buffer.uz = self.buffer.uz + uint32(dataSz)
	return nil
}

func (self *wireMessage) messageType() messageType {
	return messageType(byte(self.mt) & messageTypeMask)
}

func (self *wireMessage) setFlag(flag messageFlag) {
	self.mt = messageType(uint8(self.mt) | uint8(flag))
}

func (self *wireMessage) clearFlag(flag messageFlag) {
	self.mt = messageType(uint8(self.mt) ^ uint8(flag))
}

func (self *wireMessage) hasFlag(flag messageFlag) bool {
	if uint8(self.mt)&uint8(flag) > 0 {
		return true
	}
	return false
}