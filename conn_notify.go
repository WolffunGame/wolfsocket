package wolfsocket

import (
	"github.com/WolffunGame/wolfsocket/options"
	"github.com/WolffunGame/wolfsocket/stackexchange/protos"
)

const notifyKey = "notify."

func (nsConn *NSConn) SubscribeNotify(friendIDs ...string) {
	for _, friendID := range friendIDs {
		nsConn.Subscribe(getKeyNotify(friendID))
	}
}
func (nsConn *NSConn) Notify(msg protos.ServerMessage, opts ...options.BroadcastOption) error {
	return nsConn.SBroadcast(getKeyNotify(nsConn.ID()), msg, opts...)
}

func getKeyNotify(userID string) string {
	return notifyKey + userID
}
