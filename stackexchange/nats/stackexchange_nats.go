package nats

import (
	"context"
	"errors"
	"fmt"
	"github.com/WolffunService/wolfsocket/metrics"
	"github.com/WolffunService/wolfsocket/stackexchange/protos"
	"github.com/golang/protobuf/proto"
	"strings"
	"sync"
	"time"

	"github.com/WolffunService/wolfsocket"

	"github.com/nats-io/nats.go"
)

type StackExchangeCfgs struct {
	SubjectPrefix string
	wolfsocket.Namespaces
	*wolfsocket.Server
}

func UserInfo(user, password string) nats.Option {
	return nats.UserInfo(user, password)
}

func Addr(addr string) nats.Option {
	return func(o *nats.Options) error {
		o.Url = addr
		return nil
	}
}

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

	neffosServer *wolfsocket.Server
	namespaces   wolfsocket.Namespaces

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
		conn    *wolfsocket.Conn
		channel string
	}

	unsubscribeAction struct {
		conn    *wolfsocket.Conn
		channel string
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
func NewStackExchange(cfg StackExchangeCfgs, options ...nats.Option) (*StackExchange, error) {
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
	opts.NoEcho = true

	for _, opt := range options {
		if opt == nil {
			continue
		}
		if err := opt(&opts); err != nil {
			return nil, err
		}
	}

	if opts.Url == "" {
		opts.Url = nats.DefaultURL
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
		SubjectPrefix: cfg.SubjectPrefix,
		//neffosServer:  cfg.Server,
		//namespaces:    cfg.Namespaces,
		publisher: pubConn,

		subscribers:   make(map[*wolfsocket.Conn]*subscriber),
		addSubscriber: make(chan *subscriber),
		delSubscriber: make(chan closeAction),
		subscribe:     make(chan subscribeAction),
		unsubscribe:   make(chan unsubscribeAction),
	}

	go exc.run()
	//go exc.serverPubSub(cfg.Namespaces)

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

				subject := exc.getChannel(m.channel)
				// wolfsocket.Debugf("[%s] subscribed to [%s]", m.conn.ID(), subject)
				subscription, err := sub.subConn.Subscribe(subject, exc.makeMsgHandler(sub.conn))
				if err != nil {
					continue
				}
				channel := strings.Split(m.channel, ".")
				if len(channel) > 1 {
					//Record prefix (party, chat,notify,..)
					metrics.RecordHubSubscription(channel[0])
				}
				sub.subConn.Flush()
				if err = sub.subConn.LastError(); err != nil {
					// wolfsocket.Debugf("[%s] OnSubscribe [%s] Last Error: %v", m.conn, subject, err)
					continue
				}
				wolfsocket.Debugf(m.conn.ID(), " - Subscribe - ", exc.getChannel(m.channel), " success !!!")
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

				subject := exc.getChannel(m.channel)
				// wolfsocket.Debugf("[%s] unsubscribed from [%s]", subject)
				if sub.subscriptions == nil {
					continue
				}

				sub.mu.RLock()
				subscription, ok := sub.subscriptions[subject]
				sub.mu.RUnlock()
				if ok {
					subscription.Unsubscribe()
					delete(sub.subscriptions, subject)

					channel := strings.Split(m.channel, ".")
					if len(channel) > 1 {
						//record prefix
						metrics.RecordHubUnsubscription(channel[0])
					}
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

// SubjectPrefix.type.id
func (exc *StackExchange) getChannel(key string) string {
	return fmt.Sprintf("%s.%s", exc.SubjectPrefix, key)
}

func (exc StackExchange) makeMsgHandler(c *wolfsocket.Conn) nats.MsgHandler {
	//return func(m *nats.Msg) {
	//	msg := c.DeserializeMessage(wolfsocket.TextMessage, m.Data)
	//	msg.FromStackExchange = true
	//
	//	c.Write(msg)
	//}

	return func(msg *nats.Msg) {
		exc.handleMessage(msg, c)
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

	s := &subscriber{
		conn:    c,
		subConn: subConn,
	}

	exc.addSubscriber <- s

	return nil
}

func (exc *StackExchange) handleMessage(natsMsg *nats.Msg, conn *wolfsocket.Conn) (err error) {
	if natsMsg == nil {
		//log
		return
	}

	serverMsg := protos.ServerMessage{}
	err = proto.Unmarshal(natsMsg.Data, &serverMsg)
	if err != nil {
		return
	}
	if conn.Is(serverMsg.ExceptSender) {
		return
	}

	defer func() {
		//reply if to
		if serverMsg.Token == "" {
			_ = exc.Reply(err, serverMsg.Token)
		}
	}()

	namespace := serverMsg.Namespace

	//get namespace conn
	nsconn := conn.Namespace(namespace)
	if nsconn == nil {
		return
	}

	msg := wolfsocket.Message{
		Namespace: namespace,
		Event:     serverMsg.EventName,
		Body:      serverMsg.Body,
		SetBinary: true,
	}

	//if msg for client, send back to remote
	if serverMsg.ToClient {
		conn.Write(msg)
		return
	}

	//FireEvent and Reply to this message if this is a "ask"
	msg.Token = serverMsg.Token
	msg.IsServer = true

	err = nsconn.FireEvent(msg)

	return
}

// reply ask message
func (exc *StackExchange) Reply(err error, token string) error {
	if token == "" {
		return nil
	}
	msg := isReplyServer(err)
	data, err := proto.Marshal(msg)
	if err != nil {
		return err
	}

	channel := exc.getChannel(token)
	return exc.publishCommand(channel, data)
}

// Publish publishes messages through nats.
// It's called automatically on wolfsocket broadcasting.
//func (exc *StackExchange) Publish(msgs []wolfsocket.Message) bool {
//	for _, msg := range msgs {
//		if !exc.publish(msg) {
//			return false
//		}
//	}
//
//	return true
//}

func (exc *StackExchange) publishCommand(channel string, b []byte) error {
	//wolfsocket.Debugf("publishCommand %s %s", channel, string(b))
	return exc.publisher.Publish(channel, b)
}

//func (exc *StackExchange) publish(msg wolfsocket.Message) bool {
//	subject := exc.getSubject(msg.Namespace, msg.Room, msg.To)
//	b := msg.Serialize()
//
//	err := exc.publisher.Publish(subject, b)
//	// Let's not add logging options, let
//	// any custom nats error handler alone.
//	return err == nil
//}

// Ask implements server Ask for nats. It blocks.
func (exc *StackExchange) Ask(ctx context.Context, msg wolfsocket.Message, token string) (response wolfsocket.Message, err error) {
	// for some reason we can't use the exc.publisher.Subscribe,
	// so create a new connection for subscription which will be terminated on message receive or timeout.
	//subConn, err := exc.opts.Connect()
	//
	//if err != nil {
	//	return
	//}
	//
	//ch := make(chan wolfsocket.Message)
	//sub, err := subConn.Subscribe(token, func(m *nats.Msg) {
	//	ch <- wolfsocket.DeserializeMessage(wolfsocket.TextMessage, m.Data, false, false)
	//})
	//
	//if err != nil {
	//	return response, err
	//}
	//
	//defer sub.Unsubscribe()
	//defer subConn.Close()
	//
	//if !exc.publish(msg) {
	//	return response, wolfsocket.ErrWrite
	//}
	//
	//select {
	//case <-ctx.Done():
	//	return response, ctx.Err()
	//case response = <-ch:
	//	return response, response.Err
	//}
	return response, nil
}

// NotifyAsk notifies and unblocks a "msg" subscriber, called on a server connection's read when expects a result.
func (exc *StackExchange) NotifyAsk(msg wolfsocket.Message, token string) error {
	//err := exc.publisher.Publish(token, msg.Serialize())
	//if err != nil {
	//	return err
	//}
	//exc.publisher.Flush()
	//return exc.publisher.LastError()
	return nil
}

// Subscribe subscribes to a specific channel,
// it's called automatically on wolfsocket namespace connected.
func (exc *StackExchange) Subscribe(c *wolfsocket.Conn, channel string) {
	exc.subscribe <- subscribeAction{
		conn:    c,
		channel: channel,
	}
}

// Unsubscribe unsubscribes from a specific channel,
// it's called automatically on wolfsocket namespace disconnect.
func (exc *StackExchange) Unsubscribe(c *wolfsocket.Conn, channel string) {
	exc.unsubscribe <- unsubscribeAction{
		conn:    c,
		channel: channel,
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

func (exc *StackExchange) Publish(channel string, msgs []protos.ServerMessage) error {
	for _, msg := range msgs {
		if err := exc.publish(channel, &msg); err != nil {
			return err
		}
	}
	return nil
}

func (exc *StackExchange) publish(channel string, msg *protos.ServerMessage) error {
	if msg == nil || channel == "" {
		return ErrChannelEmpty
	}

	data, err := proto.Marshal(msg)
	if err != nil {
		return err
	}

	//return exc.redisClient.Publish(context.Background(), channel, data).Err()
	return exc.publishCommand(exc.getChannel(channel), data)
}

func (exc *StackExchange) AskServer(ctx context.Context, channel string, msg protos.ServerMessage) (response *protos.ReplyMessage, err error) {
	if msg.Token == "" || channel == "" {
		err = wolfsocket.ErrInvalidPayload
		return
	}
	subConn, errConnect := exc.opts.Connect()
	if errConnect != nil {
		return nil, errConnect
	}
	defer subConn.Close()

	//chan receive message nats
	msgChan := make(chan *nats.Msg, 1)
	defer close(msgChan)

	_, _ = subConn.ChanSubscribe(exc.getChannel(msg.Token), msgChan)
	//if err != nil{
	//	return nil, errConnect
	//}
	//defer sub.Unsubscribe()

	if err = exc.publish(channel, &msg); err != nil {
		return
	}

	select {
	case <-ctx.Done():
		err = ctx.Err()
	case m := <-msgChan:
		response = &protos.ReplyMessage{}
		err = proto.Unmarshal(m.Data, response)
		return
	}
	return
}

//func (exc *StackExchange) serverPubSub(namespaces wolfsocket.Namespaces) {
//	ch := make(chan *nats.Msg, 100)
//	subConn, err := exc.opts.Connect()
//	if err != nil {
//		log.Fatal("Cannot Subscribe namespace channel")
//	}
//	for namespace, _ := range namespaces {
//		sub, err := subConn.Subscribe(exc.getChannel(namespace), func(msg *nats.Msg) {
//			ch <- msg
//		})
//		subConn.ChanSubscribe()
//		sub.Unsubscribe()
//		err := exc.opts.Subscribe(exc.ctx())
//		if err != nil {
//
//		}
//	}
//	// Loop to handle incoming messages
//	for {
//		select {
//		case msg := <-ch:
//			// Handle the message
//			namespace := strings.TrimPrefix(msg.Subject, exc.SubjectPrefix)
//			if event, ok := namespaces[namespace]; ok {
//				_ = exc.handleServerMessage(namespace, msg.Payload, event)
//			}
//		//case <-exc.close:
//		//	// Unsubscribe from channels and close connection
//		//	for namespace := range namespaces {
//		//		pubSub.Unsubscribe(exc.ctx(), exc.getChannel(namespace))
//		//	}
//		//	pubSub.Close()
//		//	return
//		//}
//	}
//
//}

func (exc *StackExchange) ctx() context.Context {
	ctx, _ := context.WithTimeout(context.Background(), 5*time.Second)
	return ctx
}

type replyServer struct {
	msg protos.ReplyMessage
}

func (r replyServer) Error() string {
	return ""
}

type errCode interface {
	ErrorCode() int
}

func isReplyServer(err error) *protos.ReplyMessage {
	if err != nil {
		if r, ok := err.(replyServer); ok {
			return &r.msg
		}
		return &protos.ReplyMessage{Data: &protos.ReplyMessage_ErrorCode{ErrorCode: getErrCode(err)}}
	}

	return &protos.ReplyMessage{Data: &protos.ReplyMessage_Body{Body: []byte{}}}
}

func getErrCode(err error) uint32 {
	if e, ok := err.(errCode); ok {
		return e.ErrorCode()
	}
	return 99
}

func ReplyServer(msg protos.ReplyMessage) error {
	return replyServer{
		msg: msg,
	}
}

var (
	ErrChannelEmpty = errors.New("We do not accept messages with empty channel")

	// InvalidPrefix is returned when a message with a channel prefix
	// that does not match the expected prefix is received during subscription.
	// The message is not executed to prevent unauthorized access or incorrect behavior.
	InvalidPrefix = errors.New("message received with invalid prefix")
)
