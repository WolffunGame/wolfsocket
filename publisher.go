package wolfsocket

import (
	"errors"
	"wolfsocket/stackexchange/protos"
)

var ErrUninitialized = errors.New("uninitialized")

var p func(channel string, msg protos.ServerMessage) error

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

func do(server *Server) func(connID string, msg protos.ServerMessage) error {
	return func(connID string, msg protos.ServerMessage) error {
		conn := server.GetConnections()[connID]
		if conn == nil {
			return errors.New("conn not found")
		}

		m := Message{
			Namespace: msg.Namespace,
			Event:     msg.EventName,
			Body:      msg.Body,
			SetBinary: true,
		}
		ok := conn.Write(m)
		if !ok {
			return errors.New("write err " + msg.EventName)
		}
		return nil
	}
}

// Publish don't forget to assign the namespace for the mgss if needed
func Publish(channel string, msg protos.ServerMessage) error {
	if p != nil {
		return p(channel, msg)
	}

	return ErrUninitialized
}
