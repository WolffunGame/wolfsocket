package publisher

import (
	"errors"
	"github.com/WolffunService/wolfsocket"
	"github.com/WolffunService/wolfsocket/stackexchange/protos"
)

var ErrUninitialized = errors.New("uninitialized")

var p func(channel string, msgs []protos.ServerMessage) error

func Init(server *wolfsocket.Server) {
	if server == nil {
		return
	}

	p = do(server)
}

func SetStackExchange(server *wolfsocket.Server) {
	if server == nil || server.StackExchange == nil {
		return
	}

	p = server.StackExchange.Publish
}

func do(server *wolfsocket.Server) func(connID string, msgs []protos.ServerMessage) error {
	return func(connID string, msgs []protos.ServerMessage) error {
		conn := server.GetConnections()[connID]
		if conn == nil {
			return errors.New("conn not found")
		}

		for _, msg := range msgs {
			m := wolfsocket.Message{
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

func Publish(channel string, msgs ...protos.ServerMessage) error {
	if p != nil {
		return p(channel, msgs)
	}

	return ErrUninitialized
}
