package wolfsocket

import (
	"github.com/WolffunGame/wolfsocket/stackexchange/redis/protos"
	"sync"
)

const prefixParty = "party."

type Party struct {
	ID string

	membersMutex sync.RWMutex
	members      map[string]struct{}
}

func newParty(partyID string) *Party {
	return &Party{
		ID: partyID,
	}
}

func (p *Party) AskJoinParty(conn *NSConn) string {
	panic("")
}

func (p *Party) OnJoinParty(conn *NSConn) error {
	if conn.server().usesStackExchange() {
		conn.server().StackExchange.Subscribe(conn.Conn, p.getChannel())
	}
	//conn.events.fireEvent(conn, Message{})
	//fire event
	p.members[conn.Conn.ID()] = struct{}{}
	//save cache
	return nil
}

func (p *Party) OnAnotherJoinParty(userID string) error {
	p.members[userID] = struct{}{}
	return nil
}

func (p *Party) Broadcast(conn *NSConn, msg protos.ServerMessage) {
	conn.SBroadcast()
}

func (p *Party) String() string {
	return p.ID
}
func (p *Party) getChannel() string {
	return prefixParty + p.ID
}
