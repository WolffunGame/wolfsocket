package redis

import (
	"context"
	"errors"
	"fmt"
	"github.com/WolffunGame/wolfsocket"
	"github.com/WolffunGame/wolfsocket/stackexchange/redis/protos"
	"github.com/redis/go-redis/v9"
	"google.golang.org/protobuf/proto"
	"time"
)

// Config is used on the `StackExchange` package-level function.
// Can be used to customize the redis client dialer.
type Config = redis.UniversalOptions
type Client = redis.UniversalClient

type StackExchangeCfgs struct {
	RedisConfig Config
	Channel     string
	wolfsocket.Namespaces
	*wolfsocket.Server
}

// StackExchange is a `wolfsocket.StackExchange` for redis.
type StackExchange struct {
	channel string

	client Client

	neffosServer *wolfsocket.Server
	namespaces   wolfsocket.Namespaces

	subscribers map[*wolfsocket.Conn]*subscriber

	addSubscriber chan *subscriber
	subscribe     chan subscribeAction
	unsubscribe   chan unsubscribeAction
	delSubscriber chan closeAction
}

type (
	subscriber struct {
		conn   *wolfsocket.Conn
		pubSub *redis.PubSub
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

var _ wolfsocket.StackExchange = (*StackExchange)(nil)

// NewStackExchange returns a new redis StackExchange.
// The "channel" input argument is the channel prefix for publish and subscribe.
func NewStackExchange(cfg StackExchangeCfgs) (*StackExchange, error) {
	rdb := redis.NewUniversalClient(&cfg.RedisConfig)
	exc := &StackExchange{
		client: rdb,
		// If you are using one redis server for multiple wolfsocket servers,
		// use a different channel for each wolfsocket server.
		// Otherwise a message sent from one server to all of its own clients will go
		// to all clients of all wolfsocket servers that use the redis server.
		// We could use multiple channels but overcomplicate things here.
		channel:      cfg.Channel,
		namespaces:   cfg.Namespaces,
		neffosServer: cfg.Server,

		subscribers:   make(map[*wolfsocket.Conn]*subscriber),
		addSubscriber: make(chan *subscriber),
		delSubscriber: make(chan closeAction),
		subscribe:     make(chan subscribeAction),
		unsubscribe:   make(chan unsubscribeAction),
	}

	go exc.run()

	return exc, nil
}

func (exc *StackExchange) Close() {
	exc.client.Close()
	close(exc.addSubscriber)
	close(exc.delSubscriber)
	close(exc.subscribe)
	close(exc.unsubscribe)
	//close everything
}

func (exc *StackExchange) run() {
	for {
		select {
		case s := <-exc.addSubscriber:
			exc.subscribers[s.conn] = s
		case m := <-exc.subscribe:
			if sub, ok := exc.subscribers[m.conn]; ok {
				err := sub.pubSub.Subscribe(context.Background(), exc.getChannel(m.channel))
				if err != nil {
					exc.subscribe <- m //?? retry
					continue
				}
			}
		case m := <-exc.unsubscribe:
			if sub, ok := exc.subscribers[m.conn]; ok {
				_ = sub.pubSub.Unsubscribe(exc.ctx(), m.channel)
			}
		case m := <-exc.delSubscriber:
			if sub, ok := exc.subscribers[m.conn]; ok {
				// wolfsocket.Debugf("[%s] disconnected", m.conn.ID())
				_ = sub.pubSub.Close()
				delete(exc.subscribers, m.conn)
			}
		}
	}
}

//using getChannelV2
//func (exc *StackExchange) getChannel(namespace, room, connID string) string {
//	if connID != "" {
//		// publish direct and let the server-side do the checks
//		// of valid or invalid message to send on this particular client.
//		return exc.channel + "." + connID + "."
//	}
//
//	panic("connID cannot be empty")
//
//	//@Tinh comment, khong dung room
//	//if namespace == "" && room != "" {
//	//	//should never happen but give info for debugging.
//	//	panic("namespace cannot be empty when sending to a namespace's room")
//	//}
//	//return exc.channel + "." + namespace + "."
//}

// OnConnect prepares the connection redis subscriber
// and subscribes to itself for direct wolfsocket messagexc.
// It's called automatically after the wolfsocket server's OnConnect (if any)
// on incoming client connections.
func (exc *StackExchange) OnConnect(c *wolfsocket.Conn) error {
	pubSub := exc.client.Subscribe(nil)
	go func() {
		for {
			select {
			case msg := <-pubSub.Channel():
				exc.handleMessage(msg)
			}
		}
	}()
	s := &subscriber{
		conn:   c,
		pubSub: pubSub,
	}

	exc.addSubscriber <- s
	return nil
}

// Publish publishes messages through redis.
// It's called automatically on wolfsocket broadcasting.
func (exc *StackExchange) Publish(msgs []wolfsocket.Message) bool {
	//log
	return false
	//	for _, msg := range msgs {
	//		if !exc.publish(msg) {
	//			return false
	//		}
	//	}
	//
	//	return true
}
func (exc *StackExchange) PublishServer(namespace string, msgs []protos.ServerMessage) error {
	for _, msg := range msgs {
		if err := exc.publish(namespace, &msg); err != nil {
			return err
		}
	}

	return nil
}

func (exc *StackExchange) publish(namespace string, msg *protos.ServerMessage) error {
	if msg == nil || namespace == "" {
		return ErrNamespaceEmpty
	}

	data, err := proto.Marshal(msg)
	if err != nil {
		return err
	}

	channel := exc.getChannel(namespace)
	//return exc.redisClient.Publish(context.Background(), channel, data).Err()
	return exc.publishCommand(channel, data)
}

func (exc *StackExchange) publishCommand(channel string, b []byte) error {
	//cmd := radix.FlatCmd(nil, "PUBLISH", channel, b)
	return exc.client.Publish(exc.ctx(), channel, b).Err()
}

// Ask implements the server Ask feature for redis. It blocks until response.
func (exc *StackExchange) Ask(ctx context.Context, msg wolfsocket.Message, token string) (response wolfsocket.Message, err error) {
	panic("use ask server")
}

//	sub := exc.client.Subscribe(nil)
//	err = sub.Subscribe(ctx, token)
//	if err != nil {
//		return
//	}
//	defer sub.Close()
//
//	if !exc.publish(msg) {
//		return response, wolfsocket.ErrWrite
//	}
//
//	select {
//	case <-ctx.Done():
//		err = ctx.Err()
//	case redisMsg := <-sub.Channel():
//		response = wolfsocket.DeserializeMessage(wolfsocket.TextMessage, []byte(redisMsg.Payload), false, false)
//		err = response.Err
//	}
//
//	return
//}

// NotifyAsk notifies and unblocks a "msg" subscriber, called on a server connection's read when expects a result.
func (exc *StackExchange) NotifyAsk(msg wolfsocket.Message, token string) error {
	//
	msg.ClearWait()
	fmt.Println("haha???")
	return exc.publishCommand(token, msg.Serialize())
}

// Subscribe subscribes to a specific channel,
func (exc *StackExchange) Subscribe(c *wolfsocket.Conn, channel string) {
	exc.subscribe <- subscribeAction{
		conn:    c,
		channel: channel,
	}
}

// Unsubscribe unsubscribes from a specific channel,
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

func (exc *StackExchange) getChannel(key string) string {
	return prefixChannel + key
}

func (exc *StackExchange) handleMessage(redisMsg *redis.Message) error {
	serverMsg := protos.ServerMessage{}
	err := proto.Unmarshal([]byte(redisMsg.Payload), &serverMsg)
	if err != nil {
		return err
	}
	namespace := serverMsg.Namespace

	receivers := serverMsg.To
	if len(receivers) == 0 {
		return nil
	}

	event, exists := exc.namespaces[namespace]
	if !exists {
		return nil
	}

	exc.neffosServer.FindAndFire(func(conn *wolfsocket.Conn) {
		//try get nsconn
		if nsconn := conn.Namespace(namespace); nsconn != nil {
			msg := wolfsocket.Message{
				Namespace:    namespace,
				Event:        serverMsg.EventName,
				FromExplicit: serverMsg.From,
				Body:         serverMsg.Body,
			}
			if serverMsg.ToClient {
				conn.Write(msg)
				return
			}
			//FireEvent and Reply to this message if this is a "ask"
			msg.Token = serverMsg.Token
			msg.IsServer = true
			errEvent := event.FireEvent(nsconn, msg)
			//reply if to
			if serverMsg.Token == "" {
				_ = exc.Reply(errEvent, serverMsg.Token)
			}

		}
	}, receivers)

	return nil
}

func (exc *StackExchange) AskServer(namespace string, msg protos.ServerMessage) (response *protos.ReplyMessage, err error) {
	if msg.Token == "" || namespace == "" {
		err = wolfsocket.ErrInvalidPayload
		return
	}
	sub := exc.client.Subscribe(nil)
	ctx := exc.ctx()
	err = sub.Subscribe(ctx, exc.getChannel(msg.Token))
	if err != nil {
		return
	}
	defer sub.Close()
	if err = exc.publish(namespace, &msg); err != nil {
		return
	}

	select {
	case <-ctx.Done():
		err = ctx.Err()
	case redisMsg := <-sub.Channel():
		response = &protos.ReplyMessage{}
		err = proto.Unmarshal([]byte(redisMsg.Payload), response)
		return
	}

	return
}

func (exc *StackExchange) ctx() context.Context {
	ctx, _ := context.WithTimeout(context.Background(), 5*time.Second)
	return ctx
}

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

type replyServer struct {
	msg protos.ReplyMessage
}

func (r replyServer) Error() string {
	return ""
}

type errCode interface {
	ErrorCode() uint32
}

func isReplyServer(err error) *protos.ReplyMessage {
	if err != nil {
		if r, ok := err.(replyServer); ok {
			return &r.msg
		}
	}
	return &protos.ReplyMessage{Data: &protos.ReplyMessage_ErrorCode{ErrorCode: getErrCode(err)}}
}

func getErrCode(err error) uint32 {
	if e, ok := err.(errCode); ok {
		return e.ErrorCode()
	}
	return 99
}

// reply ask server
func ReplyServer(msg protos.ReplyMessage) error {
	return replyServer{msg}
}

var (
	ErrNamespaceEmpty = errors.New("We do not accept messages with empty namespaces")

	// InvalidPrefix is returned when a message with a channel prefix
	// that does not match the expected prefix is received during subscription.
	// The message is not executed to prevent unauthorized access or incorrect behavior.
	InvalidPrefix = errors.New("message received with invalid prefix")

	prefixChannel = "WSServer."
)
