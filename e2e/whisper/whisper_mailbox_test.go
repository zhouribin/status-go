package whisper

import (
	"encoding/json"
	"os"
	"strconv"
	"testing"
	"time"

	"fmt"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/crypto"
	_ "github.com/ethereum/go-ethereum/p2p/discover"
	whisper "github.com/ethereum/go-ethereum/whisper/whisperv6"
	"github.com/status-im/status-go/e2e"
	"github.com/status-im/status-go/geth/api"
	"github.com/status-im/status-go/geth/log"
	"github.com/status-im/status-go/geth/rpc"
	. "github.com/status-im/status-go/testing"
	"github.com/stretchr/testify/suite"
	"sync/atomic"
)

type WhisperMailboxSuite struct {
	suite.Suite
}

func TestWhisperMailboxTestSuite(t *testing.T) {
	suite.Run(t, new(WhisperMailboxSuite))
}

func (s *WhisperMailboxSuite) TestRequestMessagesInGroupChat() {
	//Start mailbox, alice, bob, charlie node
	mailboxBackend, stop := s.startMailboxBackend()
	defer stop()

	aliceBackend, stop := s.startBackend("alice")
	defer stop()

	bobBackend, stop := s.startBackend("bob")
	defer stop()

	charlieBackend, stop := s.startBackend("charlie")
	defer stop()

	wAlice, _ := aliceBackend.NodeManager().WhisperService()
	wAlice.Name = "Alice"
	wBob, _ := bobBackend.NodeManager().WhisperService()
	wBob.Name = "Bob"
	wCharlie, _ := charlieBackend.NodeManager().WhisperService()
	wCharlie.Name = "Charlie"
	wMailboxBackend, _ := mailboxBackend.NodeManager().WhisperService()
	wMailboxBackend.Name = "Mail Server"

	//add mailbox to static peers
	mailboxNode, err := mailboxBackend.NodeManager().Node()
	s.Require().NoError(err)
	mailboxEnode := mailboxNode.Server().NodeInfo().Enode

	err = aliceBackend.NodeManager().AddPeer(mailboxEnode)
	s.Require().NoError(err)
	err = bobBackend.NodeManager().AddPeer(mailboxEnode)
	s.Require().NoError(err)
	err = charlieBackend.NodeManager().AddPeer(mailboxEnode)
	s.Require().NoError(err)
	//wait async processes on adding peer
	time.Sleep(time.Second)

	//get whisper service
	aliceWhisperService, err := aliceBackend.NodeManager().WhisperService()
	s.Require().NoError(err)
	bobWhisperService, err := bobBackend.NodeManager().WhisperService()
	s.Require().NoError(err)
	charlieWhisperService, err := charlieBackend.NodeManager().WhisperService()
	s.Require().NoError(err)
	//get rpc client
	aliceRPCClient := aliceBackend.NodeManager().RPCClient()
	bobRPCClient := bobBackend.NodeManager().RPCClient()
	charlieRPCClient := charlieBackend.NodeManager().RPCClient()

	//bob and charlie add mailserver key
	password := "status-offline-inbox"
	bobMailServerKeyID, err := bobWhisperService.AddSymKeyFromPassword(password)
	s.Require().NoError(err)
	charlieMailServerKeyID, err := charlieWhisperService.AddSymKeyFromPassword(password)
	s.Require().NoError(err)

	_ = bobMailServerKeyID
	_ = charlieMailServerKeyID

	//generate group chat symkey and topic
	groupChatKeyID, err := aliceWhisperService.GenerateSymKey()
	s.Require().NoError(err)
	groupChatKey, err := aliceWhisperService.GetSymKey(groupChatKeyID)
	s.Require().NoError(err)
	//generate group chat topic
	groupChatTopic := whisper.BytesToTopic([]byte("groupChatTopic"))
	groupChatPayload := newGroupChatParams(groupChatKey, groupChatTopic)
	payloadStr, err := groupChatPayload.Encode()
	s.Require().NoError(err)

	//Add bob and charlie create key pairs to receive symmetric key for group chat from alice
	bobKeyID, err := bobWhisperService.NewKeyPair()
	s.Require().NoError(err)
	bobKey, err := bobWhisperService.GetPrivateKey(bobKeyID)
	s.Require().NoError(err)
	bobPubkey := hexutil.Bytes(crypto.FromECDSAPub(&bobKey.PublicKey))
	bobAliceKeySendTopic := whisper.BytesToTopic([]byte("bobAliceKeySendTopic "))

	charlieKeyID, err := charlieWhisperService.NewKeyPair()
	s.Require().NoError(err)
	charlieKey, err := charlieWhisperService.GetPrivateKey(charlieKeyID)
	s.Require().NoError(err)
	charliePubkey := hexutil.Bytes(crypto.FromECDSAPub(&charlieKey.PublicKey))
	charlieAliceKeySendTopic := whisper.BytesToTopic([]byte("charlieAliceKeySendTopic "))

	//bob and charlie create message filter
	wBob.SetBloomFilter(whisper.TopicToBloom(bobAliceKeySendTopic))
	wBob.SetBloomFilter(whisper.TopicToBloom(charlieAliceKeySendTopic))
	wBob.SetBloomFilter(whisper.TopicToBloom(groupChatTopic))

	wCharlie.SetBloomFilter(whisper.TopicToBloom(bobAliceKeySendTopic))
	wCharlie.SetBloomFilter(whisper.TopicToBloom(charlieAliceKeySendTopic))
	wCharlie.SetBloomFilter(whisper.TopicToBloom(groupChatTopic))

	wAlice.SetBloomFilter(whisper.TopicToBloom(bobAliceKeySendTopic))
	wAlice.SetBloomFilter(whisper.TopicToBloom(charlieAliceKeySendTopic))
	wAlice.SetBloomFilter(whisper.TopicToBloom(groupChatTopic))

	bobMessageFilterID := s.createPrivateChatMessageFilter(bobRPCClient, bobKeyID, bobAliceKeySendTopic.String())
	s.createPrivateChatMessageFilter(bobRPCClient, bobKeyID, charlieAliceKeySendTopic.String())

	charlieMessageFilterID := s.createPrivateChatMessageFilter(charlieRPCClient, charlieKeyID, charlieAliceKeySendTopic.String())
	s.createPrivateChatMessageFilter(charlieRPCClient, charlieKeyID, bobAliceKeySendTopic.String())

	time.Sleep(2 * time.Second)

	//Alice send message with symkey and topic to bob and charlie
	s.postMessageToPrivate(aliceRPCClient, bobPubkey.String(), bobAliceKeySendTopic.String(), payloadStr)
	s.postMessageToPrivate(aliceRPCClient, charliePubkey.String(), charlieAliceKeySendTopic.String(), payloadStr)

	//wait to receive
	time.Sleep(5 * time.Second)

	//bob receive group chat data and add it to his node
	//1. bob get group chat details
	messages := s.getMessagesByMessageFilterID(bobRPCClient, bobMessageFilterID)
	s.Require().Equal(1, len(messages))
	bobGroupChatData := groupChatParams{}
	bobGroupChatData.Decode(messages[0]["payload"].(string))
	s.EqualValues(groupChatPayload, bobGroupChatData)

	//2. bob add symkey to his node
	bobGroupChatSymkeyID := s.addSymKey(bobRPCClient, bobGroupChatData.Key)
	s.Require().NotEmpty(bobGroupChatSymkeyID)

	//3. bob create message filter to node by group chat topic
	bobGroupChatMessageFilterID := s.createGroupChatMessageFilter(bobRPCClient, bobGroupChatSymkeyID, bobGroupChatData.Topic)

	//charlie receive group chat data and add it to his node
	//1. charlie get group chat details
	messages = s.getMessagesByMessageFilterID(charlieRPCClient, charlieMessageFilterID)
	s.Require().Equal(1, len(messages))
	charlieGroupChatData := groupChatParams{}
	charlieGroupChatData.Decode(messages[0]["payload"].(string))
	s.EqualValues(groupChatPayload, charlieGroupChatData)

	//2. charlie add symkey to his node
	charlieGroupChatSymkeyID := s.addSymKey(charlieRPCClient, charlieGroupChatData.Key)
	s.Require().NotEmpty(charlieGroupChatSymkeyID)

	//3. charlie create message filter to node by group chat topic
	charlieGroupChatMessageFilterID := s.createGroupChatMessageFilter(charlieRPCClient, charlieGroupChatSymkeyID, charlieGroupChatData.Topic)

	//alice send message to group chat
	helloWorldMessage := hexutil.Encode([]byte("Hello world!"))

	for i := 0; i < 100; i++ {
		s.postMessageToGroup(aliceRPCClient, groupChatKeyID, groupChatTopic.String(), helloWorldMessage)
		time.Sleep(500 * time.Millisecond)
	}
	time.Sleep(10 * time.Second) //it need to receive envelopes by bob and charlie nodes

	//bob receive group chat message
	messages = s.getMessagesByMessageFilterID(bobRPCClient, bobGroupChatMessageFilterID)
	s.Require().Equal(true, len(messages) > 0)
	s.Require().Equal(helloWorldMessage, messages[0]["payload"].(string))

	//charlie receive group chat message
	messages = s.getMessagesByMessageFilterID(charlieRPCClient, charlieGroupChatMessageFilterID)
	s.Require().Equal(true, len(messages) > 0)
	s.Require().Equal(helloWorldMessage, messages[0]["payload"].(string))

	log.Warn(fmt.Sprintf("Alice got: %v\n", atomic.LoadInt64(&wAlice.MessageCount)))
	log.Warn(fmt.Sprintf("Bob got: %v\n", atomic.LoadInt64(&wBob.MessageCount)))
	log.Warn(fmt.Sprintf("Charlie got: %v\n", atomic.LoadInt64(&wCharlie.MessageCount)))
	log.Warn(fmt.Sprintf("In total: %v\n", atomic.LoadInt64(&whisper.TotalCount)))
}

