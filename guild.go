package wolfsocket

import (
	"github.com/WolffunService/wolfsocket/options"
	"github.com/WolffunService/wolfsocket/stackexchange/protos"
)

type Guild interface {
	NSConn() *NSConn
	GetID() string

	Broadcast(eventName string, body []byte, opts ...options.BroadcastOption)

	Subscribe()
	Unsubscribe()

	Connect()
	Disconnect()
}

var _ Guild = &BaseGuild{}

const prefixGuild = "guild."

type BaseGuild struct {
	ID string

	nsConn *NSConn
}

func NewGuild(guildID string) *BaseGuild {
	return &BaseGuild{
		ID: guildID,
	}
}

func (p *BaseGuild) NSConn() *NSConn {
	return p.nsConn
}

func (p *BaseGuild) GetID() string {
	return p.ID
}

func (p *BaseGuild) Broadcast(eventName string, body []byte, opts ...options.BroadcastOption) {
	msg := protos.ServerMessage{
		EventName: eventName,
		Body:      body,
	}
	p.nsConn.SBroadcast(p.getChannel(), msg, opts...)
}

func (p *BaseGuild) Subscribe() {
	p.nsConn.Subscribe(p.getChannel())
}
func (p *BaseGuild) Unsubscribe() {
	p.nsConn.Unsubscribe(p.getChannel())
}

func (p *BaseGuild) Connect() {
	p.Subscribe()
}

func (p *BaseGuild) Disconnect() {
	p.Unsubscribe()
}

func (p *BaseGuild) getChannel() string {
	return prefixGuild + p.ID
}
