package loop

import (
	"github.com/michaelquigley/dilithium/protocol/loop"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"net"
)

func init() {
	loopCmd.AddCommand(loopServerCmd)
}

var loopServerCmd = &cobra.Command{
	Use:   "server <listenAddress>",
	Short: "Start loop server",
	Args:  cobra.ExactArgs(1),
	Run:   loopServer,
}

func loopServer(_ *cobra.Command, args []string) {
	pool := loop.NewPool(2+64+size)

	var ds *loop.DataSet
	if startSender {
		var err error
		ds, err = loop.NewDataSet(pool)
		if err != nil {
			logrus.Fatalf("error creating dataset (%v)", err)
		}
	}

	listenAddress, err := net.ResolveTCPAddr("tcp", args[0])
	if err != nil {
		logrus.Fatalf("error parsing listen address (%v)", err)
	}

	listener, err := net.ListenTCP("tcp", listenAddress)
	if err != nil {
		logrus.Fatalf("error listening (%v)", err)
	}

	conn, err := listener.Accept()
	if err != nil {
		logrus.Fatalf("error accepting (%v)", err)
	}

	var rx *loop.Receiver
	if startReceiver {
		rx = loop.NewReceiver(pool, conn)
		go rx.Run()
	}

	var tx *loop.Sender
	if startSender {
		tx = loop.NewSender(ds, pool, conn, count)
		go tx.Run()
	}

	if rx != nil {
		<- rx.Done
	}
	if tx != nil {
		<- tx.Done
	}

	logrus.Infof("%d allocations", pool.Allocations)
}