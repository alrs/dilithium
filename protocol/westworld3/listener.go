package westworld3

import (
	"fmt"
	"github.com/emirpasic/gods/trees/btree"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"net"
	"sync"
)

type listener struct {
	lock        *sync.Mutex
	profile     *Profile
	profileId   byte
	peers       *btree.Tree
	acceptQueue chan net.Conn
	conn        *net.UDPConn
	addr        *net.UDPAddr
	pool        *pool
	ii          InstrumentInstance
}

func Listen(addr *net.UDPAddr, profileId byte) (net.Listener, error) {
	var profile *Profile
	if p, found := profileRegistry[profileId]; found {
		profile = p
	} else {
		return nil, errors.Errorf("profile [%d] not found in registry", int(profileId))
	}
	conn, err := net.ListenUDP("udp", addr)
	if err != nil {
		return nil, errors.Wrap(err, "listen")
	}
	if err := conn.SetReadBuffer(profile.RxBufferSz); err != nil {
		return nil, errors.Wrap(err, "set rx buffer size")
	}
	if err := conn.SetWriteBuffer(profile.TxBufferSz); err != nil {
		return nil, errors.Wrap(err, "set tx buffer size")
	}
	l := &listener{
		lock:        new(sync.Mutex),
		profile:     profile,
		profileId:   profileId,
		peers:       btree.NewWith(profile.ListenerPeersTreeLen, addrComparator),
		acceptQueue: make(chan net.Conn, profile.AcceptQueueLen),
		conn:        conn,
		addr:        addr,
	}
	listenerId := fmt.Sprintf("listener_%s", addr)
	l.ii = profile.i.NewInstance(listenerId, addr)
	l.pool = newPool(listenerId, uint32(dataStart+profile.MaxSegmentSz), l.ii)
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
	return errors.Errorf("close not implemented")
}

func (self *listener) Addr() net.Addr {
	return self.addr
}

func (self *listener) run() {
	logrus.Infof("started")
	defer logrus.Warn("exited")
	defer func() { self.ii.Shutdown() }()

	for {
		if wm, peer, err := readWireMessage(self.conn, self.pool); err == nil {
			conn, found := self.peers.Get(peer)
			if found {
				lc := conn.(*listenerConn)
				lc.queue(wm)

			} else {
				self.ii.WireMessageRx(peer, wm)
				if wm.messageType() == HELLO {
					go self.hello(wm, peer)

				} else {
					self.ii.UnknownPeer(peer)
					wm.buffer.unref()
				}
			}
		} else {
			self.ii.ReadError(peer, err)
		}
	}
}

func (self *listener) hello(hello *wireMessage, peer *net.UDPAddr) {
	hook := func() {
		self.lock.Lock()
		self.peers.Remove(peer)
		logrus.Infof("remaining peers: %d", self.peers.Size())
		self.lock.Unlock()
		logrus.Infof("removed peer [%s]", peer)
	}
	conn, err := newListenerConn(self, self.conn, peer, self.profile, hook)
	if err != nil {
		self.ii.ConnectionError(peer, err)
		return
	}

	self.lock.Lock()
	self.peers.Put(peer, conn)
	self.lock.Unlock()

	if err := conn.hello(hello); err != nil {
		logrus.Errorf("error connecting (%v)", err)
		self.ii.ConnectionError(peer, err)
		return
	}

	self.acceptQueue <- conn

	self.ii.Connected(peer)
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
