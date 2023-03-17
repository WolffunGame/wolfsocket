package wolfsocket

import (
	"context"
	errors "errors"
	"fmt"
	"github.com/WolffunGame/wolfsocket/stackexchange/redis/protos"
	"reflect"
)

// NSConn describes a connection connected to a specific namespace,
// it emits with the `Message.Namespace` filled and it can join to multiple rooms.
// A single `Conn` can be connected to one or more namespaces,
// each connected namespace is described by this structure.
type NSConn struct {
	Conn *Conn
	// Static from server, client can select which to use or not.
	// Client and server can ask to connect.
	// Server can forcely disconnect.
	namespace string
	// Static from server, client can select which to use or not.
	events Events

	// Dynamically channels/rooms for each connected namespace.
	// Client can ask to join, server can forcely join a connection to a room.
	// Namespace(room(fire event)).
	//rooms      map[string]*Room
	//roomsMutex sync.RWMutex

	// value is just a temporarily value.
	// Storage across event callbacks for this namespace.
	value reflect.Value

	Party Party
}

func newNSConn(c *Conn, namespace string, events Events) *NSConn {
	return &NSConn{
		Conn:      c,
		namespace: namespace,
		events:    events,
		//rooms:     make(map[string]*Room),
	}
}

// String method simply returns the Conn's ID().
// Useful method to this connected to a namespace connection to be passed on `Server#Broadcast` method
// to exclude itself from the broadcasted message's receivers.
func (ns *NSConn) String() string {
	return ns.Conn.String()
}

func (ns *NSConn) ID() string {
	return ns.Conn.ID()
}

// Emit method sends a message to the remote side
// with its `Message.Namespace` filled to this specific namespace.
func (ns *NSConn) Emit(event string, body []byte) bool {
	if ns == nil {
		return false
	}

	return ns.Conn.Write(Message{Namespace: ns.namespace, Event: event, Body: body})
}

// EmitBinary acts like `Emit` but it sets the `Message.SetBinary` to true
// and sends the data as binary, the receiver's Message in javascript-side is Uint8Array.
func (ns *NSConn) EmitBinary(event string, body []byte) bool {
	if ns == nil {
		return false
	}

	return ns.Conn.Write(Message{Namespace: ns.namespace, Event: event, Body: body, SetBinary: true})
}

// Conn#Remote Ask method writes a message to the remote side and blocks until a response or an error received.
func (ns *NSConn) AskRemote(ctx context.Context, event string, body []byte) (Message, error) {
	if ns == nil {
		return Message{}, ErrWrite
	}

	return ns.Conn.Ask(ctx, Message{Namespace: ns.namespace, Event: event, Body: body})
}

func (nsConn *NSConn) SBroadcast(channel string, msgs ...protos.ServerMessage) error {
	return nsConn.server().SBroadcast(channel, msgs...)
}

func (nsConn *NSConn) AskServer(channel string, msg protos.ServerMessage) (*protos.ReplyMessage, error) {
	return nsConn.server().AskServer(channel, msg)
}

func (nsConn *NSConn) server() *Server {
	return nsConn.Conn.Server()
}

func (nsConn *NSConn) Subscribe(channel string) {
	nsConn.server().StackExchange.Subscribe(nsConn.Conn, channel)
}

func (nsConn *NSConn) Unsubscribe(channel string) {
	nsConn.server().StackExchange.Unsubscribe(nsConn.Conn, channel)
}

func (nsConn *NSConn) ForceDisconnect() {
	nsConn.Conn.Close()
}

// Disconnect method sends a disconnect signal to the remote side and fires the local `OnNamespaceDisconnect` event.
func (ns *NSConn) Disconnect(ctx context.Context) error {
	if ns == nil {
		return nil
	}

	return ns.Conn.askDisconnect(ctx, Message{
		Namespace: ns.namespace,
		Event:     OnNamespaceDisconnect,
	}, true)
}

// Namespace return current namespace name
func (ns *NSConn) Namespace() string {
	return ns.namespace
}

// Get Set store data ns
func (ns *NSConn) Set(key string, value interface{}) {
	ns.Conn.Set(key, value)
}

