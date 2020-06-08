package westworld2

import (
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"net"
	"time"
)

const monitorInSz = 1024
const cancelInSz = 1024

type txRetx struct {
	monitorIn chan *wireMessage
	cancelIn  chan *wireMessage
	queue     []*txRetxMonitor
	conn      *net.UDPConn
	peer      *net.UDPAddr
	ins       Instrument
}

type txRetxMonitor struct {
	deadline time.Time
	wm       *wireMessage
}

func newTxRetx(conn *net.UDPConn, peer *net.UDPAddr) *txRetx {
	tr := &txRetx{
		monitorIn: make(chan *wireMessage, monitorInSz),
		cancelIn:  make(chan *wireMessage, cancelInSz),
		conn:      conn,
		peer:      peer,
	}
	go tr.run()
	return tr
}

func (self *txRetx) run() {
	for {
		if err := self.accept(); err != nil {
			return
		}
		if err := self.cancel(); err != nil {
			return
		}

		if len(self.queue) > 0 {
			head := self.queue[0]
			time.Sleep(time.Until(head.deadline))

			if err := self.cancel(); err != nil {
				return
			}

			if len(self.queue) > 0 && head == self.queue[0] {
				if err := writeWireMessage(head.wm, self.conn, self.peer, self.ins); err != nil {
					logrus.Errorf("retx (%v)", err)
				}

				head.deadline = time.Now().Add(retxTimeoutMs * time.Millisecond)
				if len(self.queue) > 1 {
					self.queue = append(self.queue[1:], self.queue[0])
				}
			}
		}
	}
}

func (self *txRetx) accept() error {
accept:
	for {
		select {
		case wm, ok := <-self.monitorIn:
			if !ok {
				return errors.New("closed")
			}

			wm.buffer.ref()
			self.queue = append(self.queue, &txRetxMonitor{time.Now().Add(retxTimeoutMs * time.Millisecond), wm})

		default:
			break accept
		}
	}
	return nil
}

func (self *txRetx) cancel() error {
cancel:
	for {
		select {
		case wm, ok := <-self.cancelIn:
			if !ok {
				return errors.New("closed")
			}

			i := -1
		search:
			for j, c := range self.queue {
				if c.wm.seq == wm.seq {
					i = j
					break search
				}
			}

			if i > -1 {
				wm.buffer.unref()
				self.queue = append(self.queue[:i], self.queue[i+1:]...)
			}

		default:
			break cancel
		}
	}
	return nil
}