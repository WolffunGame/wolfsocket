package redis

import (
	"context"
	"errors"
	"github.com/WolffunGame/wolfsocket"
	"github.com/WolffunGame/wolfsocket/stackexchange/redis/protos"
	"google.golang.org/protobuf/proto"
	"log"
	"strings"
	"time"
)

var (
	ErrNamespaceEmpty = errors.New("We do not accept messages with empty namespaces")
)

// This struct wraps the Redis and Neffos functionality
type ExchangeServer struct {
	redisClient  Client
	neffosServer *wolfsocket.Server
	done         chan bool
}

// InvalidPrefix is returned when a message with a channel prefix
// that does not match the expected prefix is received during subscription.
// The message is not executed to prevent unauthorized access or incorrect behavior.
var (
	InvalidPrefix = errors.New("message received with invalid prefix")
	prefixChannel = "WSServer."
)

func newEventServer(redisClient Client) *ExchangeServer {
	eventServer := &ExchangeServer{
		redisClient: redisClient,
		done:        make(chan bool),
	}
	return eventServer
}

func (es *ExchangeServer) UseExchangeServer(server *wolfsocket.Server, namespaces wolfsocket.Namespaces) {
	es.neffosServer = server
	go es.run(namespaces)
}

func (es *ExchangeServer) Close() {
	es.done <- true
}

func (es *ExchangeServer) run(namespaces wolfsocket.Namespaces) {
	pubsub := es.redisClient.Subscribe(nil)
	// Subscribe to channels corresponding to events
	//each namespace execute 5second
	timeout := time.Duration(len(namespaces) * 5)
	ctx, cancel := context.WithTimeout(context.Background(), timeout*time.Second)
	defer cancel()
	for namespace := range namespaces {
		err := pubsub.Subscribe(ctx, es.getChannel(namespace))
		if err != nil {
			log.Fatalf("Error subscribing to channel %s: %s", namespace, err)
		}
	}

	// Get a channel for incoming messages
	ch := pubsub.Channel()

	// Loop to handle incoming messages
	for {
		select {
		case msg := <-ch:
			// Handle the message
			namespace, err := es.getNameSpace(msg.Channel)
			if err != nil {
				//skip- log
				continue
			}
			if event, ok := namespaces[namespace]; ok {
				es.handleMessage([]byte(msg.Payload), event)
			}
		case <-es.done:
			// Unsubscribe from channels and close connection
			for namespace := range namespaces {
				pubsub.Unsubscribe(context.Background(), es.getChannel(namespace))
			}
			pubsub.Close()
			es.redisClient.Close()
			return
		}
	}
}

func (es *ExchangeServer) handleMessage(payload []byte, event wolfsocket.Events) error {
	redisMessage := protos.RedisMessage{}
	err := proto.Unmarshal(payload, &redisMessage)
	if err != nil {
		return err
	}

	receivers := redisMessage.To
	if len(receivers) == 0 {
		return nil
	}

	es.neffosServer.FindAndFire(func(conn *wolfsocket.Conn) {
		if len(redisMessage.Namespace) != 0 {
			//try get nsconn
			if nsconn := conn.Namespace(redisMessage.Namespace); nsconn != nil {
				msg := wolfsocket.Message{
					Namespace:    redisMessage.Namespace,
					Event:        redisMessage.EventName,
					FromExplicit: redisMessage.From,
					Body:         redisMessage.Body,
					Token:        redisMessage.Token,
					IsServer:     true,
				}
				if err := event.FireEvent(nsconn, msg); err != nil {
					if msg, b := isReplyServer(err); b {
						es.Reply(msg)
					}

					//log?
				}

			}
			return
		}
	}, receivers)

	return nil
}

func (es *ExchangeServer) PublishServer(msgs []protos.RedisMessage) error {
	for _, msg := range msgs {
		if err := es.publish(&msg); err != nil {
			return err
		}
	}
	return nil
}

// Broadcast to neffosServer
func (es *ExchangeServer) publish(msg *protos.RedisMessage) error {
	if msg == nil || msg.Namespace == "" {
		return ErrNamespaceEmpty
	}

	data, err := proto.Marshal(msg)
	if err != nil {
		return err
	}

	channel := es.getChannel(msg.Namespace)
	return es.redisClient.Publish(context.Background(), channel, data).Err()
}

func (es *ExchangeServer) AskServer(msg protos.RedisMessage) (response *protos.RedisMessage, err error) {
	if len(msg.Token) == 0 {
		err = wolfsocket.ErrInvalidPayload
		return
	}
	sub := es.redisClient.Subscribe(nil)
	ctx, cancel := es.ctx()
	defer cancel()
	err = sub.Subscribe(ctx, es.getChannel(msg.Token))
	if err != nil {
		return
	}
	defer sub.Close()
	if err = es.publish(&msg); err != nil {
		return
	}

	select {
	case <-ctx.Done():
		err = ctx.Err()
	case redisMsg := <-sub.Channel():
		response = &protos.RedisMessage{}
		err = proto.Unmarshal([]byte(redisMsg.Payload), response)
		return
	}

	return
}

func (es *ExchangeServer) getChannel(namespace string) string {
	return prefixChannel + namespace
}

func (es *ExchangeServer) getNameSpace(namespace string) (string, error) {
	if !strings.HasPrefix(namespace, prefixChannel) {
		return "", InvalidPrefix
	}
	return strings.TrimPrefix(namespace, prefixChannel), nil
}

func (es *ExchangeServer) ctx() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), 5*time.Second)
}

func (es *ExchangeServer) Reply(msg *protos.RedisMessage) {
	if len(msg.Token) == 0 {
		return
	}
	msg.Namespace = msg.Token
	msg.Token = ""
	es.publish(msg)
}

type replyServer struct {
	msg protos.RedisMessage
}

func (r replyServer) Error() string {
	return ""
}

func isReplyServer(err error) (*protos.RedisMessage, bool) {
	if err != nil {
		if r, ok := err.(replyServer); ok {
			return &r.msg, true
		}
	}
	return nil, false
}

// reply ask server
func ReplyServer(msg protos.RedisMessage) error {
	return replyServer{msg}
}
