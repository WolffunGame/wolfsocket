package redis

import (
	"context"
	"math/rand"
	"time"

	"github.com/WolffunService/wolfsocket"

	"github.com/mediocregopher/radix/v3"
)

// Config is used on the `StackExchange` package-level function.
// Can be used to customize the redis client dialer.
type Config struct {
	// Network to use.
	// Defaults to "tcp".
	Network string
	// Addr of a single redis server instance.
	// See "Clusters" field for clusters support.
	// Defaults to "127.0.0.1:6379".
	Addr string
	// Clusters a list of network addresses for clusters.
	// If not empty "Addr" is ignored.
	Clusters []string

	Password    string
	DialTimeout time.Duration

	// MaxActive defines the size connection pool.
	// Defaults to 10.
	MaxActive int
}

// StackExchange is a `wolfsocket.StackExchange` for redis.
type StackExchange struct {
	channel string

	pool     radix.Client
	connFunc radix.ConnFunc

	subscribers map[*wolfsocket.Conn]*subscriber

	addSubscriber chan *subscriber
	subscribe     chan subscribeAction
	unsubscribe   chan unsubscribeAction
	delSubscriber chan closeAction
}

type (
	subscriber struct {
		conn   *wolfsocket.Conn
		pubSub radix.PubSubConn
		msgCh  chan<- radix.PubSubMessage
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

var _ wolfsocket.StackExchange = (*StackExchange)(nil)

// NewStackExchange returns a new redis StackExchange.
// The "channel" input argument is the channel prefix for publish and subscribe.
func NewStackExchange(cfg Config, channel string) (*StackExchange, error) {
	if cfg.Network == "" {
		cfg.Network = "tcp"
	}

	if cfg.Addr == "" && len(cfg.Clusters) == 0 {
		cfg.Addr = "127.0.0.1:6379"
	}

	if cfg.DialTimeout < 0 {
		cfg.DialTimeout = 30 * time.Second
	}

	if cfg.MaxActive == 0 {
		cfg.MaxActive = 10
	}

	var dialOptions []radix.DialOpt

	if cfg.Password != "" {
		dialOptions = append(dialOptions, radix.DialAuthPass(cfg.Password))
	}

	if cfg.DialTimeout > 0 {
		dialOptions = append(dialOptions, radix.DialTimeout(cfg.DialTimeout))
	}

	var connFunc radix.ConnFunc
	var client radix.Client

	if len(cfg.Clusters) > 0 {
		cluster, err := radix.NewCluster(cfg.Clusters, radix.ClusterPoolFunc(func(network, addr string) (radix.Client, error) {
			return radix.NewPool(network, addr, cfg.MaxActive, radix.PoolConnFunc(func(network, addr string) (radix.Conn, error) {
				return radix.Dial(network, addr, radix.DialAuthPass(cfg.Password))
			}))
		}))
		if err != nil {
			// maybe an
			// ERR This instance has cluster support disabled
			return nil, err
		}
		client = cluster

		connFunc = func(network, addr string) (radix.Conn, error) {
			topo := cluster.Topo()
			node := topo[rand.Intn(len(topo))]
			return radix.Dial(cfg.Network, node.Addr, dialOptions...)
		}
	} else {
		connFunc = func(network, addr string) (radix.Conn, error) {
			return radix.Dial(cfg.Network, cfg.Addr, dialOptions...)
		}
		pool, err := radix.NewPool("", "", cfg.MaxActive, radix.PoolConnFunc(connFunc))
		if err != nil {
			return nil, err
		}

		client = pool
	}

	exc := &StackExchange{
		pool:     client,
		connFunc: connFunc,
		// If you are using one redis server for multiple wolfsocket servers,
		// use a different channel for each wolfsocket server.
		// Otherwise a message sent from one server to all of its own clients will go
		// to all clients of all wolfsocket servers that use the redis server.
		// We could use multiple channels but overcomplicate things here.
		channel: channel,

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
			exc.subscribers[s.conn] = s
			// wolfsocket.Debugf("[%s] added to potential subscribers", s.conn.ID())
		case m := <-exc.subscribe:
			if sub, ok := exc.subscribers[m.conn]; ok {
				channel := exc.getChannel(m.namespace, "", "")
				sub.pubSub.PSubscribe(sub.msgCh, channel)
				// wolfsocket.Debugf("[%s] subscribed to [%s] for namespace [%s]", m.conn.ID(), channel, m.namespace)
				//	} else {
				// wolfsocket.Debugf("[%s] tried to subscribe to [%s] namespace before 'OnConnect.addSubscriber'!", m.conn.ID(), m.namespace)
			}
		case m := <-exc.unsubscribe:
			if sub, ok := exc.subscribers[m.conn]; ok {
				channel := exc.getChannel(m.namespace, "", "")
				// wolfsocket.Debugf("[%s] unsubscribed from [%s]", channel)
				sub.pubSub.PUnsubscribe(sub.msgCh, channel)
			}
		case m := <-exc.delSubscriber:
			if sub, ok := exc.subscribers[m.conn]; ok {
				// wolfsocket.Debugf("[%s] disconnected", m.conn.ID())
				sub.pubSub.Close()
				close(sub.msgCh)
				delete(exc.subscribers, m.conn)
			}
		}
	}
}

func (exc *StackExchange) getChannel(namespace, room, connID string) string {
	if connID != "" {
		// publish direct and let the server-side do the checks
		// of valid or invalid message to send on this particular client.
		return exc.channel + "." + connID + "."
	}

	if namespace == "" && room != "" {
		// should never happen but give info for debugging.
		panic("namespace cannot be empty when sending to a namespace's room")
	}

	return exc.channel + "." + namespace + "."
}

// OnConnect prepares the connection redis subscriber
// and subscribes to itself for direct wolfsocket messages.
// It's called automatically after the wolfsocket server's OnConnect (if any)
// on incoming client connections.
func (exc *StackExchange) OnConnect(c *wolfsocket.Conn) error {
	redisMsgCh := make(chan radix.PubSubMessage)
	go func() {
		for redisMsg := range redisMsgCh {
			// wolfsocket.Debugf("[%s] send to client: [%s]", c.ID(), string(redisMsg.Message))
			msg := c.DeserializeMessage(wolfsocket.BinaryMessage, redisMsg.Message)
			msg.FromStackExchange = true

			c.Write(msg)
		}
	}()

	pubSub := radix.PersistentPubSub("", "", exc.connFunc)
	s := &subscriber{
		conn:   c,
		pubSub: pubSub,
		msgCh:  redisMsgCh,
	}
	selfChannel := exc.getChannel("", "", c.ID())
	pubSub.PSubscribe(redisMsgCh, selfChannel)

	exc.addSubscriber <- s

	return nil
}

// Publish publishes messages through redis.
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
	// channel := exc.getMessageChannel(c.ID(), msg)
	channel := exc.getChannel(msg.Namespace, msg.Room, msg.To)
	// wolfsocket.Debugf("[%s] publish to channel [%s] the data [%s]\n", msg.FromExplicit, channel, string(msg.Serialize()))

	err := exc.publishCommand(channel, msg.Serialize())
	return err == nil
}

func (exc *StackExchange) publishCommand(channel string, b []byte) error {
	cmd := radix.FlatCmd(nil, "PUBLISH", channel, b)
	return exc.pool.Do(cmd)
}

// Ask implements the server Ask feature for redis. It blocks until response.
func (exc *StackExchange) Ask(ctx context.Context, msg wolfsocket.Message, token string) (response wolfsocket.Message, err error) {
	sub := radix.PersistentPubSub("", "", exc.connFunc)
	msgCh := make(chan radix.PubSubMessage)
	err = sub.Subscribe(msgCh, token)
	if err != nil {
		return
	}
	defer sub.Close()

	if !exc.publish(msg) {
		return response, wolfsocket.ErrWrite
	}

	select {
	case <-ctx.Done():
		err = ctx.Err()
	case redisMsg := <-msgCh:
		response = wolfsocket.DeserializeMessage(wolfsocket.TextMessage, redisMsg.Message, false, false)
		err = response.Err
	}

	return
}

// NotifyAsk notifies and unblocks a "msg" subscriber, called on a server connection's read when expects a result.
func (exc *StackExchange) NotifyAsk(msg wolfsocket.Message, token string) error {
	msg.ClearWait()
	return exc.publishCommand(token, msg.Serialize())
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
