package wolfsocket

import (
	"errors"
	"wolfsocket/wserror"
)

var (
	forcedJoin      = false
	AutoCreateParty = false
)

// Forced to join a new group when already in another group
func SetForcedJoin(isForced bool) {
	forcedJoin = isForced
}

func (ns *NSConn) IsInParty() bool {
	return ns.Party != nil
}

func (ns *NSConn) forceLeaveAll() {
	ns.AskPartyLeave(Message{
		Namespace: ns.namespace,
		Event:     OnPartyLeave,
	})

	ns.leaveAllRoomChat()
}

func (ns *NSConn) AskPartyCreate(msg Message) error {
	if ns.Party != nil {
		//already in party
		msg.Err = errors.New("you already in party")
		ns.Conn.Write(msg)
		return msg.Err
	}

	//fireEventCreateAndJoin
	err := ns.events.fireEvent(ns, msg)
	if err != nil {
		b, ok := isReply(err)
		if !ok {
			msg.Err = err
			ns.Conn.Write(msg)
			return msg.Err
		}

		resp := msg
		resp.Body = b
		ns.Conn.Write(resp)
	}

	//when you haven't handled the OnCreateParty
	if ns.Party == nil {
		msg.Err = errors.New("the party is not available, please assign the Party in the OnCreateParty event")
		ns.Conn.Write(msg)
		return msg.Err
	}

	ns.Party.Create(ns)
	ns.ReplyJoined()
	return nil
}

func (ns *NSConn) AskPartyInvite(msg Message) {
	if ns.Party == nil && AutoCreateParty {
		msgCreate := msg
		msgCreate.Event = OnPartyCreate
		err := ns.AskPartyCreate(msgCreate)
		if err != nil {
			msg.Err = errors.New("cannot invite at this time")
			ns.Conn.Write(msg)
			return
		}
	}

	//fire event invite
	err := ns.events.fireEvent(ns, msg)
	if err != nil {
		msg.Err = err
		ns.Conn.Write(msg)
		return
	}

	//ai handle nay gui data
	//receiverID := string(msg.Body)
	//if len(receiverID) > 0 {
	//	//body must is connID receive invite message
	//	ns.SBroadcast(receiverID, protos.ServerMessage{
	//		Namespace: ns.Namespace(),
	//		EventName: OnPartyReceiveMessageInvite,
	//		Body:      []byte(ns.Party.PartyID()),
	//		ToClient:  true,
	//	})
	//}
	ns.Conn.writeEmptyReply(msg.wait)
}

func (ns *NSConn) replyPartyReplyInvitation(msg Message) {
	//fire event accept invite
	err := ns.events.fireEvent(ns, msg)
	if err != nil {
		msg.Err = err
		ns.Conn.Write(msg)
		return
	}

	//if accept success - force leave current party
	if ns.Party != nil {
		//force leave current party
		err := ns.AskPartyLeave(Message{
			Namespace: ns.namespace,
			Event:     OnPartyLeave,
		})
		if err != nil {
			msg.Err = err
			ns.Conn.Write(msg)
			return
		}
	}

	//and ask join this party
	msgJoin, err := ns.JoinMsg(msg)
	if err != nil {
		msg.Err = err
		ns.Conn.Write(msg)
		return
	}

	err = ns.AskPartyJoin(msgJoin)
	if err != nil {
		msg.Err = err
		ns.Conn.Write(msg)
		return
	}
}

func (ns *NSConn) AskPartyJoin(msg Message) error {
	if ns == nil {
		msg.Err = errInvalidMethod
		ns.Conn.Write(msg)
		return msg.Err
	}

	if ns.Party != nil {
		if !forcedJoin {
			msg.Err = wserror.AlreadyInParty.WSErr("You are already in party", ns.Party.PartyID())
			ns.Conn.Write(msg)
			return msg.Err
		}
		//leave current joined party
		err := ns.AskPartyLeave(Message{
			Namespace: ns.namespace,
			Event:     OnPartyLeave,
		})
		if err != nil {
			msg.Err = err
			ns.Conn.Write(msg)
			return msg.Err
		}
	}

	//OnPartyJoin event( check can join party ,...)
	err := ns.events.fireEvent(ns, msg)
	if err != nil {
		msg.Err = err
		ns.Conn.Write(msg)
		return msg.Err
	}

	//when you haven't handled the OnJoinParty
	if ns.Party == nil {
		msg.Err = errors.New("The party is not available, please assign the Party in the OnJoinParty event")
		ns.Conn.Write(msg)
		return msg.Err
	}

	if ns.Party.NSConn() == nil {
		ns.Party.Join(ns, nil)
	}

	ns.ReplyJoined()
	return nil
}

func (ns *NSConn) ForceLeaveParty() error {
	return ns.AskPartyLeave(Message{
		Namespace: ns.namespace,
		Event:     OnPartyLeave,
	})
}

// remote request
func (ns *NSConn) AskPartyLeave(msg Message) error {
	if ns == nil {
		msg.Err = errInvalidMethod
		ns.Conn.Write(msg)
		return msg.Err
	}

	party := ns.Party
	if party == nil {
		msg.Err = errors.New("You are not in party ")
		ns.Conn.Write(msg)
		return msg.Err
	}

	// server-side, check for error on the local event first.
	err := ns.events.fireEvent(ns, msg)
	if err != nil {
		msg.Err = err
		ns.Conn.Write(msg)
		return msg.Err
	}

	//unsubscribe and broadcast left message all player this room
	party.Leave()
	//reset party
	ns.Party = nil
	//reply leave
	ns.Conn.Write(msg)

	ns.ReplyLeft()

	return nil
}

func (ns *NSConn) ReplyLeft() {
	msg := Message{
		Namespace: ns.namespace,
		Event:     OnPartyLeft,
		SetBinary: true,
	}
	ns.events.fireEvent(ns, msg)
	ns.LeaveRoomChat(PartyChatRoom) //leave room chat
	ns.Conn.Write(msg)              //send back remote side msg OnPartyLeft
}

func (ns *NSConn) ReplyJoined() {
	partyInfo := ns.Party.PartyInfo()
	if len(partyInfo) == 0 {
		partyInfo = []byte(ns.Party.PartyID())
	}

	msg := Message{
		Namespace: ns.namespace,
		Event:     OnPartyJoined,
		Body:      partyInfo,
		SetBinary: true,
	}
	ns.events.fireEvent(ns, msg)
	//join room chat
	ns.JoinRoomChat(NewRoomChat(ns.Party.PartyID(), PartyChatRoom, ns))

	ns.Conn.Write(msg)
}
