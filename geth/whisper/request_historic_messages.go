package whisper

import (
	"context"
	"crypto/ecdsa"
	"encoding/binary"
	"errors"
	"time"

	"github.com/ethereum/go-ethereum/whisper/whisperv5"
	"github.com/status-im/status-go/geth/common"
	"github.com/status-im/status-go/geth/rpc"
)

// whisper related errors
var (
	ErrInvalidNumberOfArgs      = errors.New("invalid number of arguments, expected 1")
	ErrInvalidArgs              = errors.New("invalid args")
	ErrTopicNotExist            = errors.New("topic value does not exist")
	ErrTopicNotString           = errors.New("topic value is not string")
	ErrMailboxSymkeyIDNotExist  = errors.New("symKeyID does not exist")
	ErrMailboxSymkeyIDNotString = errors.New("symKeyID is not string")
	ErrPeerNotExist             = errors.New("peer does not exist")
	ErrPeerNotString            = errors.New("peer is not string")
)

const defaultWorkTime = 5

// RequestHistoricMessagesHandler returns an RPC handler which sends a p2p request for historic messages.
func RequestHistoricMessagesHandler(nodeManager common.NodeManager) (rpc.Handler, error) {
	whisper, err := nodeManager.WhisperService()
	if err != nil {
		return nil, err
	}

	node, err := nodeManager.Node()
	if err != nil {
		return nil, err
	}

	return func(ctx context.Context, args ...interface{}) (interface{}, error) {
		r, err := requestFromArgs(args)
		if err != nil {
			return nil, err
		}

		symkey, err := whisper.GetSymKey(r.SymkeyID)
		if err != nil {
			return nil, err
		}

		r.PoW = whisper.MinPow()
		env, err := makeEnvelop(r, symkey, node.Server().PrivateKey)
		if err != nil {
			return nil, err
		}

		err = whisper.RequestHistoricMessages(r.Peer, env)
		if err != nil {
			return nil, err
		}

		return true, nil
	}, nil
}

type historicMessagesRequest struct {
	Peer     []byte              // mailbox peer
	TimeLow  uint32              // resend messages from
	TimeUp   uint32              // resend messages to
	Topic    whisperv5.TopicType // resend messages by topic
	SymkeyID string              // mailbox symmetric key id
	PoW      float64             // whisper proof of work
}

// requestFromArgs constructs new request from arguments.
func requestFromArgs(args ...interface{}) (historicMessagesRequest, error) {
	var r = historicMessagesRequest{
		TimeLow: uint32(time.Now().Add(-24 * time.Hour).Unix()),
		TimeUp:  uint32(time.Now().Unix()),
	}

	if len(args) != 1 {
		return historicMessagesRequest{}, ErrInvalidNumberOfArgs
	}

	historicMessagesArgs, ok := args[0].(map[string]interface{})
	if !ok {
		return historicMessagesRequest{}, ErrInvalidArgs
	}

	if t, ok := historicMessagesArgs["from"]; ok {
		if parsed, ok := t.(uint32); ok {
			r.TimeLow = parsed
		}
	}
	if t, ok := historicMessagesArgs["to"]; ok {
		if parsed, ok := t.(uint32); ok {
			r.TimeUp = parsed
		}
	}
	topicInterfaceValue, ok := historicMessagesArgs["topic"]
	if !ok {
		return historicMessagesRequest{}, ErrTopicNotExist
	}

	topicStringValue, ok := topicInterfaceValue.(string)
	if !ok {
		return historicMessagesRequest{}, ErrTopicNotString
	}

	if err := r.Topic.UnmarshalText([]byte(topicStringValue)); err != nil {
		return historicMessagesRequest{}, nil
	}

	symkeyIDInterfaceValue, ok := historicMessagesArgs["symKeyID"]
	if !ok {
		return historicMessagesRequest{}, ErrMailboxSymkeyIDNotExist
	}
	r.SymkeyID, ok = symkeyIDInterfaceValue.(string)
	if !ok {
		return historicMessagesRequest{}, ErrMailboxSymkeyIDNotString
	}

	peerInterfaceValue, ok := historicMessagesArgs["peer"]
	if !ok {
		return historicMessagesRequest{}, ErrPeerNotExist
	}
	r.Peer, ok = peerInterfaceValue.([]byte)
	if !ok {
		return historicMessagesRequest{}, ErrPeerNotString
	}

	return r, nil
}

// makeEnvelop makes envelop for request historic messages.
// symmetric key to authenticate to MailServer node and pk is the current node ID.
func makeEnvelop(r historicMessagesRequest, symkey []byte, pk *ecdsa.PrivateKey) (*whisperv5.Envelope, error) {
	var params whisperv5.MessageParams
	params.PoW = r.PoW
	params.Payload = makePayloadData(r)
	params.KeySym = symkey
	params.WorkTime = defaultWorkTime
	params.Src = pk

	message, err := whisperv5.NewSentMessage(&params)
	if err != nil {
		return nil, err
	}
	return message.Wrap(&params)
}

// makePayloadData makes specific payload for mailserver.
func makePayloadData(r historicMessagesRequest) []byte {
	data := make([]byte, 8+whisperv5.TopicLength)
	binary.BigEndian.PutUint32(data, r.TimeLow)
	binary.BigEndian.PutUint32(data[4:], r.TimeUp)
	copy(data[8:], r.Topic[:])
	return data
}
