package wolfsocket

import (
	"github.com/WolffunGame/wolfsocket/stackexchange/redis/protos"
	uuid "github.com/iris-contrib/go.uuid"
)

type Party interface {
	Conn() *NSConn
	PartyID() string
	Subscribe(conn *NSConn)
	Unsubscribe(conn *NSConn)

	Broadcast(msg ...protos.ServerMessage)

	Create(nsConn *NSConn) error
	Join(nsConn *NSConn, playerInfo []byte) error
	Leave() error

	PartyInfo() []byte
}

const prefixParty = "party."

type BaseParty struct {
	ID string

	conn *NSConn
}

func NewParty(partyID string) BaseParty {
	if partyID == "" {
		partyID = genID()
	}
	return BaseParty{
		ID: partyID,
	}
}

func genID() string {
	return uuid.Must(uuid.NewV4()).String()
}

func (p *BaseParty) Conn() *NSConn {
	return p.conn
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

func (p *BaseParty) Create(nsConn *NSConn) error {
	p.conn = nsConn
	p.Subscribe(nsConn)
	return nil
}

// playerInfo send back to remote-side
func (p *BaseParty) Join(nsConn *NSConn, playerInfo []byte) error {
	p.conn = nsConn
	//send message to all playerservice in this party
	p.Broadcast(protos.ServerMessage{
		Namespace: p.conn.Namespace(),
		EventName: OnPartySomebodyJoined,
		Body:      playerInfo,
		ToClient:  true,

		//skip sender
		From:         p.conn.Conn.serverConnID,
		ExceptSender: true,
	})

	p.Subscribe(p.conn)

	return nil
}

func (p *BaseParty) Leave() error {

	p.Unsubscribe(p.conn)
	//send message to all playerservice in this party
	p.Broadcast(protos.ServerMessage{
		Namespace: p.conn.Namespace(),
		EventName: OnPartySomebodyLeft,
		Body:      []byte(p.conn.ID()),
		ToClient:  true,

		//skip sender
		From:         p.conn.Conn.serverConnID,
		ExceptSender: true,
	})

	p.conn = nil

	return nil
}

func (p *BaseParty) getChannel() string {
	return prefixParty + p.ID
}