// Namespace return current namespace name
func (ns *NSConn) Get(key string, value interface{}) (err error) {
	defer func() {
		if e := recover(); e != nil {
			err = errors.New(fmt.Sprintf("Can get key %s : %v", key, e))
		}
	}()
	v := ns.Conn.Get(key)
	// create a pointer to the value parameter
	ptr := reflect.ValueOf(value)
	if ptr.Kind() != reflect.Ptr || ptr.IsNil() {
		return errors.New("value parameter must be a non-nil pointer")
	}
	// assign the value to the pointer, using reflection
	reflect.ValueOf(value).Elem().Set(reflect.ValueOf(v))
	return
}

func (ns *NSConn) Write(eventName string, body []byte) {
	ns.Conn.Write(Message{
		Namespace: ns.namespace,
		Event:     eventName,
		Body:      body,
	})
}

// PARTY
//
//	func (ns *NSConn) CreateParty(ctx context.Context) (*BaseParty, error) {
//		if ns == nil {
//			return nil, ErrWrite
//		}
//		if ns.BaseParty != nil {
//			return nil, errors.New("rời phòng cái đi rồi yêu cầu cái khác bạn eiii")
//		}
//
//		return
//	}
//
//	func (ns *NSConn) JoinParty(ctx context.Context, partyID string) (*BaseParty, error) {
//		if ns == nil {
//			return nil, ErrWrite
//		}
//
//		return ns.askPartyJoin(ctx, partyID)
//	}
//
//	func (ns *NSConn) askPartyJoin(ctx context.Context, partyID string) (*Party, error) {
//		if ns.Party != nil {
//			return nil, errors.New("rời phòng cái đi rồi yêu cầu cái khác bạn eiii")
//		}
//
//		joinMsg := protos.ServerMessage{
//			Namespace: ns.namespace,
//			EventName: OnPartyJoin,
//		}
//		party := NewParty(partyID)
//		_, err := ns.AskServer(party.GetChannel(), joinMsg)
//		if err != nil {
//			return nil, err
//		}
//
//		err = ns.events.fireEvent(ns, joinMsg)
//		if err != nil {
//			return nil, err
//		}
//
//		joinMsg.Event = OnRoomJoined
//		ns.events.fireEvent(ns, joinMsg)
//		return party, nil
//	}

func (ns *NSConn) askPartyCreate(msg Message) error {
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
	ns.replyJoined()
	return nil
}

