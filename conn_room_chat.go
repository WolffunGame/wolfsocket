package wolfsocket

import "errors"

func (ns *NSConn) RoomChat(roomChannel RoomChannel) RoomChat {
	if ns == nil {
		return nil
	}

	ns.roomsChatMutex.RLock()
	room := ns.roomsChat[roomChannel]
	ns.roomsChatMutex.RUnlock()

	return room
}

func (ns *NSConn) JoinRoomChat(room RoomChat) error {
	if ns == nil {
		return nil
	}

	ns.roomsChatMutex.Lock()
	defer ns.roomsChatMutex.Unlock()
	if _, exists := ns.roomsChat[room.Channel()]; exists {
		return errors.New("You are already in this room chat channel ")
	}
	ns.roomsChat[room.Channel()] = room
	room.Subscribe()
	return nil
}

func (ns *NSConn) LeaveRoomChat(roomChannel RoomChannel) error {
	if ns == nil {
		return nil
	}

	ns.roomsChatMutex.Lock()
	defer ns.roomsChatMutex.Unlock()
	room, exists := ns.roomsChat[roomChannel]
	if !exists {
		return errors.New("You aren't in this room chat channel ")
	}
	room.Unsubscribe()
	delete(ns.roomsChat, roomChannel)

	return nil
}

func (ns *NSConn) leaveAllRoomChat() {
	if ns == nil {
		return
	}

	ns.roomsChatMutex.Lock()
	defer ns.roomsChatMutex.Unlock()
	for roomChannel, room := range ns.roomsChat {
		room.Unsubscribe()
		delete(ns.roomsChat, roomChannel)
	}
	return
}
