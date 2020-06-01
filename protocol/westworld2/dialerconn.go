package westworld2

import (
	"github.com/michaelquigley/dilithium/util"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"net"
	"time"
)

type dialerConn struct {
	conn     *net.UDPConn
	peer     *net.UDPAddr
	seq      *util.Sequence
	txPortal *txPortal
	rxPortal *rxPortal
	pool     *pool
}

func newDialerConn(conn *net.UDPConn, peer *net.UDPAddr) *dialerConn {
	dc := &dialerConn{
		conn: conn,
		peer: peer,
		seq:  util.NewSequence(0),
		pool: newPool("dialerConn"),
	}
	dc.txPortal = newTxPortal(conn, peer)
	dc.rxPortal = newRxPortal(dc.txPortal.txAcks)
	return dc
}

func (self *dialerConn) Read(p []byte) (int, error) {
	select {
	case rxdr, ok := <-self.rxPortal.rxDataQueue:
		if !ok {
			return 0, errors.New("closed")
		}
		n := copy(p, rxdr.buf[:rxdr.sz])
		self.rxPortal.rxDataPool.Put(rxdr.buf)
		return n, nil
	default:
		// no data
	}

	for {
		wm, _, err := readWireMessage(self.conn, self.pool)
		if err != nil {
			return 0, errors.Wrap(err, "read")
		}

		if wm.mt == DATA {
			if wm.ack != -1 {
				self.txPortal.rxAcks <- wm.ack
			}
			self.rxPortal.rxWmQueue <- wm

			select {
			case rxdr, ok := <- self.rxPortal.rxDataQueue:
				if !ok {
					return 0, errors.New("closed")
				}
				n := copy(p, rxdr.buf[:rxdr.sz])
				self.rxPortal.rxDataPool.Put(rxdr.buf)
				return n, nil
			default:
				// no data
			}

		} else if wm.mt == ACK {
			if wm.ack != -1 {
				self.txPortal.rxAcks <- wm.ack
			}
			wm.buffer.unref()

		} else {
			wm.buffer.unref()
			return 0, errors.Errorf("invalid mt [%d]", wm.mt)
		}
	}
}

func (self *dialerConn) Write(p []byte) (int, error) {
	self.txPortal.txQueue <- newData(self.seq.Next(), p, self.pool)

	select {
	case err := <-self.txPortal.txErrors:
		return 0, err
	default:
		// no error
	}

	return len(p), nil
}

func (self *dialerConn) Close() error {
	return nil
}

func (self *dialerConn) RemoteAddr() net.Addr {
	return self.peer
}

func (self *dialerConn) LocalAddr() net.Addr {
	return self.conn.LocalAddr()
}

func (self *dialerConn) SetDeadline(t time.Time) error {
	return nil
}

func (self *dialerConn) SetReadDeadline(t time.Time) error {
	return nil
}

func (self *dialerConn) SetWriteDeadline(t time.Time) error {
	return nil
}

func (self *dialerConn) hello() error {
	helloSeq := self.seq.Next()
	hello := newHello(helloSeq, self.pool)
	defer hello.buffer.unref()

	if err := writeWireMessage(hello, self.conn, self.peer); err != nil {
		return errors.Wrap(err, "write hello")
	}
	logrus.Infof("{hello} -> [%s]", self.peer)

	if err := self.conn.SetReadDeadline(time.Now().Add(5 * time.Second)); err != nil {
		return errors.Wrap(err, "set read deadline")
	}

	helloAck, _, err := readWireMessage(self.conn, self.pool)
	if err != nil {
		return errors.Wrap(err, "read hello ack")
	}
	defer helloAck.buffer.unref()

	if helloAck.mt != HELLO {
		return errors.Wrap(err, "unexpected response")
	}
	if helloAck.ack != helloSeq {
		return errors.New("invalid hello ack")
	}
	if err := self.conn.SetReadDeadline(time.Time{}); err != nil {
		return errors.Wrap(err, "clear read deadline")
	}
	logrus.Infof("{helloack} <- [%s]", self.peer)

	self.rxPortal.accepted = helloAck.seq

	ack := newAck(helloAck.seq, self.pool)
	defer ack.buffer.unref()

	if err := writeWireMessage(ack, self.conn, self.peer); err != nil {
		return errors.Wrap(err, "write ack")
	}
	logrus.Infof("{ack} -> [%s]", self.peer)

	logrus.Infof("connection established, peer [%s]", self.peer)

	return nil
}