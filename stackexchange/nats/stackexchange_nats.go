package nats

import (
	"context"
	"github.com/WolffunGame/wolfsocket/stackexchange/redis/protos"
	"strings"
	"sync"

	"github.com/WolffunGame/wolfsocket"

	"github.com/nats-io/nats.go"
)

// StackExchange is a `wolfsocket.StackExchange` for nats
// based on https://nats-io.github.io/docs/developer/tutorials/pubsub.html.
type StackExchange struct {
	// options holds the nats options for clients.
	// Defaults to the `nats.GetDefaultOptions()` which
	// can be overridden by the `With` function on `NewStackExchange`.
	opts nats.Options
	// If you use the same nats server instance for multiple wolfsocket apps,
	// set this to different values across your apps.
	SubjectPrefix string

	publisher   *nats.Conn
	subscribers map[*wolfsocket.Conn]*subscriber

	addSubscriber chan *subscriber
	subscribe     chan subscribeAction
	unsubscribe   chan unsubscribeAction
	delSubscriber chan closeAction
}

var _ wolfsocket.StackExchange = (*StackExchange)(nil)

type (
	subscriber struct {
		conn    *wolfsocket.Conn
		subConn *nats.Conn

		// To unsubscribe a connection per namespace, set on subscribe channel.
		// Key is the subject pattern, with lock for any case, although
		// they shouldn't execute in parallel from wolfsocket conn itself.
		subscriptions map[string]*nats.Subscription
		mu            sync.RWMutex
	}

	subscribeAction struct {
		conn      *wolfsocket.Conn
		namespace string
	}

	unsubscribeAction struct {
		conn      *wolfsocket.Conn
		namespace string
	}

	closeAction struct {
		conn *wolfsocket.Conn
	}
)

// With accepts a nats.Options structure
// which contains the whole configuration
// and returns a nats.Option which can be passed
// to the `NewStackExchange`'s second input variadic argument.
// Note that use this method only when you want to override the default options
// at once.
func With(options nats.Options) nats.Option {
	return func(opts *nats.Options) error {
		*opts = options
		return nil
	}
}

// NewStackExchange returns a new nats StackExchange.
// The required field is "url" which should be in the form
// of nats connection string, e.g. nats://username:pass@localhost:4222.
// Other option is to leave the url with localhost:4222 and pass
// authentication options such as `nats.UserInfo(username, pass)` or
// nats.UserCredentials("./userCredsFile") at the second variadic input argument.
//
// Options can be used to register nats error and close handlers too.
//
// Alternatively, use the `With(nats.Options)` function to
// customize the client through struct fields.
func NewStackExchange(url string, options ...nats.Option) (*StackExchange, error) {
	// For subscribing:
	// Use a single client or create new for each new incoming websocket connection?
	// - nats does not have a connection pool and
	// - it uses callbacks for subscribers and
	// so I assumed it's tend to be uses as single client BUT inside its source code:
	// - the connect itself is done under its nats.go/Conn.connect()
	// - the reading is done through loop waits for each server message
	//   and it parses and stores field data using connection-level locks.
	// - and the subscriber at nats.go/Conn#waitForMsgs(s *Subscription) for channel use
	// also uses connection-level locks. ^ this is slower than callbacks,
	// callbacks are more low level there as far as my research goes.
	// So I will proceed with making a new nats connection for each websocket connection,
	// if anyone with more experience on nats than me has a different approach
	// we should listen to and process with actions on making it more efficient.
	// For publishing:
	// Create a connection, here, which will only be used to Publish.

	// Cache the options to be used on every client and
	// respect any customization by caller.
	opts := nats.GetDefaultOptions()
	if url == "" {
		url = nats.DefaultURL
	}
	opts.Url = url
	// TODO: export the wolfsocket.debugEnabled
	// and set that:
	// opts.Verbose = true

	opts.NoEcho = true

	for _, opt := range options {
		if opt == nil {
			continue
		}
		if err := opt(&opts); err != nil {
			return nil, err
		}
	}

	// opts.Url may change from caller, use the struct's field to respect it.
	servers := strings.Split(opts.Url, ",")
	for i, s := range servers {
		servers[i] = strings.TrimSpace(s)
	}
	// append to make sure that any custom servers from caller
	// are respected, no check for duplications.
	opts.Servers = append(opts.Servers, servers...)

	pubConn, err := opts.Connect()
	if err != nil {
		return nil, err
	}

	exc := &StackExchange{
		opts:          opts,
		SubjectPrefix: "wolfsocket",
		publisher:     pubConn,

		subscribers:   make(map[*wolfsocket.Conn]*subscriber),
		addSubscriber: make(chan *subscriber),
		delSubscriber: make(chan closeAction),
		subscribe:     make(chan subscribeAction),
		unsubscribe:   make(chan unsubscribeAction),
	}

	go exc.run()

	return exc, nil
}

