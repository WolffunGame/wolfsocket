package options

import (
	"github.com/WolffunGame/wolfsocket/stackexchange/protos"
)

type BroadcastOption = Option[protos.ServerMessage]

func ToClient() BroadcastOption {
	return func(msg *protos.ServerMessage) error {
		msg.ToClient = true
		return nil
	}
}

func Except(except string) BroadcastOption {
	return func(msg *protos.ServerMessage) error {
		msg.ExceptSender = except
		return nil
	}
}