func (ns *NSConn) askPartyInvite(msg Message) {
	if ns.Party == nil {
		msgCreate := msg
		msgCreate.Event = OnPartyJoin
		err := ns.askPartyCreate(msgCreate)
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

	receiverID := string(msg.Body)
	if len(receiverID) > 0 {
		//body must is connID receive invite message
		ns.SBroadcast(receiverID, protos.ServerMessage{
			Namespace: ns.Namespace(),
			EventName: OnPartyReceiveMessageInvite,
			Body:      []byte(ns.Party.PartyID()),
			ToClient:  true,
		})
	}
}

func (ns *NSConn) replyPartyAcceptInvite(msg Message) {
	if ns.Party != nil {
		//force leave current party
		err := ns.replyPartyLeave(Message{
			Namespace: ns.namespace,
			Event:     OnPartyLeave,
		})
		if err != nil {
			return
		}
	}

	//fire event accept invite
	err := ns.events.fireEvent(ns, msg)
	if err != nil {
		msg.Err = err
		ns.Conn.Write(msg)
		return
	}

	//join
	msgJoin := msg
	msgJoin.Event = OnPartyJoin
	err = ns.replyPartyJoin(msgJoin)
	if err != nil {
		msg.Err = err
		ns.Conn.Write(msg)
		return
	}
}

func (ns *NSConn) replyPartyJoin(msg Message) error {
	if ns == nil {
		msg.Err = errInvalidMethod
		ns.Conn.Write(msg)
		return msg.Err
	}

	if ns.Party != nil {
		errors.New(fmt.Sprintf("You are already in party : %s", ns.Party.PartyID()))
		ns.Conn.Write(msg)
		return msg.Err
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

	if ns.Party.Conn() == nil {
		ns.Party.Join(ns, nil)
	}

	ns.replyJoined()
	return nil
}

// remote request
func (ns *NSConn) replyPartyLeave(msg Message) error {
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

	//unsubscribe and broadcast left message all player this room
	party.Leave()
	//reset party
	ns.Party = nil

	msg.Event = OnPartyLeft
	ns.events.fireEvent(ns, msg)

	ns.Conn.Write(msg) //send back remote side msg OnPartyLeft
	return nil
}

func (ns *NSConn) replyJoined() {
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

	ns.Conn.Write(msg)
}

func (ns *NSConn) FireEvent(msg Message) error {
	return ns.events.fireEvent(ns, msg)
}

//
//// Room method returns a joined `Room`.
//func (ns *NSConn) Room(roomName string) *Room {
//	if ns == nil {
//		return nil
//	}
//
//	ns.roomsMutex.RLock()
//	room := ns.rooms[roomName]
//	ns.roomsMutex.RUnlock()
//
//	return room
//}
//
//// Rooms returns a slice copy of the joined rooms.
//func (ns *NSConn) Rooms() []*Room {
//	ns.roomsMutex.RLock()
//	rooms := make([]*Room, len(ns.rooms))
//	i := 0
//	for _, room := range ns.rooms {
//		rooms[i] = room
//		i++
//	}
//	ns.roomsMutex.RUnlock()
//
//	return rooms
//}
//
//// LeaveAll method sends a remote and local leave room signal `OnRoomLeave` to and for all rooms
//// and fires the `OnRoomLeft` event if succeed.
//func (ns *NSConn) LeaveAll(ctx context.Context) error {
//	if ns == nil {
//		return nil
//	}
//
//	ns.roomsMutex.Lock()
//	defer ns.roomsMutex.Unlock()
//
//	leaveMsg := Message{Namespace: ns.namespace, Event: OnRoomLeave, IsLocal: true, locked: true}
//	for room := range ns.rooms {
//		leaveMsg.Room = room
//		if err := ns.askRoomLeave(ctx, leaveMsg, false); err != nil {
//			return err
//		}
//	}
//
//	return nil
//}
//
//func (ns *NSConn) forceLeaveAll(isLocal bool) {
//	ns.roomsMutex.Lock()
//	defer ns.roomsMutex.Unlock()
//
//	leaveMsg := Message{Namespace: ns.namespace, Event: OnRoomLeave, IsForced: true, IsLocal: isLocal}
//	for room := range ns.rooms {
//		leaveMsg.Room = room
//		ns.events.fireEvent(ns, leaveMsg)
//
//		delete(ns.rooms, room)
//
//		leaveMsg.Event = OnRoomLeft
//		ns.events.fireEvent(ns, leaveMsg)
//
//		leaveMsg.Event = OnRoomLeave
//	}
//}
//
//func (ns *NSConn) askPartyJoin(ctx context.Context, partyID string) (*Party, error) {
//	if ns.Party != nil {
//		return nil, errors.New("rời phòng cái đi rồi yêu cầu cái khác bạn eiii")
//	}
//
//	joinMsg := Message{
//		Namespace: ns.namespace,
//		Event:     OnRoomJoin,
//	}
//
//	_, err := ns.Ask(ctx, joinMsg)
//	if err != nil {
//		return nil, err
//	}
//
//	err = ns.events.fireEvent(ns, joinMsg)
//	if err != nil {
//		return nil, err
//	}
//
//	room = newRoom(ns, roomName)
//	ns.roomsMutex.Lock()
//	ns.rooms[roomName] = room
//	ns.roomsMutex.Unlock()
//
//	joinMsg.Event = OnRoomJoined
//	ns.events.fireEvent(ns, joinMsg)
//	return room, nil
//}
//
//func (ns *NSConn) replyRoomJoin(msg Message) {
//	if ns == nil || msg.wait == "" || msg.isNoOp {
//		return
//	}
//
//	ns.roomsMutex.RLock()
//	_, ok := ns.rooms[msg.Room]
//	ns.roomsMutex.RUnlock()
//	if !ok {
//		err := ns.events.fireEvent(ns, msg)
//		if err != nil {
//			msg.Err = err
//			ns.Conn.Write(msg)
//			return
//		}
//		ns.roomsMutex.Lock()
//		ns.rooms[msg.Room] = newRoom(ns, msg.Room)
//		ns.roomsMutex.Unlock()
//
//		msg.Event = OnRoomJoined
//		ns.events.fireEvent(ns, msg)
//	}
//
//	ns.Conn.writeEmptyReply(msg.wait)
//}
//
//func (ns *NSConn) askRoomLeave(ctx context.Context, msg Message, lock bool) error {
//	if ns == nil {
//		return nil
//	}
//
//	if lock {
//		ns.roomsMutex.RLock()
//	}
//	_, ok := ns.rooms[msg.Room]
//	if lock {
//		ns.roomsMutex.RUnlock()
//	}
//
//	if !ok {
//		return ErrBadRoom
//	}
//
//	_, err := ns.Conn.Ask(ctx, msg)
//	if err != nil {
//		return err
//	}
//
//	// msg.IsLocal = true
//	err = ns.events.fireEvent(ns, msg)
//	if err != nil {
//		return err
//	}
//
//	if lock {
//		ns.roomsMutex.Lock()
//	}
//
//	delete(ns.rooms, msg.Room)
//
//	if lock {
//		ns.roomsMutex.Unlock()
//	}
//
//	msg.Event = OnRoomLeft
//	ns.events.fireEvent(ns, msg)
//
//	return nil
//}
//
//func (ns *NSConn) replyRoomLeave(msg Message) {
//	if ns == nil || msg.wait == "" || msg.isNoOp {
//		return
//	}
//
//	room := ns.Room(msg.Room)
//	if room == nil {
//		ns.Conn.writeEmptyReply(msg.wait)
//		return
//	}
//
//	// if client then we need to respond to server and delete the room without ask the local event.
//	if ns.Conn.IsClient() {
//		ns.events.fireEvent(ns, msg)
//
//		ns.roomsMutex.Lock()
//		delete(ns.rooms, msg.Room)
//		ns.roomsMutex.Unlock()
//
//		ns.Conn.writeEmptyReply(msg.wait)
//
//		msg.Event = OnRoomLeft
//		ns.events.fireEvent(ns, msg)
//		return
//	}
//
//	// server-side, check for error on the local event first.
//	err := ns.events.fireEvent(ns, msg)
//	if err != nil {
//		msg.Err = err
//		ns.Conn.Write(msg)
//		return
//	}
//
//	ns.roomsMutex.Lock()
//	delete(ns.rooms, msg.Room)
//	ns.roomsMutex.Unlock()
//
//	msg.Event = OnRoomLeft
//	ns.events.fireEvent(ns, msg)
//
//	ns.Conn.writeEmptyReply(msg.wait)
//}

//#region Tinh comment room, chuyen qua party
//Room
//#
//// JoinRoom method can be used to join a connection to a specific room, rooms are dynamic.
//// Returns the joined `Room`.
//func (ns *NSConn) JoinRoom(ctx context.Context, roomName string) (*Room, error) {
//	if ns == nil {
//		return nil, ErrWrite
//	}
//
//	return ns.askRoomJoin(ctx, roomName)
//}
//
//// Room method returns a joined `Room`.
//func (ns *NSConn) Room(roomName string) *Room {
//	if ns == nil {
//		return nil
//	}
//
//	ns.roomsMutex.RLock()
//	room := ns.rooms[roomName]
//	ns.roomsMutex.RUnlock()
//
//	return room
//}
//
//// Rooms returns a slice copy of the joined rooms.
//func (ns *NSConn) Rooms() []*Room {
//	ns.roomsMutex.RLock()
//	rooms := make([]*Room, len(ns.rooms))
//	i := 0
//	for _, room := range ns.rooms {
//		rooms[i] = room
//		i++
//	}
//	ns.roomsMutex.RUnlock()
//
//	return rooms
//}
//
//// LeaveAll method sends a remote and local leave room signal `OnRoomLeave` to and for all rooms
//// and fires the `OnRoomLeft` event if succeed.
//func (ns *NSConn) LeaveAll(ctx context.Context) error {
//	if ns == nil {
//		return nil
//	}
//
//	ns.roomsMutex.Lock()
//	defer ns.roomsMutex.Unlock()
//
//	leaveMsg := Message{Namespace: ns.namespace, Event: OnRoomLeave, IsLocal: true, locked: true}
//	for room := range ns.rooms {
//		leaveMsg.Room = room
//		if err := ns.askRoomLeave(ctx, leaveMsg, false); err != nil {
//			return err
//		}
//	}
//
//	return nil
//}
//
//func (ns *NSConn) forceLeaveAll(isLocal bool) {
//	ns.roomsMutex.Lock()
//	defer ns.roomsMutex.Unlock()
//
//	leaveMsg := Message{Namespace: ns.namespace, Event: OnRoomLeave, IsForced: true, IsLocal: isLocal}
//	for room := range ns.rooms {
//		leaveMsg.Room = room
//		ns.events.fireEvent(ns, leaveMsg)
//
//		delete(ns.rooms, room)
//
//		leaveMsg.Event = OnRoomLeft
//		ns.events.fireEvent(ns, leaveMsg)
//
//		leaveMsg.Event = OnRoomLeave
//	}
//}
//
//func (ns *NSConn) askRoomJoin(ctx context.Context, roomName string) (*Room, error) {
//	ns.roomsMutex.RLock()
//	room, ok := ns.rooms[roomName]
//	ns.roomsMutex.RUnlock()
//	if ok {
//		return room, nil
//	}
//
//	joinMsg := Message{
//		Namespace: ns.namespace,
//		Room:      roomName,
//		Event:     OnRoomJoin,
//		IsLocal:   true,
//	}
//
//	_, err := ns.Conn.Ask(ctx, joinMsg)
//	if err != nil {
//		return nil, err
//	}
//
//	err = ns.events.fireEvent(ns, joinMsg)
//	if err != nil {
//		return nil, err
//	}
//
//	room = newRoom(ns, roomName)
//	ns.roomsMutex.Lock()
//	ns.rooms[roomName] = room
//	ns.roomsMutex.Unlock()
//
//	joinMsg.Event = OnRoomJoined
//	ns.events.fireEvent(ns, joinMsg)
//	return room, nil
//}
//
//func (ns *NSConn) replyRoomJoin(msg Message) {
//	if ns == nil || msg.wait == "" || msg.isNoOp {
//		return
//	}
//
//	ns.roomsMutex.RLock()
//	_, ok := ns.rooms[msg.Room]
//	ns.roomsMutex.RUnlock()
//	if !ok {
//		err := ns.events.fireEvent(ns, msg)
//		if err != nil {
//			msg.Err = err
//			ns.Conn.Write(msg)
//			return
//		}
//		ns.roomsMutex.Lock()
//		ns.rooms[msg.Room] = newRoom(ns, msg.Room)
//		ns.roomsMutex.Unlock()
//
//		msg.Event = OnRoomJoined
//		ns.events.fireEvent(ns, msg)
//	}
//
//	ns.Conn.writeEmptyReply(msg.wait)
//}
//
//func (ns *NSConn) askRoomLeave(ctx context.Context, msg Message, lock bool) error {
//	if ns == nil {
//		return nil
//	}
//
//	if lock {
//		ns.roomsMutex.RLock()
//	}
//	_, ok := ns.rooms[msg.Room]
//	if lock {
//		ns.roomsMutex.RUnlock()
//	}
//
//	if !ok {
//		return ErrBadRoom
//	}
//
//	_, err := ns.Conn.Ask(ctx, msg)
//	if err != nil {
//		return err
//	}
//
//	// msg.IsLocal = true
//	err = ns.events.fireEvent(ns, msg)
//	if err != nil {
//		return err
//	}
//
//	if lock {
//		ns.roomsMutex.Lock()
//	}
//
//	delete(ns.rooms, msg.Room)
//
//	if lock {
//		ns.roomsMutex.Unlock()
//	}
//
//	msg.Event = OnRoomLeft
//	ns.events.fireEvent(ns, msg)
//
//	return nil
//}
//
//func (ns *NSConn) replyRoomLeave(msg Message) {
//	if ns == nil || msg.wait == "" || msg.isNoOp {
//		return
//	}
//
//	room := ns.Room(msg.Room)
//	if room == nil {
//		ns.Conn.writeEmptyReply(msg.wait)
//		return
//	}
//
//	// if client then we need to respond to server and delete the room without ask the local event.
//	if ns.Conn.IsClient() {
//		ns.events.fireEvent(ns, msg)
//
//		ns.roomsMutex.Lock()
//		delete(ns.rooms, msg.Room)
//		ns.roomsMutex.Unlock()
//
//		ns.Conn.writeEmptyReply(msg.wait)
//
//		msg.Event = OnRoomLeft
//		ns.events.fireEvent(ns, msg)
//		return
//	}
//
//	// server-side, check for error on the local event first.
//	err := ns.events.fireEvent(ns, msg)
//	if err != nil {
//		msg.Err = err
//		ns.Conn.Write(msg)
//		return
//	}
//
//	ns.roomsMutex.Lock()
//	delete(ns.rooms, msg.Room)
//	ns.roomsMutex.Unlock()
//
//	msg.Event = OnRoomLeft
//	ns.events.fireEvent(ns, msg)
//
//	ns.Conn.writeEmptyReply(msg.wait)
//}
//EndRoom
//#endregion