func (exc *StackExchange) run() {
	for {
		select {
		case s := <-exc.addSubscriber:
			// wolfsocket.Debugf("[%s] added to potential subscribers", s.conn.ID())
			exc.subscribers[s.conn] = s
		case m := <-exc.subscribe:
			if sub, ok := exc.subscribers[m.conn]; ok {
				if sub.subConn.IsClosed() {
					// wolfsocket.Debugf("[%s] has an unexpected nats connection closing on subscribe", m.conn.ID())
					delete(exc.subscribers, m.conn)
					continue
				}

				subject := exc.getSubject(m.namespace, "", "")
				// wolfsocket.Debugf("[%s] subscribed to [%s]", m.conn.ID(), subject)
				subscription, err := sub.subConn.Subscribe(subject, makeMsgHandler(sub.conn))
				if err != nil {
					continue
				}
				sub.subConn.Flush()
				if err = sub.subConn.LastError(); err != nil {
					// wolfsocket.Debugf("[%s] OnSubscribe [%s] Last Error: %v", m.conn, subject, err)
					continue
				}

				sub.mu.Lock()
				if sub.subscriptions == nil {
					sub.subscriptions = make(map[string]*nats.Subscription)
				}
				sub.subscriptions[subject] = subscription
				sub.mu.Unlock()
			}
		case m := <-exc.unsubscribe:
			if sub, ok := exc.subscribers[m.conn]; ok {
				if sub.subConn.IsClosed() {
					// wolfsocket.Debugf("[%s] has an unexpected nats connection closing on unsubscribe", m.conn.ID())
					delete(exc.subscribers, m.conn)
					continue
				}

				subject := exc.getSubject(m.namespace, "", "")
				// wolfsocket.Debugf("[%s] unsubscribed from [%s]", subject)
				if sub.subscriptions == nil {
					continue
				}

				sub.mu.RLock()
				subscription, ok := sub.subscriptions[subject]
				sub.mu.RUnlock()
				if ok {
					subscription.Unsubscribe()
				}
			}
		case m := <-exc.delSubscriber:
			if sub, ok := exc.subscribers[m.conn]; ok {
				// wolfsocket.Debugf("[%s] disconnected", m.conn.ID())
				if sub.subConn.IsConnected() {
					sub.subConn.Close()
				}

				delete(exc.subscribers, m.conn)
			}
		}
	}
}

// Nats does not allow ending with ".", it uses pattern matching.
func (exc *StackExchange) getSubject(namespace, room, connID string) string {
	if connID != "" {
		// publish direct and let the server-side do the checks
		// of valid or invalid message to send on this particular client.
		return exc.SubjectPrefix + "." + connID
	}

	if namespace == "" && room != "" {
		// should never happen but give info for debugging.
		panic("namespace cannot be empty when sending to a namespace's room")
	}

	return exc.SubjectPrefix + "." + namespace
}

func makeMsgHandler(c *wolfsocket.Conn) nats.MsgHandler {
	return func(m *nats.Msg) {
		msg := c.DeserializeMessage(wolfsocket.TextMessage, m.Data)
		msg.FromStackExchange = true

		c.Write(msg)
	}
}