func newGroupChatParams(symkey []byte, topic whisper.TopicType) groupChatParams {
	groupChatKeyStr := hexutil.Bytes(symkey).String()
	return groupChatParams{
		Key:   groupChatKeyStr,
		Topic: topic.String(),
	}
}

type groupChatParams struct {
	Key   string
	Topic string
}

func (d *groupChatParams) Decode(i string) error {
	b, err := hexutil.Decode(i)
	if err != nil {
		return err
	}
	return json.Unmarshal(b, &d)
}

func (d *groupChatParams) Encode() (string, error) {
	payload, err := json.Marshal(d)
	if err != nil {
		return "", err
	}
	return hexutil.Bytes(payload).String(), nil
}

//Start status node
func (s *WhisperMailboxSuite) startBackend(name string) (*api.StatusBackend, func()) {
	datadir := "../../.ethereumtest/mailbox/" + name
	backend := api.NewStatusBackend()
	nodeConfig, err := e2e.MakeTestNodeConfig(GetNetworkID())
	nodeConfig.DataDir = datadir
	s.Require().NoError(err)
	s.Require().False(backend.IsNodeRunning())

	if addr, err := GetRemoteURL(); err == nil {
		nodeConfig.UpstreamConfig.Enabled = true
		nodeConfig.UpstreamConfig.URL = addr
	}

	nodeStarted, err := backend.StartNode(nodeConfig)
	s.Require().NoError(err)
	<-nodeStarted // wait till node is started
	s.Require().True(backend.IsNodeRunning())

	return backend, func() {
		s.True(backend.IsNodeRunning())
		backendStopped, err := backend.StopNode()
		s.NoError(err)
		<-backendStopped
		s.False(backend.IsNodeRunning())
		os.RemoveAll(datadir)
	}
}

