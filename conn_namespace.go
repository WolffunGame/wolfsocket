package wolfsocket

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"sync"
	"wolfsocket/options"
	"wolfsocket/stackexchange/protos"
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
	// rooms      map[string]*Room
	// roomsMutex sync.RWMutex

	// value is just a temporarily value.
	// Storage across event callbacks for this namespace.
	value reflect.Value

	Party Party

	onJoinMsg func(msgInvite Message) (Message, error)

	friends *Friends

	roomsChatMutex sync.RWMutex
	roomsChat      map[RoomChannel]RoomChat
}

func newNSConn(c *Conn, namespace string, events Events) *NSConn {
	return &NSConn{
		Conn:      c,
		namespace: namespace,
		events:    events,
		// rooms:     make(map[string]*Room),
		roomsChat: make(map[RoomChannel]RoomChat),
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

// only need input event-name + body data
func (nsConn *NSConn) SBroadcast(channel string, msg protos.ServerMessage, opts ...options.BroadcastOption) error {
	msg.Namespace = nsConn.namespace
	return nsConn.server().SBroadcast(channel, msg, opts...)
}

func (nsConn *NSConn) AskServer(ctx context.Context, channel string, msg protos.ServerMessage, opts ...options.BroadcastOption) (*protos.ReplyMessage, error) {
	msg.Namespace = nsConn.namespace
	err := mergeOptions(&msg, opts...)
	if err != nil {
		return nil, err
	}
	return nsConn.server().AskServer(ctx, channel, msg)
}

func mergeOptions(msg *protos.ServerMessage, opts ...options.BroadcastOption) error {
	for _, opt := range opts {
		err := opt(msg)
		if err != nil {
			return err
		}
	}
	return nil
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
		SetBinary: true,
	})
}

func (ns *NSConn) JoinMsg(msgInvite Message) (Message, error) {
	if ns.onJoinMsg == nil {
		return msgInvite, nil
	}
	return ns.onJoinMsg(msgInvite)
}

func (ns *NSConn) SetOnJoinMsg(fn func(msgInvite Message) (Message, error)) {
	ns.onJoinMsg = fn
}

func (ns *NSConn) FireEvent(msg Message) error {
	return ns.events.fireEvent(ns, msg)
}
