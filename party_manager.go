package wolfsocket

import (
	"errors"
	uuid "github.com/iris-contrib/go.uuid"
	"sync"
)

// Tổ chức và lưu trữ các party
type PartyManager struct {
	parties      map[string]*Party
	partiesMutex sync.RWMutex
}

func (p *PartyManager) FindParty(partyID string) (*Party, error) {
	p.partiesMutex.RLock()
	defer p.partiesMutex.RUnlock()
	if party, exists := p.parties[partyID]; exists {
		return party, nil
	}
	return nil, ErrPartyNotFound
}

func (p *PartyManager) CreateNewParty() (*Party, error) {
	p.partiesMutex.Lock()
	defer p.partiesMutex.Unlock()
	partyID := p.genID()
	party := newParty(partyID)
	//if party, exists := p.parties[partyID]; exists {
	//	return party, nil
	//}
	p.parties[partyID] = party
	return party, nil
}

func (p *PartyManager) genID() string {
	return uuid.Must(uuid.NewV4()).String()
}

var (
	ErrPartyNotFound = errors.New("Party not found ")
)
