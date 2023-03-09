package redis

import (
	"context"
	"errors"
	"github.com/WolffunGame/wolfsocket"
	"github.com/WolffunGame/wolfsocket/stackexchange/redis/protos"
	"github.com/go-redis/redis/v8"
	"google.golang.org/protobuf/proto"
	"log"
	"strings"
)

var (
	ErrNamespaceEmpty = errors.New("We do not accept messages with empty namespaces")
)

var exchangeServer ExchangeServer

type ExchangeServer interface {
	Broadcast(eventName string, msg *protos.RedisMessage) error
	//Ask(ctx context.Context, msg *protos.RedisMessage)  error
}

// This struct wraps the Redis and Neffos functionality
type EventServer struct {
	redisClient  *redis.Client
	neffosServer *wolfsocket.Server
	done         chan bool
}

// InvalidPrefix is returned when a message with a channel prefix
// that does not match the expected prefix is received during subscription.
// The message is not executed to prevent unauthorized access or incorrect behavior.
var (
	InvalidPrefix = errors.New("message received with invalid prefix")
	prefixChannel = "WSServer:"
)

func NewEventServer(redisClient *redis.Client, server *wolfsocket.Server, events map[string]wolfsocket.MessageHandlerFunc) *EventServer {
	eventServer := &EventServer{
		redisClient:  redisClient,
		neffosServer: server,
		done:         make(chan bool),
	}

	exchangeServer = eventServer
	//subri
	go eventServer.Run(events)
	return eventServer
}

func (es *EventServer) Init(redisClient *redis.Client, neffosServer *wolfsocket.Server) {
	es.redisClient = redisClient
	es.neffosServer = neffosServer
	es.done = make(chan bool)
}

func (es *EventServer) Close() {
	es.done <- true
}

func (es *EventServer) Run(events map[string]wolfsocket.MessageHandlerFunc) {
	pubsub := es.redisClient.Subscribe(context.Background())
	// Subscribe to channels corresponding to events
	for eventName := range events {
		err := pubsub.Subscribe(context.Background(), es.getChannel(eventName))
		if err != nil {
			log.Fatalf("Error subscribing to channel %s: %s", eventName, err)
		}
	}

	// Get a channel for incoming messages
	ch := pubsub.Channel()

	// Loop to handle incoming messages
	for {
		select {
		case msg := <-ch:
			// Handle the message
			eventName, err := es.getEventName(msg.Channel)
			if err != nil {
				//skip- log
				continue
			}
			eventHandler, ok := events[eventName]

			if ok {
				es.handleMessage([]byte(msg.Payload), eventHandler)
			}
		case <-es.done:
			// Unsubscribe from channels and close connection
			for eventName := range events {
				pubsub.Unsubscribe(context.Background(), es.getChannel(eventName))
			}
			pubsub.Close()
			es.redisClient.Close()
			return
		}
	}
}

func (r *EventServer) handleMessage(payload []byte, eventHandler wolfsocket.MessageHandlerFunc) error {
	redisMessage := protos.RedisMessage{}
	err := proto.Unmarshal(payload, &redisMessage)
	if err != nil {
		return err
	}

	receivers := redisMessage.To
	if len(receivers) == 0 {
		return nil
	}

	r.neffosServer.FindAndFire(func(conn *wolfsocket.Conn) {
		if len(redisMessage.Namespace) != 0 {
			//try get nsconn
			if nsconn := conn.Namespace(redisMessage.Namespace); nsconn != nil {
				eventHandler(nsconn, wolfsocket.Message{Body: redisMessage.Body})
			}
			return
		}
	}, receivers)

	return nil
}

// Broadcast to neffosServer
func (r *EventServer) Broadcast(eventName string, msg *protos.RedisMessage) error {
	if msg == nil || msg.Namespace == "" {
		return ErrNamespaceEmpty
	}

	data, err := proto.Marshal(msg)
	if err != nil {
		return err
	}

	channel := r.getChannel(eventName)
	return r.redisClient.Publish(context.Background(), channel, data).Err()
}

// Ask some neffosServer??
func Ask(ctx context.Context, connID string, serverConnUUID string) (err error) {
	//sub := radix.PersistentPubSub("", "", exc.connFunc)
	//msgCh := make(chan radix.PubSubMessage)
	//err = sub.Subscribe(msgCh, token)
	//if err != nil {
	//	return
	//}
	//defer sub.Close()
	//
	//if !exc.publish(msg) {
	//	return response, wolfsocket.ErrWrite
	//}
	//
	//select {
	//case <-ctx.Done():
	//	err = ctx.Err()
	//case redisMsg := <-msgCh:
	//	response = wolfsocket.DeserializeMessage(wolfsocket.TextMessage, redisMsg.Message, false, false)
	//	err = response.Err
	//}
	return
}

func (r *EventServer) getChannel(eventName string) string {
	return prefixChannel + eventName
}

func (r *EventServer) getEventName(channel string) (string, error) {
	if !strings.HasPrefix(channel, prefixChannel) {
		return "", InvalidPrefix
	}
	return strings.TrimPrefix(channel, prefixChannel), nil
}
