package wolfsocket

import (
	"github.com/WolffunGame/wolfsocket/stackexchange/redis/protos"
	uuid "github.com/iris-contrib/go.uuid"
)

const prefixParty = "party."

type Party struct {
	ID string

	//partyInfo
}

func NewParty(partyID string) *Party {
	if partyID == "" {
		partyID = genID()
	}
	return &Party{
		ID: partyID,
	}
}

func genID() string {
	return uuid.Must(uuid.NewV4()).String()
}

func (p *Party) Broadcast(conn *NSConn, msg ...protos.ServerMessage) {
	conn.SBroadcast(p.getChannel(), msg...)
}

func (p *Party) Subscribe(conn *NSConn) {
	conn.Subscribe(p.getChannel())
}
func (p *Party) Unsubscribe(conn *NSConn) {
	conn.Unsubscribe(p.getChannel())
}

func (p *Party) String() string {
	return p.ID
}

func (p *Party) getChannel() string {
	return prefixParty + p.ID
}

func (p *Party) Save() error {
	return nil
}

func (p *Party) Update() error {
	return nil
}
