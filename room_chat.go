package wolfsocket

import (
	"github.com/WolffunGame/wolfsocket/stackexchange/redis/protos"
)

type RoomChannel string //country - party - lobby,...
var PartyChatRoom RoomChannel = "party"

type RoomChat interface {
	Conn() *NSConn
	RoomChatID() string

	Subscribe()
	Unsubscribe()

	Chat(protos.ServerMessage)
	Channel() RoomChannel
}

var x RoomChat = &BaseRoomChat{}

const prefixRoomChat = "chat."

type BaseRoomChat struct {
	ID      string
	channel RoomChannel

	conn *NSConn
}

func NewRoomChat(roomID string, channel RoomChannel, conn *NSConn) *BaseRoomChat {
	if roomID == "" {
		roomID = genID()
	}
	return &BaseRoomChat{
		ID:      roomID,
		conn:    conn,
		channel: channel,
	}
}

func (rc *BaseRoomChat) Conn() *NSConn {
	return rc.conn
}

func (rc *BaseRoomChat) Chat(msg protos.ServerMessage) {
	rc.conn.SBroadcast(rc.getChannel(), msg)
}

func (rc *BaseRoomChat) Subscribe() {
	rc.conn.Subscribe(rc.getChannel())
}
func (rc *BaseRoomChat) Unsubscribe() {
	rc.conn.Unsubscribe(rc.getChannel())
}

func (rc *BaseRoomChat) String() string {
	return rc.ID
}

func (rc *BaseRoomChat) RoomChatID() string {
	return rc.ID
}

// return channel pubsub chat room
func (rc *BaseRoomChat) getChannel() string {
	return prefixRoomChat + rc.ID
}

// return channel type of this room
func (rc *BaseRoomChat) Channel() RoomChannel {
	return rc.channel
}
