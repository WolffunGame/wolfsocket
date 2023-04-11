package wolfsocket

import (
	"github.com/WolffunGame/wolfsocket/options"
	"github.com/WolffunGame/wolfsocket/stackexchange/protos"
	uuid "github.com/iris-contrib/go.uuid"
)

type Party interface {
	NSConn() *NSConn
	PartyID() string
	SetPartyID(partyID string)

	Broadcast(eventName string, body []byte, opts ...options.BroadcastOption)

	Create(nsConn *NSConn) error
	Join(nsConn *NSConn, playerInfo []byte) error
	Leave() error

	Subscribe()
	Unsubscribe()

	PartyInfo() []byte
}

var _ Party = &BaseParty{}

const prefixParty = "party."

type BaseParty struct {
	ID string

	nsConn *NSConn
}

func NewParty(partyID string) *BaseParty {
	if partyID == "" {
		partyID = genID()
	}
	bp := &BaseParty{}
	bp.SetPartyID(partyID)

	return bp
}

func genID() string {
	return uuid.Must(uuid.NewV4()).String()
}

func (p *BaseParty) SetPartyID(partyID string) {
	p.ID = partyID
}

func (p *BaseParty) NSConn() *NSConn {
	return p.nsConn
}

func (p *BaseParty) Broadcast(eventName string, body []byte, opts ...options.BroadcastOption) {
	msg := protos.ServerMessage{
		EventName: eventName,
		Body:      body,
	}
	p.nsConn.SBroadcast(p.getChannel(), msg, opts...)
}

func (p *BaseParty) Subscribe() {
	p.nsConn.Subscribe(p.getChannel())
}
func (p *BaseParty) Unsubscribe() {
	p.nsConn.Unsubscribe(p.getChannel())
}

func (p *BaseParty) String() string {
	return p.ID
}

func (p *BaseParty) PartyID() string {
	return p.ID
}

func (p *BaseParty) Create(nsConn *NSConn) error {
	p.nsConn = nsConn
	p.Subscribe()
	return nil
}

// playerInfo send back to remote-side
func (p *BaseParty) Join(nsConn *NSConn, playerInfo []byte) error {
	p.nsConn = nsConn
	//send message to all playerservice in this party
	p.Broadcast(OnPartySomebodyJoined, playerInfo,
		options.ToClient(),
		options.Except(p.nsConn.Conn.GetServerConnID()),
	)

	p.Subscribe()

	return nil
}

func (p *BaseParty) Leave() error {

	p.Unsubscribe()
	//send message to all playerservice in this party
	p.Broadcast(OnPartySomebodyLeft, []byte(p.nsConn.ID()),
		options.ToClient(),
		options.Except(p.nsConn.Conn.GetServerConnID()),
	)

	p.nsConn = nil

	return nil
}

func (p *BaseParty) PartyInfo() []byte {
	//TODO implement me
	panic("implement me")
}

func (p *BaseParty) getChannel() string {
	return prefixParty + p.ID
}