//Start mailbox node
func (s *WhisperMailboxSuite) startMailboxBackend() (*api.StatusBackend, func()) {
	mailboxBackend := api.NewStatusBackend()
	mailboxConfig, err := e2e.MakeTestNodeConfig(GetNetworkID())
	s.Require().NoError(err)
	datadir := "../../.ethereumtest/mailbox/mailserver/"

	mailboxConfig.LightEthConfig.Enabled = false
	mailboxConfig.WhisperConfig.Enabled = true
	mailboxConfig.KeyStoreDir = "../../.ethereumtest/mailbox/mailserver"
	mailboxConfig.WhisperConfig.EnableMailServer = true
	mailboxConfig.WhisperConfig.IdentityFile = "../../static/keys/wnodekey"
	mailboxConfig.WhisperConfig.PasswordFile = "../../static/keys/wnodepassword"
	mailboxConfig.WhisperConfig.DataDir = "../../.ethereumtest/mailbox/mailserver/data"
	mailboxConfig.DataDir = datadir

	mailboxNodeStarted, err := mailboxBackend.StartNode(mailboxConfig)
	s.Require().NoError(err)
	<-mailboxNodeStarted // wait till node is started
	s.Require().True(mailboxBackend.IsNodeRunning())
	return mailboxBackend, func() {
		s.True(mailboxBackend.IsNodeRunning())
		backendStopped, err := mailboxBackend.StopNode()
		s.NoError(err)
		<-backendStopped
		s.False(mailboxBackend.IsNodeRunning())
		os.RemoveAll(datadir)
	}
}

//createPrivateChatMessageFilter create message filter with asymmetric encryption
func (s *WhisperMailboxSuite) createPrivateChatMessageFilter(rpcCli *rpc.Client, privateKeyID string, topic string) string {
	req := `{
			"jsonrpc": "2.0",
			"method": "shh_newMessageFilter", "params": [
				{"privateKeyID": "` + privateKeyID + `", "topics": [ "` + topic + `"], "allowP2P":true}
			],
			"id": 1
		}`

	resp := rpcCli.CallRaw(req)
	msgFilterResp := returnedIDResponse{}
	err := json.Unmarshal([]byte(resp), &msgFilterResp)
	messageFilterID := msgFilterResp.Result
	s.Require().NoError(err)
	s.Require().Nil(msgFilterResp.Err)
	s.Require().NotEqual("", messageFilterID, resp)
	return messageFilterID
}

