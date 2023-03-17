package wolfsocket

import (
	"context"
	"github.com/WolffunGame/wolfsocket/stackexchange/redis/protos"
	uuid "github.com/iris-contrib/go.uuid"
)

type Party interface {
	PartyID() string
	Subscribe(conn *NSConn)
	Unsubscribe(conn *NSConn)

	Broadcast(conn *NSConn, msg ...protos.ServerMessage)

	Create(ctx context.Context, nsConn *NSConn) error
	Join(ctx context.Context, nsConn *NSConn) error
	Leave(ctx context.Context) error
}

const prefixParty = "party."

type BaseParty struct {
	ID string

	conn *NSConn
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

func (p *BaseParty) Broadcast(msg ...protos.ServerMessage) {
	p.conn.SBroadcast(p.getChannel(), msg...)
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

func (p *BaseParty) Create(nsConn *NSConn) {
	p.conn = nsConn
	p.Subscribe(nsConn)
}

// playerInfo send back to remote-side
func (p *BaseParty) Join(nsConn *NSConn, playerInfo []byte) {
	p.conn = nsConn
	//send message to all playerservice in this party
	p.Broadcast(protos.ServerMessage{
		Namespace: nsConn.Namespace(),
		EventName: OnPartySomebodyJoined,
		Body:      playerInfo,
		ToClient:  true,
	})

	p.Subscribe(nsConn)
}

func (p *BaseParty) Leave() {

	p.Unsubscribe(p.conn)
	//send message to all playerservice in this party
	p.Broadcast(protos.ServerMessage{
		Namespace: p.conn.Namespace(),
		EventName: OnPartySomebodyLeft,
		Body:      []byte(p.conn.ID()),
		ToClient:  true,
	})

	p.conn = nil

}

func (p *BaseParty) getChannel() string {
	return prefixParty + p.ID
}
