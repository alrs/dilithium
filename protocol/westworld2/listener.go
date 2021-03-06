package westworld2

import (
	"github.com/emirpasic/gods/trees/btree"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"net"
	"sync"
)

type listener struct {
	lock        *sync.Mutex
	peers       *btree.Tree
	acceptQueue chan net.Conn
	conn        *net.UDPConn
	addr        *net.UDPAddr
	pool        *pool
	config      *Config
	ii          InstrumentInstance
}

func Listen(addr *net.UDPAddr, config *Config) (net.Listener, error) {
	conn, err := net.ListenUDP("udp", addr)
	if err != nil {
		return nil, errors.Wrap(err, "listen")
	}
	if err := conn.SetReadBuffer(config.rxBufferSz); err != nil {
		return nil, errors.Wrap(err, "rx buffer")
	}
	if err := conn.SetWriteBuffer(config.txBufferSz); err != nil {
		return nil, errors.Wrap(err, "tx buffer")
	}
	l := &listener{
		lock:        new(sync.Mutex),
		peers:       btree.NewWith(config.treeLen, addrComparator),
		acceptQueue: make(chan net.Conn, config.acceptQLen),
		conn:        conn,
		addr:        addr,
		config:      config,
	}
	if config.i != nil {
		l.ii = config.i.newInstance(addr)
	}
	l.pool = newPool("listener", l.ii)
	go l.run()
	return l, nil
}

func (self *listener) Accept() (net.Conn, error) {
	conn, ok := <-self.acceptQueue
	if !ok {
		return nil, errors.New("listener closed")
	}
	return conn, nil
}

func (self *listener) Close() error {
	return nil
}

func (self *listener) Addr() net.Addr {
	return self.addr
}

func (self *listener) run() {
	logrus.Info("started")
	defer logrus.Warn("exited")

	for {
		if wm, peer, err := readWireMessage(self.conn, self.pool); err == nil {
			conn, found := self.peers.Get(peer)
			if found {
				lc := conn.(*listenerConn)
				if lc.ii != nil {
					lc.ii.wireMessageRx(peer, wm)
				}
				lc.queue(wm)

			} else {
				if wm.mt == HELLO {
					if self.ii != nil {
						self.ii.wireMessageRx(peer, wm)
					}
					go self.hello(wm, peer)

				} else {
					if self.ii != nil {
						self.ii.unknownPeer(peer)
						self.ii.wireMessageRx(peer, wm)
					}
					wm.buffer.unref()
				}
			}
		} else {
			if self.ii != nil {
				self.ii.wireMessageRx(peer, wm)
				self.ii.readError(peer, err)
			}
		}
	}
}

func (self *listener) hello(hello *wireMessage, peer *net.UDPAddr) {
	conn, err := newListenerConn(self, self.conn, peer, self.config)
	if err != nil {
		if self.ii != nil {
			self.ii.connectError(peer, err)
		}
		return
	}

	self.lock.Lock()
	self.peers.Put(peer, conn)
	self.lock.Unlock()

	go conn.rxer()
	if err := conn.hello(hello); err != nil {
		if self.ii != nil {
			self.ii.connectError(peer, err)
		}
		return
	}

	self.acceptQueue <- conn

	if self.ii != nil {
		self.ii.connected(peer)
	}
}

func addrComparator(i, j interface{}) int {
	ai := i.(*net.UDPAddr)
	aj := j.(*net.UDPAddr)
	for l := 0; l < 4; l++ {
		if ai.IP[l] < aj.IP[l] {
			return -1
		}
		if ai.IP[l] > aj.IP[l] {
			return 1
		}
	}
	if ai.Port < aj.Port {
		return -1
	}
	if ai.Port > aj.Port {
		return 1
	}
	return 0
}
