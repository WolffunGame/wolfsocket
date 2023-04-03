package wolfsocket

import (
	"github.com/WolffunGame/wolfsocket/stackexchange/protos"
)

const notifyKey = "notify."

func (nsConn *NSConn) SubscribeNotify(friendIDs ...string) {
	for _, friendID := range friendIDs {
		nsConn.Subscribe(getKeyNotify(friendID))
	}
}
func (nsConn *NSConn) Notify(msgs ...protos.ServerMessage) error {
	return nsConn.SBroadcast(getKeyNotify(nsConn.ID()), msgs...)
}

func getKeyNotify(userID string) string {
	return notifyKey + userID
}
