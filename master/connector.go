package master

import (
	log "github.com/Sirupsen/logrus"
	"github.com/go-mangos/mangos"
	"github.com/go-mangos/mangos/protocol/pub"
	"github.com/go-mangos/mangos/protocol/sub"
	"github.com/go-mangos/mangos/transport/inproc"
	"github.com/go-mangos/mangos/transport/tcp"
	"github.com/loadimpact/speedboat/message"
)

// A bidirectional pub/sub connector, used to connect to a master.
type Connector struct {
	InSocket  mangos.Socket
	OutSocket mangos.Socket
}

// Creates a bare, unconnected connector.
func NewBareConnector() (conn Connector, err error) {
	if conn.OutSocket, err = pub.NewSocket(); err != nil {
		return conn, err
	}

	if conn.InSocket, err = sub.NewSocket(); err != nil {
		return conn, err
	}

	return conn, nil
}

func NewClientConnector(topic string, inAddr string, outAddr string) (conn Connector, err error) {
	if conn, err = NewBareConnector(); err != nil {
		return conn, err
	}
	if err = setupAndDial(conn.InSocket, inAddr); err != nil {
		return conn, err
	}
	if err = setupAndDial(conn.OutSocket, outAddr); err != nil {
		return conn, err
	}

	err = conn.InSocket.SetOption(mangos.OptionSubscribe, []byte(topic))
	if err != nil {
		return conn, err
	}

	return conn, nil
}

func NewServerConnector(outAddr string, inAddr string) (conn Connector, err error) {
	if conn, err = NewBareConnector(); err != nil {
		return conn, err
	}
	if err = setupAndListen(conn.OutSocket, outAddr); err != nil {
		return conn, err
	}
	if err = setupAndListen(conn.InSocket, inAddr); err != nil {
		return conn, err
	}

	err = conn.InSocket.SetOption(mangos.OptionSubscribe, []byte(""))
	if err != nil {
		return conn, err
	}

	return conn, nil
}

func setupSocket(sock mangos.Socket) {
	sock.AddTransport(inproc.NewTransport())
	sock.AddTransport(tcp.NewTransport())
}

func setupAndListen(sock mangos.Socket, addr string) error {
	setupSocket(sock)
	if err := sock.Listen(addr); err != nil {
		return err
	}
	return nil
}

func setupAndDial(sock mangos.Socket, addr string) error {
	setupSocket(sock)
	if err := sock.Dial(addr); err != nil {
		return err
	}
	return nil
}

// Provides a channel-based interface around the underlying socket API.
func (c *Connector) Run() (<-chan message.Message, chan message.Message, <-chan error) {
	errors := make(chan error)
	in := make(chan message.Message)
	out := make(chan message.Message)

	// Read incoming messages
	go func() {
		for {
			msg, err := c.Read()
			if err != nil {
				errors <- err
				continue
			}
			in <- msg
		}
	}()

	// Write outgoing messages
	go func() {
		for {
			msg := <-out
			err := c.Write(msg)
			if err != nil {
				errors <- err
				continue
			}
		}
	}()

	return in, out, errors
}

func (c *Connector) Read() (msg message.Message, err error) {
	data, err := c.InSocket.Recv()
	if err != nil {
		return msg, err
	}
	log.WithField("data", string(data)).Debug("Read data")
	err = message.Decode(data, &msg)
	if err != nil {
		return msg, err
	}
	log.WithFields(log.Fields{
		"type": msg.Type,
		"body": msg.Body,
	}).Debug("Decoded message")
	return msg, nil
}

func (c *Connector) Write(msg message.Message) (err error) {
	data, err := msg.Encode()
	if err != nil {
		return err
	}
	log.WithField("data", string(data)).Debug("Writing data")
	err = c.OutSocket.Send(data)
	if err != nil {
		return err
	}
	return nil
}