// OnConnect prepares the connection nats subscriber
// and subscribes to itself for direct wolfsocket messages.
// It's called automatically after the wolfsocket server's OnConnect (if any)
// on incoming client connections.
func (exc *StackExchange) OnConnect(c *wolfsocket.Conn) error {
	subConn, err := exc.opts.Connect()
	if err != nil {
		// wolfsocket.Debugf("[%s] OnConnect Error: %v", c, err)
		return err
	}

	selfSubject := exc.getSubject("", "", c.ID())
	// unsubscribes automatically on close.
	_, err = subConn.Subscribe(selfSubject, makeMsgHandler(c))
	if err != nil {
		// wolfsocket.Debugf("[%s] OnConnect.SelfSubscribe Error: %v", c, err)
		return err
	}

	subConn.Flush()

	if err = subConn.LastError(); err != nil {
		// maybe an invalid subject, send back to the client which will window.alert it.
		// wolfsocket.Debugf("[%s] OnConnect.SelfSubscribe Last Error: %v", c, err)
		return err
	}

	s := &subscriber{
		conn:    c,
		subConn: subConn,
	}

	exc.addSubscriber <- s

	return nil
}

// Publish publishes messages through nats.
// It's called automatically on wolfsocket broadcasting.
func (exc *StackExchange) Publish(msgs []wolfsocket.Message) bool {
	for _, msg := range msgs {
		if !exc.publish(msg) {
			return false
		}
	}

	return true
}

func (exc *StackExchange) publish(msg wolfsocket.Message) bool {
	subject := exc.getSubject(msg.Namespace, msg.Room, msg.To)
	b := msg.Serialize()

	err := exc.publisher.Publish(subject, b)
	// Let's not add logging options, let
	// any custom nats error handler alone.
	return err == nil
}

// Ask implements server Ask for nats. It blocks.
func (exc *StackExchange) Ask(ctx context.Context, msg wolfsocket.Message, token string) (response wolfsocket.Message, err error) {
	// for some reason we can't use the exc.publisher.Subscribe,
	// so create a new connection for subscription which will be terminated on message receive or timeout.
	subConn, err := exc.opts.Connect()

	if err != nil {
		return
	}

	ch := make(chan wolfsocket.Message)
	sub, err := subConn.Subscribe(token, func(m *nats.Msg) {
		ch <- wolfsocket.DeserializeMessage(wolfsocket.TextMessage, m.Data, false, false)
	})

	if err != nil {
		return response, err
	}

	defer sub.Unsubscribe()
	defer subConn.Close()

	if !exc.publish(msg) {
		return response, wolfsocket.ErrWrite
	}

	select {
	case <-ctx.Done():
		return response, ctx.Err()
	case response = <-ch:
		return response, response.Err
	}
}

// NotifyAsk notifies and unblocks a "msg" subscriber, called on a server connection's read when expects a result.
func (exc *StackExchange) NotifyAsk(msg wolfsocket.Message, token string) error {
	msg.ClearWait()
	err := exc.publisher.Publish(token, msg.Serialize())
	if err != nil {
		return err
	}
	exc.publisher.Flush()
	return exc.publisher.LastError()
}

// Subscribe subscribes to a specific namespace,
// it's called automatically on wolfsocket namespace connected.
func (exc *StackExchange) Subscribe(c *wolfsocket.Conn, namespace string) {
	exc.subscribe <- subscribeAction{
		conn:      c,
		namespace: namespace,
	}
}

// Unsubscribe unsubscribes from a specific namespace,
// it's called automatically on wolfsocket namespace disconnect.
func (exc *StackExchange) Unsubscribe(c *wolfsocket.Conn, namespace string) {
	exc.unsubscribe <- unsubscribeAction{
		conn:      c,
		namespace: namespace,
	}
}

// OnDisconnect terminates the connection's subscriber that
// created on the `OnConnect` method.
// It unsubscribes to all opened channels and
// closes the internal read messages channel.
// It's called automatically when a connection goes offline,
// manually by server or client or by network failure.
func (exc *StackExchange) OnDisconnect(c *wolfsocket.Conn) {
	exc.delSubscriber <- closeAction{conn: c}
}

func (exc *StackExchange) PublishServer(namespace string, msgs []protos.ServerMessage) error {
	//TODO implement me
	panic("implement me")
}

func (exc *StackExchange) AskServer(namespace string, msg protos.ServerMessage) (*protos.ReplyMessage, error) {
	//TODO implement me
	panic("implement me")
}
