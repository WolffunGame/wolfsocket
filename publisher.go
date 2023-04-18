package wolfsocket

import (
	"errors"
	"github.com/WolffunService/wolfsocket/stackexchange/protos"
)

var ErrUninitialized = errors.New("uninitialized")

var p func(channel string, msgs []protos.ServerMessage) error

func initPublisher(server *Server) {
	if server == nil {
		return
	}

	p = do(server)
}

func setStackExchangePublisher(exc StackExchange) {
	if exc == nil {
		return
	}

	p = exc.Publish
}

func do(server *Server) func(connID string, msgs []protos.ServerMessage) error {
	return func(connID string, msgs []protos.ServerMessage) error {
		conn := server.GetConnections()[connID]
		if conn == nil {
			return errors.New("conn not found")
		}

		for _, msg := range msgs {
			m := Message{
				Namespace: msg.Namespace,
				Event:     msg.EventName,
				Body:      msg.Body,
				SetBinary: true,
			}
			conn.Write(m)
		}
		return nil
	}
}

// Publish don't forget to assign the namespace for the mgss if needed
func Publish(channel string, msgs ...protos.ServerMessage) error {
	if p != nil {
		return p(channel, msgs)
	}

	return ErrUninitialized
}
