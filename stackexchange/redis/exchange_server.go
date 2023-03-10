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
	done         chan struct{}
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
		done:        make(chan struct{}),
	}
	return eventServer
}

func (es *ExchangeServer) UseExchangeServer(server *wolfsocket.Server, namespaces wolfsocket.Namespaces) {
	es.neffosServer = server
	go es.run(namespaces)
}

func (es *ExchangeServer) Close() {
	es.done <- struct{}{}
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
				es.handleMessage(namespace, []byte(msg.Payload), event)
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

func (es *ExchangeServer) handleMessage(namespace string, payload []byte, event wolfsocket.Events) error {
	redisMessage := protos.ServerMessage{}
	err := proto.Unmarshal(payload, &redisMessage)
	if err != nil {
		return err
	}

	receivers := redisMessage.To
	if len(receivers) == 0 {
		return nil
	}

	es.neffosServer.FindAndFire(func(conn *wolfsocket.Conn) {
		//try get nsconn
		if nsconn := conn.Namespace(namespace); nsconn != nil {
			msg := wolfsocket.Message{
				Namespace:    namespace,
				Event:        redisMessage.EventName,
				FromExplicit: redisMessage.From,
				Body:         redisMessage.Body,
			}
			if redisMessage.ToClient {
				conn.Write(msg)
				return
			}
			//FireEvent and Reply to this message if this is a "ask"
			msg.Token = redisMessage.Token
			msg.IsServer = true
			errEvent := event.FireEvent(nsconn, msg)
			es.Reply(errEvent, redisMessage.Token)

		}
	}, receivers)

	return nil
}

func (es *ExchangeServer) PublishServer(namespace string, msgs []protos.ServerMessage) error {
	for _, msg := range msgs {
		if err := es.publish(namespace, &msg); err != nil {
			return err
		}
	}
	return nil
}

// Broadcast to neffosServer
func (es *ExchangeServer) publish(namespace string, msg *protos.ServerMessage) error {
	if msg == nil || namespace == "" {
		return ErrNamespaceEmpty
	}

	data, err := proto.Marshal(msg)
	if err != nil {
		return err
	}

	channel := es.getChannel(namespace)
	return es.redisClient.Publish(context.Background(), channel, data).Err()
}

func (es *ExchangeServer) AskServer(namespace string, msg protos.ServerMessage) (response *protos.ReplyMessage, err error) {
	if msg.Token == "" || namespace == "" {
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
	if err = es.publish(namespace, &msg); err != nil {
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

func (es *ExchangeServer) Reply(err error, token string) {
	if token == "" {
		return
	}
	msg := isReplyServer(err)
	data, err := proto.Marshal(msg)
	if err != nil {
		return
	}

	channel := es.getChannel(token)
	es.redisClient.Publish(context.Background(), channel, data)
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
