package wolfsocket

import (
	"github.com/WolffunService/wolfsocket/options"
	"github.com/WolffunService/wolfsocket/stackexchange/protos"
	"sync"
)

const notifyKey = "notify."

type Friends struct {
	nsConn *NSConn

	listID map[string]struct{}
	mutex  sync.RWMutex
}

func (nsConn *NSConn) SubscribeNotify(friendIDs ...string) {
	nsConn.Subscribe(getKeyNotify(nsConn.ID())) //tu sub ban than

	nsConn.AddFriends(friendIDs...)
}

func (nsConn *NSConn) Notify(msg protos.ServerMessage, opts ...options.BroadcastOption) error {
	if nsConn.friends == nil {
		return nil
	}

	return nsConn.friends.notify(msg, opts...)
}

func (nsConn *NSConn) AddFriends(friendIDs ...string) {
	numFriends := len(friendIDs)
	if numFriends > 0 {
		nsConn.friends = &Friends{
			listID: make(map[string]struct{}, numFriends),
			nsConn: nsConn,
		}
		nsConn.friends.add(friendIDs...)

	}
}

func (nsConn *NSConn) RemoveFriends(friendIDs ...string) {
	if nsConn.friends == nil {
		return
	}
	nsConn.friends.remove(friendIDs...)
}

// check is exist this friend
func (nsConn *NSConn) CheckIsFriend(friendsID string) bool {
	if nsConn.friends == nil {
		return false
	}
	return nsConn.friends.exist(friendsID)
}

func (f *Friends) notify(msg protos.ServerMessage, opts ...options.BroadcastOption) error {
	f.mutex.RLock()
	defer f.mutex.RUnlock()
	for friendID, _ := range f.listID {
		if err := f.nsConn.SBroadcast(getKeyNotify(friendID), msg, opts...); err != nil {
			return err
		}
	}
	return nil
}

func (f *Friends) add(friendIDs ...string) {
	f.mutex.Lock()
	defer f.mutex.Unlock()
	for _, friendID := range friendIDs {
		f.listID[friendID] = struct{}{}
	}
}

func (f *Friends) remove(friendIDs ...string) {
	f.mutex.Lock()
	defer f.mutex.Unlock()
	for _, friendID := range friendIDs {
		delete(f.listID, friendID)
	}
}

func (f *Friends) exist(friendID string) bool {
	f.mutex.Lock()
	defer f.mutex.Unlock()
	_, exist := f.listID[friendID]
	return exist
}

func getKeyNotify(userID string) string {
	return notifyKey + userID
}