//createGroupChatMessageFilter create message filter with symmetric encryption
func (s *WhisperMailboxSuite) createGroupChatMessageFilter(rpcCli *rpc.Client, symkeyID string, topic string) string {
	req := `{
			"jsonrpc": "2.0",
			"method": "shh_newMessageFilter", "params": [
				{"symKeyID": "` + symkeyID + `", "topics": [ "` + topic + `"], "allowP2P":true}
			],
			"id": 1
		}`

	resp := rpcCli.CallRaw(req)
	msgFilterResp := returnedIDResponse{}
	err := json.Unmarshal([]byte(resp), &msgFilterResp)
	messageFilterID := msgFilterResp.Result
	s.Require().NoError(err)
	s.Require().Nil(msgFilterResp.Err)
	s.Require().NotEqual("", messageFilterID, resp)
	return messageFilterID
}

func (s *WhisperMailboxSuite) postMessageToPrivate(rpcCli *rpc.Client, bobPubkey string, topic string, payload string) {
	resp := rpcCli.CallRaw(`{
		"jsonrpc": "2.0",
		"method": "shh_post",
		"params": [
			{
			"pubKey": "` + bobPubkey + `",
			"topic": "` + topic + `",
			"payload": "` + payload + `",
			"powTarget": 0.001,
			"powTime": 2
			}
		],
		"id": 1}`)
	postResp := baseRPCResponse{}
	err := json.Unmarshal([]byte(resp), &postResp)
	s.Require().NoError(err)
	s.Require().Nil(postResp.Err)
}

func (s *WhisperMailboxSuite) postMessageToGroup(rpcCli *rpc.Client, groupChatKeyID string, topic string, payload string) {
	resp := rpcCli.CallRaw(`{
		"jsonrpc": "2.0",
		"method": "shh_post",
		"params": [
			{
			"symKeyID": "` + groupChatKeyID + `",
			"topic": "` + topic + `",
			"payload": "` + payload + `",
			"powTarget": 1,
			"powTime": 2
			}
		],
		"id": 1}`)
	postResp := baseRPCResponse{}
	err := json.Unmarshal([]byte(resp), &postResp)
	s.Require().NoError(err)
	s.Require().Nil(postResp.Err)
}

//getMessagesByMessageFilterID get received messages by messageFilterID
func (s *WhisperMailboxSuite) getMessagesByMessageFilterID(rpcCli *rpc.Client, messageFilterID string) []map[string]interface{} {
	resp := rpcCli.CallRaw(`{
		"jsonrpc": "2.0",
		"method": "shh_getFilterMessages",
		"params": ["` + messageFilterID + `"],
		"id": 1}`)
	messages := getFilterMessagesResponse{}
	err := json.Unmarshal([]byte(resp), &messages)
	s.Require().NoError(err)
	s.Require().Nil(messages.Err)
	return messages.Result
}

//addSymKey added symkey to node and return symkeyID
func (s *WhisperMailboxSuite) addSymKey(rpcCli *rpc.Client, symkey string) string {
	resp := rpcCli.CallRaw(`{"jsonrpc":"2.0","method":"shh_addSymKey",
			"params":["` + symkey + `"],
			"id":1}`)
	symkeyAddResp := returnedIDResponse{}
	err := json.Unmarshal([]byte(resp), &symkeyAddResp)
	s.Require().NoError(err)
	s.Require().Nil(symkeyAddResp.Err)
	symkeyID := symkeyAddResp.Result
	s.Require().NotEmpty(symkeyID)
	return symkeyID
}

//requestHistoricMessages ask mailnode to resend messagess
func (s *WhisperMailboxSuite) requestHistoricMessages(rpcCli *rpc.Client, mailboxEnode, mailServerKeyID, topic string) {
	resp := rpcCli.CallRaw(`{
		"jsonrpc": "2.0",
		"id": 2,
		"method": "shh_requestMessages",
		"params": [{
					"mailServerPeer":"` + mailboxEnode + `",
					"topic":"` + topic + `",
					"symKeyID":"` + mailServerKeyID + `",
					"from":0,
					"to":` + strconv.FormatInt(time.Now().Unix(), 10) + `
		}]
	}`)
	reqMessagesResp := baseRPCResponse{}
	err := json.Unmarshal([]byte(resp), &reqMessagesResp)
	s.Require().NoError(err)
	s.Require().Nil(reqMessagesResp.Err)
}

type getFilterMessagesResponse struct {
	Result []map[string]interface{}
	Err    interface{}
}

type returnedIDResponse struct {
	Result string
	Err    interface{}
}
type baseRPCResponse struct {
	Result interface{}
	Err    interface{}
}
