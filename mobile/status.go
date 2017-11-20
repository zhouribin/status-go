package status

import (
	"encoding/json"

	"github.com/status-im/status-go/geth/api"
	"github.com/status-im/status-go/geth/params"
)

// GenerateConfig returns a `NodeConfig` marshaled to JSON.
func GenerateConfig(datadir string, networkID int64, devMode bool) (string, error) {
	config, err := params.NewNodeConfig(datadir, uint64(networkID), devMode)
	if err != nil {
		return "", err
	}

	rawConfig, err := json.Marshal(config)
	if err != nil {
		return "", err
	}

	return string(rawConfig), nil
}

const (
	_ = iota
	// EventNodeReady means that the node is ready.
	EventNodeReady int = iota
	// EventNodeStopped means that the node stopped.
	EventNodeStopped
)

// Event represents a single event.
type Event struct {
	Type  int
	Error error
}

// EventBus is an object that can receive and handle events.
type EventBus interface {
	SendEvent(event *Event)
}

// API represents an object to interact with
// various Status components like node, jail and accounts.
type API struct {
	api      *api.StatusAPI
	eventBus EventBus
}

// NewAPI creates an API instance.
func NewAPI(eventBus EventBus) *API {
	statusAPI := api.NewStatusAPI()
	return &API{statusAPI, eventBus}
}

// SendEvent sends an event to EventBus.
func (a *API) sendEvent(event *Event) {
	if a.eventBus == nil {
		return
	}

	a.eventBus.SendEvent(event)
}

// StartNodeAsync start a node asynchronously.
// When the node is ready, a signal is sent.
func (a *API) StartNodeAsync(rawConfig string) error {
	config, err := params.LoadNodeConfig(rawConfig)
	if err != nil {
		return err
	}

	nodeReady, err := a.api.StartNodeAsync(config)

	go func() {
		<-nodeReady
		a.sendEvent(&Event{EventNodeReady, nil})
	}()

	return err
}

// StopNodeAsync stops the running node.
func (a *API) StopNodeAsync() error {
	nodeStopped, err := a.api.StopNodeAsync()

	go func() {
		<-nodeStopped
		a.sendEvent(&Event{EventNodeStopped, nil})
	}()

	return err
}

// AccountInfo contains information about a newly created account.
type AccountInfo struct {
	Address  string
	PubKey   string
	Mnemonic string
}

// CreateAccount creates a new account.
// It returns a struct with the account info.
func (a *API) CreateAccount(passphrase string) (*AccountInfo, error) {
	address, pubKey, mnemonic, err := a.api.CreateAccount(passphrase)
	if err != nil {
		return nil, err
	}

	return &AccountInfo{
		Address:  address,
		PubKey:   pubKey,
		Mnemonic: mnemonic,
	}, nil
}

// Logout removes whisper identities.
func (a *API) Logout() error {
	return a.api.Logout()
}

// SelectAccount selects a given account. It also adds the key
// to Whisper as an identity.
func (a *API) SelectAccount(address, password string) error {
	return a.api.SelectAccount(address, password)
}

// CallRPC makes an JSON-RPC call.
func (a *API) CallRPC(input string) string {
	return a.api.CallRPC(input)
}
