package wolfsocket

import (
	"github.com/WolffunGame/wolfsocket/stackexchange/redis/protos"
	uuid "github.com/iris-contrib/go.uuid"
)

type Party interface {
	PartyID() string
	Subscribe(conn *NSConn)
	Unsubscribe(conn *NSConn)
}

const prefixParty = "party."

type BaseParty struct {
	ID string
}

func NewParty(partyID string) *BaseParty {
	if partyID == "" {
		partyID = genID()
	}
	return &BaseParty{
		ID: partyID,
	}
}

func genID() string {
	return uuid.Must(uuid.NewV4()).String()
}

func (p *BaseParty) Broadcast(conn *NSConn, msg ...protos.ServerMessage) {
	conn.SBroadcast(p.getChannel(), msg...)
}

func (p *BaseParty) Subscribe(conn *NSConn) {
	conn.Subscribe(p.getChannel())
}
func (p *BaseParty) Unsubscribe(conn *NSConn) {
	conn.Unsubscribe(p.getChannel())
}

func (p *BaseParty) String() string {
	return p.ID
}

func (p *BaseParty) PartyID() string {
	return p.ID
}

func (p *BaseParty) getChannel() string {
	return prefixParty + p.ID
}

func (p *BaseParty) Save() error {
	return nil
}

func (p *BaseParty) Update() error {
	return nil
}
