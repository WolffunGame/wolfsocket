package wolfsocket

import (
	"context"
	"wolfsocket/stackexchange/protos"
)

// StackExchange is an optional interface
// that can be used to change the way neffos
// sends messages to its clients, i.e
// communication between multiple neffos servers.
//
// See the "kataras/neffos/stackexchange" subpackage for more details.
// Real-World example and usage documentation
// can be found at: "kataras/neffos/_examples/redis".
type StackExchange interface {
	// OnConnect should prepare the connection's subscriber.
	// It's called automatically after the neffos server's OnConnect (if any)
	// on incoming client connections.
	OnConnect(c *Conn) error
	// OnDisconnect should close the connection's subscriber that
	// created on the `OnConnect` method.
	// It's called automatically when a connection goes offline,
	// manually by server or client or by network failure.
	OnDisconnect(c *Conn)

	// Publish should publish messages through a stackexchange.
	// It's called automatically on neffos broadcasting.
	// Publish(msgs []Message) bool

	// Subscribe should subscribe to a specific namespace,
	// it's called automatically on neffos namespace connected.
	Subscribe(c *Conn, namespace string)
	// Unsubscribe should unsubscribe from a specific namespace,
	// it's called automatically on neffos namespace disconnect.
	Unsubscribe(c *Conn, namespace string) // should close the subscriber.
	// Ask should be able to perform a server Ask to a specific client or to all clients
	// It blocks until response from a specific client if msg.To is filled,
	// otherwise will return on the first responder's reply.
	// DEPRECATED:  use AskServer
	Ask(ctx context.Context, msg Message, token string) (Message, error)
	// NotifyAsk should notify and unblock a subscribed connection for this
	// specific message, "token" is the neffos wait signal for this message.
	NotifyAsk(msg Message, token string) error

	// PublishServer should publish messages through a stackexchange.
	// It's called automatically on neffos broadcasting when toClient equal false.
	Publish(channel string, msg protos.ServerMessage) error

	AskServer(ctx context.Context, channel string, msg protos.ServerMessage) (*protos.ReplyMessage, error)
}

// StackExchangeInitializer is an optional interface for a `StackExchange`.
// It contains a single `Init` method which accepts
// the registered server namespaces  and returns error.
// It does not called on manual `Server.StackExchange` field set,
// use the `Server.UseStackExchange` to make sure that this implementation is respected.
type StackExchangeInitializer interface {
	// Init should initialize a stackexchange, it's optional.
	Init(Namespaces) error
}

func stackExchangeInit(s StackExchange, namespaces Namespaces) error {
	if s != nil {
		if sinit, ok := s.(StackExchangeInitializer); ok {
			return sinit.Init(namespaces)
		}
	}

	return nil
}

type PublishOption struct {
}
