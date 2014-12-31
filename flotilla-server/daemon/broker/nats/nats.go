package nats

import (
	"fmt"
	"time"

	"github.com/apcera/nats"
)

const (
	subject = "test"

	// Maximum bytes we will get behind before we start slowing down publishing.
	maxBytesBehind = 1024 * 1024 // 1MB

	// Maximum msgs we will get behind before we start slowing down publishing.
	maxMsgsBehind = 65536 // 64k

	// Time to delay publishing when we are behind.
	delay = 1 * time.Millisecond
)

type NATSPeer struct {
	conn     *nats.Conn
	messages chan []byte
	send     chan []byte
	errors   chan error
	done     chan bool
}

func NewNATSPeer(host string) (*NATSPeer, error) {
	conn, err := nats.Connect(fmt.Sprintf("nats://%s", host))
	if err != nil {
		return nil, err
	}

	// We want to be alerted if we get disconnected, this will be due to Slow
	// Consumer.
	conn.Opts.AllowReconnect = false

	return &NATSPeer{
		conn:     conn,
		messages: make(chan []byte, 10000),
		send:     make(chan []byte),
		errors:   make(chan error, 1),
		done:     make(chan bool),
	}, nil
}

func (n *NATSPeer) Subscribe() error {
	n.conn.Subscribe(subject, func(message *nats.Msg) {
		n.messages <- message.Data
	})
	return nil
}

func (n *NATSPeer) Recv() ([]byte, error) {
	return <-n.messages, nil
}

func (n *NATSPeer) Send() chan<- []byte {
	return n.send
}

func (n *NATSPeer) Errors() <-chan error {
	return n.errors
}

func (n *NATSPeer) Done() {
	n.done <- true
}

func (n *NATSPeer) Setup() {
	go func() {
		for {
			select {
			case msg := <-n.send:
				if err := n.sendMessage(msg); err != nil {
					n.errors <- err
				}
			case <-n.done:
				return
			}
		}
	}()
}

func (n *NATSPeer) sendMessage(message []byte) error {
	// Check if we are behind by >= 1MB bytes.
	bytesDeltaOver := n.conn.OutBytes-n.conn.InBytes >= maxBytesBehind

	// Check if we are behind by >= 65k msgs.
	msgsDeltaOver := n.conn.OutMsgs-n.conn.InMsgs >= maxMsgsBehind

	// If we are behind on either condition, sleep a bit to catch up receiver.
	if bytesDeltaOver || msgsDeltaOver {
		time.Sleep(delay)
	}

	return n.conn.Publish(subject, message)
}

func (n *NATSPeer) Teardown() {
	n.conn.Close()
}
