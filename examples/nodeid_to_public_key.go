package main

import (
	"log"
	"time"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/p2p/discover"
	whisper "github.com/ethereum/go-ethereum/whisper/whisperv6"
)

func main() {
	key, err := crypto.GenerateKey()
	if err != nil {
		log.Fatalf("error when generating a key: %v", err)
	}

	// nodeID is a marshaled version of a public key
	nodeID := discover.PubkeyID(&key.PublicKey)
	log.Printf("nodeID: %s", nodeID)

	// but we can get public key from node ID :yay:
	pubKey, err := nodeID.Pubkey()
	if err != nil {
		log.Fatalf("error when getting public key from nodeID: %v", err)
	}
	log.Printf("public key: %v", pubKey)

	// let's create a Whisper sent message
	payload := []byte("Status is awesome")
	sentMessage, err := whisper.NewSentMessage(&whisper.MessageParams{
		Payload: payload,
	})
	if err != nil {
		log.Fatalf("failed to create a Whisper sent message: %v", err)
	}
	log.Printf("sent message:\n%v\npayload:\n%v", sentMessage.Raw, payload)

	// and let's encrypt it with a public key get from the nodeID
	envelope, err := sentMessage.Wrap(&whisper.MessageParams{
		Dst:   pubKey,
		TTL:   10,
		Topic: whisper.BytesToTopic([]byte{0x01, 0x02, 0x03, 0x04}),
	}, time.Now())
	if err != nil {
		log.Fatalf("failed to wrap a message: %v", err)
	}
	log.Printf("envelope: %s", envelope.Hash().Hex())

	// finally, let's verify we can encrypt that message
	filter := whisper.Filter{
		KeyAsym: key,
	}
	receivedMessage := envelope.Open(&filter)
	if receivedMessage == nil {
		log.Fatalf("failed to open message")
	}
	log.Printf("received messaged payload: %s", receivedMessage.Payload)
}
