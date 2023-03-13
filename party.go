package wolfsocket

import "sync"

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

func (p *Party) OnJoinParty(conn *NSConn) string {
	panic("")
}

func (p *Party) String() string {
	return p.ID
}
