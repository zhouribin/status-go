package main

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/whisper/whisperv5"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"
)

const (
	WNODE_BIN   = "./wnode-status"
	STATUSD_BIN = "./statusd"
)

//see api at https://github.com/ethereum/go-ethereum/wiki/Whisper-v5-RPC-API
/*
{"jsonrpc": "2.0", "method": "shh_newSymKey", "params": [], "id": 9999999999}

{"jsonrpc": "2.0", "method": "shh_post", "params": [
{
"symKeyID": "14af8a4f90f90bff29ff24ee41e35d95684ddb513afc3c0f3fc15557a603ebd4",
"topic": "0xe00123a5",
"payload": "0x73656e74206265666f72652066696c7465722077617320616374697665202873796d6d657472696329",
"powTarget": 0.001,
"powTime": 2
}
], "id": 9999999999}

{"jsonrpc": "2.0", "method": "shh_newMessageFilter", "params": [
{"symKeyID": "e48b4ef9a55ff1f4f87d3dc9883fdf9a53fc7d982e29af83d13048320b70ed65", "topics": [ "0xdeadbeef", "0xbeefdead", "0x20028f4c"]}
], "id": 9999999999}

{"jsonrpc": "2.0", "method": "shh_getFilterMessages", "params": ["fc202cbafc840d249420dd9a61bfd8bf6a9b339f336359aafad5e5f2aca71901"], "id": 9999999999}
*/

type shhPost struct {
	SymKeyId   string  `json:"symKeyID,omitempty"`
	PubKey     string  `json:"pubKey,omitempty"`
	Topic      string  `json:"topic,omitempty"`
	Payload    string  `json:"payload"`
	PowTarget  float32 `json:"powTarget"`
	PowTime    int     `json:"powTime"`
	TTL        int     `json:"TTL"`
	TargetPeer string  `json:"targetPeer,omitempty"`
	Sig        string  `json:"sig,omitempty"`
}
type shhNewMessageFilter struct {
	SymKeyId     string   `json:"symKeyID,omitempty"`
	PrivateKeyID string   `json:"privateKeyID,omitempty"`
	Topics       []string `json:"topics,omitempty"`
	AllowP2P     bool     `json:"allowP2P,omitempty"`
}

//{"jsonrpc": "2.0", "method": "admin_nodeInfo", "params": [], "id": 9999999999}
/*
{
	"id": "5db6b0e6f9bc762b76b4a50180b2f35c22ab12bf465d805958340800b070bd364f9ec40fe1f76db780baad1cbab96b7a60e02f106daf3cf9a1de77d326888741",
	"name": "StatusIM/v0.9.9-unstable/linux-amd64/go1.8.3",
	"enode": "enode://5db6b0e6f9bc762b76b4a50180b2f35c22ab12bf465d805958340800b070bd364f9ec40fe1f76db780baad1cbab96b7a60e02f106daf3cf9a1de77d326888741@[::]:30303?discport=0",
	"ip": "::",
	"ports": {
		"discovery": 0,
		"listener": 30303
	},
	"listenAddr": "[::]:30303",
	"protocols": {
		"shh": {
			"maxMessageSize": 1048576,
			"minimumPoW": 0.001,
			"version": "5.0"
		}
	}
}

*/

type RpcRequest struct {
	Version string      `json:"jsonrpc"`
	Method  string      `json:"method"`
	Params  interface{} `json:"params"`
	Id      int         `json:"id"`
}

type RpcResponse struct {
	Version string      `json:"jsonrpc"`
	Result  interface{} `json:"result"`
	Error   rpcError    `json:"error"`
	Id      int         `json:"id"`
}
type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type port int

func (p port) String() string {
	return strconv.Itoa(int(p))
}

func (p port) Int() int {
	return int(p)
}

func getFreePort() port {
	l, _ := net.Listen("tcp", ":0")
	defer l.Close()

	addr := l.Addr().String()
	portStartIndex := strings.LastIndex(addr, ":")

	pNum, _ := strconv.Atoi(addr[portStartIndex+1:])

	return port(pNum)
}

func MakeRpcRequest(method string, params interface{}) RpcRequest {
	return RpcRequest{
		Version: "2.0",
		Id:      1,
		Method:  method,
		Params:  params,
	}
}

func mailServerParams(portNumber string) []string {
	return []string{"-bootstrap=true", "-forward=true", "-mailserver=true", "-httpport=" + portNumber, "-http=true", "-identity=../../static/keys/wnodekey", "-password=../../static/keys/wnodepassword", "-datadir=w2"}
}

func TestAliceSendMessageToBobWithSymkeyAndTopicAndBobReceiveThisMessage_Success(t *testing.T) {
	alice := newCLI()
	bob := newCLI()

	t.Log("Start nodes")
	startLocalNode(alice.Port())
	defer stopLocalNode()

	closeCh := make(chan struct{})
	doneFn := composeNodesClose(
		startNode("bob", STATUSD_BIN, closeCh, "-shh", "-httpport="+bob.PortString(), "-http=true", "-datadir=w1"),
	)
	time.Sleep(4 * time.Second)

	defer func() {
		close(closeCh)
		doneFn()
	}()

	t.Log("Create symkey and get alicesymkeyID")
	alicesymkeyID, err := alice.createSymkey()
	if err != nil {
		t.Fatal(err)
	}

	t.Log("Create topic")
	topic := whisperv5.BytesToTopic([]byte("some topic name"))

	t.Log("Get symkey by alicesymkeyID")
	symkey, err := alice.getSymkey(alicesymkeyID)
	if err != nil {
		t.Fatal(err)
	}

	t.Logf("alice send to bob symkey and topic: %v  -  %v\n", topic, symkey)

	t.Log("Get symkey to bob node and get bobsymkeyID")
	bobSymkeyID, err := bob.addSymkey(symkey)

	t.Log("Make alice filter for topic")
	aliceMsgFilterID, err := alice.makeMessageFilter(alicesymkeyID, topic.String())
	if err != nil {
		t.Fatal(err)
	}

	t.Log("Make bob filter for topic")
	bobMsgFilterID, err := bob.makeMessageFilter(bobSymkeyID, topic.String())
	if err != nil {
		t.Fatal(err)
	}

	t.Log("Alice send message to Bob")
	_, err = alice.postMessage(alicesymkeyID, topic.String(), 4, "")
	if err != nil {
		t.Fatal(err)
	}

	//wait message
	time.Sleep(1 * time.Second)

	t.Log("Bob get message")
	r, err := bob.getFilterMessages(bobMsgFilterID)
	if len(r.Result.([]interface{})) == 0 {
		t.Fatal("Hasnt got any messages")
	}
	t.Log(err, r)

	t.Log("Alice get message")
	r, err = alice.getFilterMessages(aliceMsgFilterID)
	if len(r.Result.([]interface{})) == 0 {
		t.Fatal("Hasnt got any messages")
	}

	t.Log(err, r)
}

func TestAliceAndBobP2PMessagingExample_Success(t *testing.T) {
	alice := newCLI()
	bob := newCLI()
	mailbox := newCLI()

	t.Log("Start nodes")
	startLocalNode(alice.Port())
	defer stopLocalNode()

	closeCh := make(chan struct{})
	doneFn := composeNodesClose(
		startNode("mailserver", WNODE_BIN, closeCh, mailServerParams(mailbox.PortString())...),
		startNode("bob", STATUSD_BIN, closeCh, "-shh", "-httpport="+bob.PortString(), "-http=true", "-datadir=w1"),
	)
	time.Sleep(4 * time.Second)

	defer func() {
		close(closeCh)
		doneFn()
	}()

	t.Log("Create symkey and get alicesymkeyID")
	alicesymkeyID, err := alice.createSymkey()
	if err != nil {
		t.Fatal(err)
	}

	t.Log("Create topic")
	topic := whisperv5.BytesToTopic([]byte("some topic TestAliceSendMessageToBobPeerWithSymkeyAndTopicAndBobReceiveThisMessage_AtMailboxNodeMessageDontExist_Success"))

	t.Log("Get symkey by alicesymkeyID")
	symkey, err := alice.getSymkey(alicesymkeyID)
	if err != nil {
		t.Fatal(err)
	}

	t.Logf("alice send to bob symkey and topic: %v  -  %v\n", topic, symkey)

	t.Log("Get symkey to bob node and get bobsymkeyID")
	bobSymkeyID, err := bob.addSymkey(symkey)
	if err != nil {
		t.Fatal(err)
	}

	t.Log("Make bob filter for topic")
	bobMsgFilterID, err := bob.makeMessageFilter(bobSymkeyID, topic.String())
	if err != nil {
		t.Fatal(err)
	}

	bobNodeInfo, err := bob.getNodeInfo()
	if err != nil {
		t.Fatal(err)
	}
	bobEnode := bobNodeInfo["enode"].(string)

	aliceNodeInfo, err := alice.getNodeInfo()
	if err != nil {
		t.Fatal(err)
	}
	aliceEnode := aliceNodeInfo["enode"].(string)

	t.Log("Adding to trusted peers")
	r, err := alice.adminAddPeer(bobEnode)
	t.Log(r, err)
	if err != nil {
		t.Fatal(err)
	}

	time.Sleep(time.Second)
	r, err = bob.markTrusted(aliceEnode)
	t.Log(r, err)
	if err != nil {
		t.Fatal(err)
	}

	t.Log("Alice send message to Bob")
	_, err = alice.postMessage(alicesymkeyID, topic.String(), 4, bobEnode)
	if err != nil {
		t.Fatal(err)
	}

	//wait message
	time.Sleep(time.Second)

	t.Log("Bob get message")
	r, err = bob.getFilterMessages(bobMsgFilterID)
	if len(r.Result.([]interface{})) == 0 {
		t.Fatal("Hasnt got any messages for bob")
	}
	t.Log(err, r)
}

func TestGetWhisperMessageMailServer_Symmetric(t *testing.T) {
	alice := newCLI()
	bob := newCLI()
	mailbox := newCLI()

	topic := whisperv5.BytesToTopic([]byte("TestGetWhisperMessageMailServer topic name"))

	t.Log("Start nodes")
	closeCh := make(chan struct{})
	doneFn := composeNodesClose(
		startNode("mailserver", WNODE_BIN, closeCh, mailServerParams(mailbox.PortString())...),
		startNode("alice", STATUSD_BIN, closeCh, "-shh", "-httpport="+alice.PortString(), "-http=true", "-datadir=w1"),
	)
	time.Sleep(4 * time.Second)
	defer func() {
		close(closeCh)
		doneFn()
	}()

	t.Log("Alice create aliceSymKey")
	time.Sleep(time.Millisecond)
	aliceSymkeyID, err := alice.createSymkey()
	if err != nil {
		t.Fatal(err)
	}

	t.Log("Alice send message to bob")
	_, err = alice.postMessage(aliceSymkeyID, topic.String(), 4, "")
	if err != nil {
		t.Fatal(err)
	}

	t.Log("Wait that Alice message is being expired")
	time.Sleep(10 * time.Second)

	t.Log("Start bob node")
	startLocalNode(bob.Port())
	defer stopLocalNode()
	time.Sleep(4 * time.Second)

	t.Log("Get alice symKey")
	aliceSymKey, err := alice.getSymkey(aliceSymkeyID)
	if err != nil {
		t.Fatal(err)
	}

	t.Log("Bob add aliceSymKey to his node")
	bobSymKeyID, err := bob.addSymkey(aliceSymKey)
	if err != nil {
		t.Fatal(err)
	}

	t.Log("Bob makes filter on his node")
	bobFilterID, err := bob.makeMessageFilter(bobSymKeyID, topic.String())
	if err != nil {
		t.Fatal(err)
	}

	time.Sleep(1 * time.Second)
	t.Log("Bob check messages. There are no messages")
	r, err := bob.getFilterMessages(bobFilterID)
	if len(r.Result.([]interface{})) != 0 {
		t.Fatal("Has got a messages")
	}
	t.Log(err, r)

	// prepare and send request to mail server for archive messages
	timeLow := uint32(time.Now().Add(-2 * time.Minute).Unix())
	timeUpp := uint32(time.Now().Add(2 * time.Minute).Unix())
	t.Log("Time:", timeLow, timeUpp)

	data := make([]byte, 8+whisperv5.TopicLength)
	binary.BigEndian.PutUint32(data, timeLow)
	binary.BigEndian.PutUint32(data[4:], timeUpp)
	copy(data[8:], topic[:])

	bobNode, err := backend.NodeManager().Node()
	if err != nil {
		t.Fatal(err)
	}

	mailServerPeerID, bobKeyFromPassword := bob.addMailServerNode(t, mailbox)

	var params whisperv5.MessageParams
	params.PoW = 1
	params.Payload = data
	params.KeySym = bobKeyFromPassword
	params.Src = bobNode.Server().PrivateKey
	params.WorkTime = 5

	msg, err := whisperv5.NewSentMessage(&params)
	if err != nil {
		t.Fatal(err)
	}
	env, err := msg.Wrap(&params)
	if err != nil {
		t.Fatal(err)
	}

	bobWhisper, _ := backend.NodeManager().WhisperService()
	err = bobWhisper.RequestHistoricMessages(mailServerPeerID, env)
	if err != nil {
		t.Fatal(err)
	}

	time.Sleep(time.Second)
	t.Log("Bob get alice message which sent from mailbox")
	r, err = bob.getFilterMessages(bobFilterID)
	t.Log(err, r)
	if len(r.Result.([]interface{})) == 0 {
		t.Fatal("Hasnt got any messages")
	}
}

func TestGetWhisperMessage_Asymmetric(t *testing.T) {
	alice := newCLI()
	bob := newCLI()

	topic := whisperv5.BytesToTopic([]byte("TestGetWhisperMessage_Asymmetric topic name"))

	t.Log("Start nodes")
	closeCh := make(chan struct{})
	doneFn := composeNodesClose(
		startNode("alice", STATUSD_BIN, closeCh, "-shh", "-httpport="+alice.PortString(), "-http=true", "-datadir=w1"),
	)
	t.Log("Start bob node")
	startLocalNode(bob.Port())

	time.Sleep(4 * time.Second)
	defer func() {
		close(closeCh)
		doneFn()
	}()

	t.Log("Alice create createAsymkey")
	time.Sleep(time.Millisecond)
	aliceAsymkeyID, err := alice.createAsymkey()
	if err != nil {
		t.Fatal(err)
	}
	_ = aliceAsymkeyID

	t.Log("Bob create createAsymkey")
	bobAsymkeyID, err := bob.createAsymkey()
	if err != nil {
		t.Fatal(err)
	}
	bobPubKey, err := bob.getPublicKey(bobAsymkeyID)
	if err != nil {
		t.Fatal(err)
	}

	t.Log("Bob sends to alice bob pubkey")

	t.Log("Alice send message to bob using bob pubkey")
	_, err = alice.postAsymMessage(bobPubKey, topic.String(), 4, "")
	if err != nil {
		t.Fatal(err)
	}

	t.Log("Bob makes filter on his node")
	bobFilterID, err := bob.makeAsyncMessageFilter(bobAsymkeyID, topic.String())
	if err != nil {
		t.Fatal(err)
	}

	time.Sleep(1 * time.Second)
	t.Log("Bob check messages. There are messages")
	r, err := bob.getFilterMessages(bobFilterID)
	if len(r.Result.([]interface{})) == 0 {
		t.Fatal("Has got a messages")
	}
}

func TestGetWhisperMessageMailServer_Asymmetric(t *testing.T) {
	alice := newCLI()
	bob := newCLI()
	mailbox := newCLI()

	topic := whisperv5.BytesToTopic([]byte("TestGetWhisperMessageMailServer topic name"))

	t.Log("Start nodes")
	closeCh := make(chan struct{})
	doneFn := composeNodesClose(
		startNode("mailserver", WNODE_BIN, closeCh, mailServerParams(mailbox.PortString())...),
		startNode("alice", STATUSD_BIN, closeCh, "-shh", "-httpport="+alice.PortString(), "-http=true", "-datadir=w1"),
	)

	t.Log("Start bob node")
	startLocalNode(bob.Port())
	time.Sleep(4 * time.Second)

	defer func() {
		close(closeCh)
		doneFn()
	}()

	t.Log("Alice create aliceKey")
	time.Sleep(time.Millisecond)
	_, err := alice.createAsymkey()
	if err != nil {
		t.Fatal(err)
	}

	t.Log("Bob creates key pair to his node")
	bobKeyID, err := bob.createAsymkey()
	if err != nil {
		t.Fatal(err)
	}

	bobPrivateKey, bobPublicKey, err := bob.getKeyPair(bobKeyID)
	if err != nil {
		t.Fatal(err)
	}

	// At this time nodes do public keys exchange

	// Bob goes offline
	err = stopLocalNode()
	if err != nil {
		t.Fatal(err)
	}
	t.Log("Bob has been stopped", backend)

	t.Log("Alice send message to bob")
	_, err = alice.postAsymMessage(bobPublicKey, topic.String(), 4, "")
	if err != nil {
		t.Fatal(err)
	}

	t.Log("Wait that Alice message is being expired")
	time.Sleep(3 * time.Second)

	t.Log("Resume bob node")
	bob = newCLI()
	startLocalNode(bob.Port())
	bob = cli{addr: "http://localhost:" + bob.PortString()}
	time.Sleep(4 * time.Second)
	defer stopLocalNode()

	t.Log("Is Bob`s node running", backend.NodeManager().IsNodeRunning())

	t.Log("Bob restores private key")
	_, err = bob.addPrivateKey(bobPrivateKey)
	if err != nil {
		t.Fatal(err)
	}

	t.Log("Bob makes filter on his node")
	bobFilterID, err := bob.makeAsyncMessageFilter(bobKeyID, topic.String())
	if err != nil {
		t.Fatal(err)
	}

	time.Sleep(1 * time.Second)
	t.Log("Bob check messages. There are no messages")
	r, err := bob.getFilterMessages(bobFilterID)
	if len(r.Result.([]interface{})) != 0 {
		t.Fatal("Has got a messages")
	}
	if err != nil {
		t.Fatal(err)
	}

	bobNode, err := backend.NodeManager().Node()
	if err != nil {
		t.Fatal(err)
	}

	mailServerPeerID, bobKeyFromPassword := bob.addMailServerNode(t, mailbox)

	// prepare and send request to mail server for archive messages
	timeLow := uint32(time.Now().Add(-2 * time.Minute).Unix())
	timeUpp := uint32(time.Now().Add(2 * time.Minute).Unix())
	t.Log("Time:", timeLow, timeUpp)

	data := make([]byte, 8+whisperv5.TopicLength)
	binary.BigEndian.PutUint32(data, timeLow)
	binary.BigEndian.PutUint32(data[4:], timeUpp)
	copy(data[8:], topic[:])

	var params whisperv5.MessageParams
	params.PoW = 1
	params.Payload = data
	params.KeySym = bobKeyFromPassword
	params.Src = bobNode.Server().PrivateKey
	params.WorkTime = 5

	msg, err := whisperv5.NewSentMessage(&params)
	if err != nil {
		t.Fatal(err)
	}
	env, err := msg.Wrap(&params)
	if err != nil {
		t.Fatal(err)
	}

	bobWhisper, _ := backend.NodeManager().WhisperService()
	err = bobWhisper.RequestHistoricMessages(mailServerPeerID, env)
	if err != nil {
		t.Fatal(err)
	}

	time.Sleep(time.Second)
	t.Log("Bob get alice message which sent from mailbox")
	r, err = bob.getFilterMessages(bobFilterID)
	t.Log(err, r)
	if len(r.Result.([]interface{})) == 0 {
		t.Fatal("Hasnt got any messages")
	}
}

func Test_StatusdClient_GetWhisperMessageMailServer_Asymmetric(t *testing.T) {
	alice := newCLI()
	bob := newCLI()
	mailbox := newCLI()

	topic := whisperv5.BytesToTopic([]byte("TestGetWhisperMessageMailServer topic name"))

	t.Log("Start nodes")
	closeCh := make(chan struct{})
	doneFn := composeNodesClose(
		startNode("mailserver", WNODE_BIN, closeCh, mailServerParams(mailbox.PortString())...),
		startNode("alice", STATUSD_BIN, closeCh, "-shh", "-httpport="+alice.PortString(), "-http=true", "-datadir=w1"),
	)
	t.Log("Start bob node")
	startLocalNode(bob.Port())
	time.Sleep(4 * time.Second)
	defer func() {
		close(closeCh)
		doneFn()
	}()

	t.Log("Alice create aliceKey")
	time.Sleep(time.Millisecond)
	_, err := alice.createAsymkey()
	if err != nil {
		t.Fatal(err)
	}

	t.Log("Bob creates key pair to his node")
	bobKeyID, err := bob.createAsymkey()
	if err != nil {
		t.Fatal(err)
	}

	bobPrivateKey, bobPublicKey, err := bob.getKeyPair(bobKeyID)
	if err != nil {
		t.Fatal(err)
	}

	// At this time nodes do public keys exchange

	// Bob goes offline
	err = stopLocalNode()
	if err != nil {
		t.Fatal(err)
	}
	t.Log("Bob has been stopped", backend)

	t.Log("Alice send message to bob")
	_, err = alice.postAsymMessage(bobPublicKey, topic.String(), 4, "")
	if err != nil {
		t.Fatal(err)
	}

	t.Log("Wait that Alice message is being expired")
	time.Sleep(10 * time.Second)

	t.Log("Resume bob node")
	bob = newCLI()
	startLocalNode(bob.Port())
	bob = cli{addr: "http://localhost:" + bob.PortString()}
	time.Sleep(4 * time.Second)
	defer stopLocalNode()

	t.Log("Bob restores private key")
	_, err = bob.addPrivateKey(bobPrivateKey)
	if err != nil {
		t.Fatal(err)
	}

	t.Log("Bob makes filter on his node")
	bobFilterID, err := bob.makeAsyncMessageFilter(bobKeyID, topic.String())
	if err != nil {
		t.Fatal(err)
	}

	time.Sleep(1 * time.Second)
	t.Log("Bob check messages. There are no messages")
	r, err := bob.getFilterMessages(bobFilterID)
	if len(r.Result.([]interface{})) != 0 {
		t.Fatal("Has got a messages")
	}
	if err != nil {
		t.Fatal(err)
	}

	bobNode, err := backend.NodeManager().Node()
	if err != nil {
		t.Fatal(err)
	}

	mailServerPeerID, bobKeyFromPassword := bob.addMailServerNode(t, mailbox)

	// prepare and send request to mail server for archive messages
	timeLow := uint32(time.Now().Add(-2 * time.Minute).Unix())
	timeUpp := uint32(time.Now().Add(2 * time.Minute).Unix())
	t.Log("Time:", timeLow, timeUpp)

	data := make([]byte, 8+whisperv5.TopicLength)
	binary.BigEndian.PutUint32(data, timeLow)
	binary.BigEndian.PutUint32(data[4:], timeUpp)
	copy(data[8:], topic[:])

	var params whisperv5.MessageParams
	params.PoW = 1
	params.Payload = data
	params.KeySym = bobKeyFromPassword
	params.Src = bobNode.Server().PrivateKey
	params.WorkTime = 5

	msg, err := whisperv5.NewSentMessage(&params)
	if err != nil {
		t.Fatal(err)
	}
	env, err := msg.Wrap(&params)
	if err != nil {
		t.Fatal(err)
	}

	bobWhisper, _ := backend.NodeManager().WhisperService()
	err = bobWhisper.RequestHistoricMessages(mailServerPeerID, env)
	if err != nil {
		t.Fatal(err)
	}

	time.Sleep(time.Second)
	t.Log("Bob get alice message which sent from mailbox")
	r, err = bob.getFilterMessages(bobFilterID)
	t.Log(err, r)

	results := r.Result.([]interface{})
	if len(results) == 0 {
		t.Fatal("Hasnt got any messages")
	}

	for _, res := range results {
		r := res.(map[string]interface{})

		payloadHex := r["payload"].(string)
		payload, err := hexutil.Decode(payloadHex)
		if err != nil {
			t.Fatal("Cant read payload", err)
		}

		//fixme add as assertion
		t.Log("Result", string(payload))
	}
}

func TestGetWhisperMessageMailServer_AllTopicMessages(t *testing.T) {
	alice := newCLI()
	bob := newCLI()
	mailbox := newCLI()

	topic := whisperv5.BytesToTopic([]byte("TestGetWhisperMessageMailServer topic name"))

	t.Log("Start nodes")
	closeCh := make(chan struct{})
	doneFn := composeNodesClose(
		startNode("mailserver", WNODE_BIN, closeCh, mailServerParams(mailbox.PortString())...),
		startNode("alice", STATUSD_BIN, closeCh, "-shh", "-httpport="+alice.PortString(), "-http=true", "-datadir=w1"),
	)
	time.Sleep(4 * time.Second)
	defer func() {
		close(closeCh)
		doneFn()
	}()

	t.Log("Alice create aliceSymKey")
	time.Sleep(time.Millisecond)
	aliceSymkeyID, err := alice.createSymkey()
	if err != nil {
		t.Fatal(err)
	}

	t.Log("Alice send message to bob")
	_, err = alice.postMessage(aliceSymkeyID, topic.String(), 4, "")
	if err != nil {
		t.Fatal(err)
	}

	t.Log("Wait that Alice message is being expired")
	time.Sleep(10 * time.Second)

	t.Log("Start bob node")
	startLocalNode(bob.Port())
	defer stopLocalNode()
	time.Sleep(4 * time.Second)

	t.Log("Get alice symKey")
	aliceSymKey, err := alice.getSymkey(aliceSymkeyID)
	if err != nil {
		t.Fatal(err)
	}

	t.Log("Bob add aliceSymKey to his node")
	bobSymKeyID, err := bob.addSymkey(aliceSymKey)
	if err != nil {
		t.Fatal(err)
	}

	t.Log("Bob sends message to alice")
	_, err = bob.postMessage(bobSymKeyID, topic.String(), 4, "")
	if err != nil {
		t.Fatal(err)
	}

	t.Log("Bob makes filter on his node")
	bobFilterID, err := bob.makeMessageFilter(bobSymKeyID, topic.String())
	if err != nil {
		t.Fatal(err)
	}

	time.Sleep(1 * time.Second)

	// prepare and send request to mail server for archive messages
	timeLow := uint32(time.Now().Add(-2 * time.Minute).Unix())
	timeUpp := uint32(time.Now().Add(2 * time.Minute).Unix())
	t.Log("Time:", timeLow, timeUpp)

	data := make([]byte, 8+whisperv5.TopicLength)
	binary.BigEndian.PutUint32(data, timeLow)
	binary.BigEndian.PutUint32(data[4:], timeUpp)
	copy(data[8:], topic[:])

	bobNode, err := backend.NodeManager().Node()
	if err != nil {
		t.Fatal(err)
	}

	mailServerPeerID, bobKeyFromPassword := bob.addMailServerNode(t, mailbox)

	var params whisperv5.MessageParams
	params.PoW = 1
	params.Payload = data
	params.KeySym = bobKeyFromPassword
	params.Src = bobNode.Server().PrivateKey
	params.WorkTime = 5

	msg, err := whisperv5.NewSentMessage(&params)
	if err != nil {
		t.Fatal(err)
	}
	env, err := msg.Wrap(&params)
	if err != nil {
		t.Fatal(err)
	}

	bobWhisper, _ := backend.NodeManager().WhisperService()
	err = bobWhisper.RequestHistoricMessages(mailServerPeerID, env)
	if err != nil {
		t.Fatal(err)
	}

	time.Sleep(time.Second)
	t.Log("Bob get alice message which sent from mailbox")
	r, err := bob.getFilterMessages(bobFilterID)
	t.Log(err, r)
	if len(r.Result.([]interface{})) != 2 {
		t.Fatal("Hasnt got messages from Alice and Bob both")
	}

	results := r.Result.([]interface{})
	if len(results) == 0 {
		t.Fatal("Hasnt got any messages")
	}

	for _, res := range results {
		r := res.(map[string]interface{})

		payloadHex := r["payload"].(string)
		payload, err := hexutil.Decode(payloadHex)
		if err != nil {
			t.Fatal("Cant read payload", err)
		}

		//fixme add as assertion
		t.Log("Result", string(payload))
	}
}

func TestAliceSendsMessageAndMessageExistsOnMailserverNode(t *testing.T) {
	alice := newCLI()
	mailbox := newCLI()

	t.Log("Create topic")
	topic := whisperv5.BytesToTopic([]byte("TestAliceSendsMessageAndMessageExistsOnMailserverNode topic "))

	t.Log("Start nodes")
	closeCh := make(chan struct{})
	doneFn := composeNodesClose(
		startNode("mailserver", WNODE_BIN, closeCh, mailServerParams(mailbox.PortString())...),
		startNode("alice", STATUSD_BIN, closeCh, "-shh", "-httpport="+alice.PortString(), "-http=true", "-datadir=w1"),
	)
	time.Sleep(4 * time.Second)
	defer func() {
		close(closeCh)
		doneFn()
	}()

	t.Log("Alice create symkey")
	symkeyID1, err := alice.createSymkey()
	if err != nil {
		t.Fatal(err)
	}

	t.Log("Alice send message to topic")
	_, err = alice.postMessage(symkeyID1, topic.String(), 4, "")
	if err != nil {
		t.Fatal(err)
	}

	t.Log("Alice get symkey by symkeyID")
	symkey, err := alice.getSymkey(symkeyID1)
	if err != nil {
		t.Fatal(err)
	}

	t.Log("Alice add symkey to mailserver")
	symkeyIDMailserver, err := mailbox.addSymkey(symkey)
	if err != nil {
		t.Fatal(err)
	}

	t.Log("Alice make filter on mailserver")
	msgFilterID2, err := mailbox.makeMessageFilter(symkeyIDMailserver, topic.String())
	if err != nil {
		t.Fatal(err)
	}

	time.Sleep(time.Second)
	t.Log("Check messages by filter")
	r, err := mailbox.getFilterMessages(msgFilterID2)
	if len(r.Result.([]interface{})) == 0 {
		t.Fatal("Hasn't got a messages")
	}
}

type cli struct {
	addr string
	p    port
	c    http.Client
}

func newCLI(addr ...string) cli {
	if len(addr) == 0 {
		addr = []string{"http://localhost:"}
	}

	p := getFreePort()

	return cli{addr: addr[0] + p.String(), p: p}
}

func (c cli) Port() int {
	return c.p.Int()
}

func (c cli) PortString() string {
	return c.p.String()
}

//create sym key
func (c cli) createSymkey() (string, error) {
	r, err := makeBody(MakeRpcRequest("shh_newSymKey", nil))
	if err != nil {
		return "", err
	}
	resp, err := c.c.Post(c.addr, "application/json", r)
	if err != nil {
		return "", err
	}
	rsp, err := makeRpcResponse(resp.Body)
	if err != nil {
		return "", err
	}
	return rsp.Result.(string), nil
}

//create asym key
func (c cli) createAsymkey() (string, error) {
	r, err := makeBody(MakeRpcRequest("shh_newKeyPair", nil))
	if err != nil {
		return "", err
	}
	resp, err := c.c.Post(c.addr, "application/json", r)
	if err != nil {
		return "", err
	}
	rsp, err := makeRpcResponse(resp.Body)
	if err != nil {
		return "", err
	}
	return rsp.Result.(string), nil
}

//get public key
func (c cli) getKeyPair(keyID string) (string, string, error) {
	pk, err := c.getPrivateKey(keyID)
	if err != nil {
		return "", "", err
	}

	pubk, err := c.getPublicKey(keyID)
	if err != nil {
		return "", "", err
	}

	return pk, pubk, nil
}

//get private key
func (c cli) getPrivateKey(keyID string) (string, error) {
	r, err := makeBody(MakeRpcRequest("shh_getPrivateKey", []string{keyID}))
	if err != nil {
		return "", err
	}
	resp, err := c.c.Post(c.addr, "application/json", r)
	if err != nil {
		return "", err
	}
	rsp, err := makeRpcResponse(resp.Body)
	if err != nil {
		return "", err
	}
	return rsp.Result.(string), nil
}

//get public key
func (c cli) getPublicKey(keyID string) (string, error) {
	r, err := makeBody(MakeRpcRequest("shh_getPublicKey", []string{keyID}))
	if err != nil {
		return "", err
	}
	resp, err := c.c.Post(c.addr, "application/json", r)
	if err != nil {
		return "", err
	}
	rsp, err := makeRpcResponse(resp.Body)
	if err != nil {
		return "", err
	}
	return rsp.Result.(string), nil
}

//generate sym key
func (c cli) generateSymkeyFromPassword(password string) (string, error) {
	r, err := makeBody(MakeRpcRequest("shh_generateSymKeyFromPassword", []string{password}))
	if err != nil {
		return "", err
	}
	resp, err := c.c.Post(c.addr, "application/json", r)
	if err != nil {
		return "", err
	}
	rsp, err := makeRpcResponse(resp.Body)
	if err != nil {
		return "", err
	}
	return rsp.Result.(string), nil
}

//get sym key
func (c cli) getSymkey(s string) (string, error) {
	r, err := makeBody(MakeRpcRequest("shh_getSymKey", []string{s}))
	if err != nil {
		return "", err
	}
	resp, err := c.c.Post(c.addr, "application/json", r)
	if err != nil {
		return "", err
	}
	rsp, err := makeRpcResponse(resp.Body)
	if err != nil {
		return "", err
	}
	return rsp.Result.(string), nil
}

//create sym key
//curl -X POST --data '{"jsonrpc":"2.0","method":"shh_addSymKey","params":["0xf6dcf21ed6a17bd78d8c4c63195ab997b3b65ea683705501eae82d32667adc92"],"id":1}'
func (c cli) addSymkey(s string) (string, error) {
	r, err := makeBody(MakeRpcRequest("shh_addSymKey", []string{s}))
	if err != nil {
		return "", err
	}
	resp, err := c.c.Post(c.addr, "application/json", r)
	if err != nil {
		return "", err
	}
	rsp, err := makeRpcResponse(resp.Body)
	if err != nil {
		return "", err
	}
	return rsp.Result.(string), nil
}

// add private key
func (c cli) addPrivateKey(s string) (string, error) {
	r, err := makeBody(MakeRpcRequest("shh_addPrivateKey", []string{s}))
	if err != nil {
		return "", err
	}
	resp, err := c.c.Post(c.addr, "application/json", r)
	if err != nil {
		return "", err
	}
	rsp, err := makeRpcResponse(resp.Body)
	if err != nil {
		return "", err
	}
	return rsp.Result.(string), nil
}

//post whisper message
func (c cli) postMessage(symKeyID string, topic string, ttl int, targetPeer string) (RpcResponse, error) {
	r, err := makeBody(MakeRpcRequest("shh_post", []shhPost{{
		SymKeyId:   symKeyID,
		Topic:      topic,
		Payload:    hexutil.Encode([]byte("hello in symmetric style!")),
		PowTarget:  0.001,
		PowTime:    1,
		TTL:        ttl,
		TargetPeer: targetPeer,
	}}))
	if err != nil {
		return RpcResponse{}, err
	}

	resp, err := c.c.Post(c.addr, "application/json", r)
	if err != nil {
		return RpcResponse{}, err
	}
	return makeRpcResponse(resp.Body)
}

//post wisper message with asymmetric encryption
func (c cli) postAsymMessage(pubKey, topic string, ttl int, targetPeer string) (RpcResponse, error) {
	r, err := makeBody(MakeRpcRequest("shh_post", []shhPost{{
		PubKey:     pubKey,
		Topic:      topic,
		Payload:    hexutil.Encode([]byte("hello world!!")),
		PowTarget:  0.001,
		PowTime:    1,
		TTL:        119,
		TargetPeer: targetPeer,
	}}))
	if err != nil {
		return RpcResponse{}, err
	}

	resp, err := c.c.Post(c.addr, "application/json", r)
	if err != nil {
		return RpcResponse{}, err
	}
	return makeRpcResponse(resp.Body)
}

func (c cli) makeMessageFilter(symKeyID string, topic string) (string, error) {
	//make filter
	r, err := makeBody(MakeRpcRequest("shh_newMessageFilter", []shhNewMessageFilter{{
		SymKeyId: symKeyID,
		Topics:   []string{topic},
		AllowP2P: true,
	}}))
	if err != nil {
		return "", err
	}

	resp, err := c.c.Post(c.addr, "application/json", r)
	if err != nil {
		return "", err
	}
	rsp, err := makeRpcResponse(resp.Body)
	if err != nil {
		return "", err
	}

	return rsp.Result.(string), nil
}

func (c cli) makeAsyncMessageFilter(privateKeyID string, topic string) (string, error) {
	//make filter
	r, err := makeBody(MakeRpcRequest("shh_newMessageFilter", []shhNewMessageFilter{{
		PrivateKeyID: privateKeyID,
		Topics:       []string{topic},
		AllowP2P:     true,
	}}))
	if err != nil {
		return "", err
	}

	resp, err := c.c.Post(c.addr, "application/json", r)
	if err != nil {
		return "", err
	}

	rsp, err := makeRpcResponse(resp.Body)
	if err != nil {
		return "", err
	}

	if rsp.Error.Message != "" {
		return "", errors.New(rsp.Error.Message)
	}

	return rsp.Result.(string), nil
}

func (c cli) getFilterMessages(msgFilterID string) (RpcResponse, error) {
	r, err := makeBody(MakeRpcRequest("shh_getFilterMessages", []string{msgFilterID}))
	if err != nil {
		return RpcResponse{}, err
	}

	resp, err := c.c.Post(c.addr, "application/json", r)
	if err != nil {
		return RpcResponse{}, err
	}
	return makeRpcResponse(resp.Body)
}

func (c cli) getNodeInfo() (map[string]interface{}, error) {
	r, err := makeBody(MakeRpcRequest("admin_nodeInfo", []string{}))
	if err != nil {
		return nil, err
	}

	resp, err := c.c.Post(c.addr, "application/json", r)
	if err != nil {
		return nil, err
	}
	rsp, err := makeRpcResponse(resp.Body)
	if err != nil {
		return nil, err
	}
	return rsp.Result.(map[string]interface{}), nil
}
func (c cli) adminAddPeer(enode string) (RpcResponse, error) {
	r, err := makeBody(MakeRpcRequest("admin_addPeer", []string{enode}))
	if err != nil {
		return RpcResponse{}, err
	}

	resp, err := c.c.Post(c.addr, "application/json", r)
	if err != nil {
		return RpcResponse{}, err
	}
	rsp, err := makeRpcResponse(resp.Body)
	if err != nil {
		return RpcResponse{}, err
	}
	return rsp, nil
}

//curl -X POST --data '{"jsonrpc":"2.0","method":"shh_markTrustedPeer","params":["enode://d25474361659861e9e651bc728a17e807a3359ca0d344afd544ed0f11a31faecaf4d74b55db53c6670fd624f08d5c79adfc8da5dd4a11b9213db49a3b750845e@52.178.209.125:30379"],"id":1}'
func (c cli) markTrusted(enode string) (RpcResponse, error) {
	r, err := makeBody(MakeRpcRequest("shh_markTrustedPeer", []string{enode}))
	if err != nil {
		return RpcResponse{}, err
	}

	resp, err := c.c.Post(c.addr, "application/json", r)
	if err != nil {
		return RpcResponse{}, err
	}
	rsp, err := makeRpcResponse(resp.Body)
	if err != nil {
		return RpcResponse{}, err
	}
	return rsp, nil
}

func (c cli) addMailServerNode(t *testing.T, nMail cli) (mailServerPeerID, bobKeyFromPassword []byte) {
	mNodeInfo, err := nMail.getNodeInfo()
	if err != nil {
		t.Fatal(err)
	}
	mailServerEnode := mNodeInfo["enode"].(string)

	t.Log("Add mailserver peer to bob node")
	err = backend.NodeManager().AddPeer(mailServerEnode)
	if err != nil {
		t.Fatal(err)
	}

	t.Log("Add mailserver peer to bob node too??")
	time.Sleep(5 * time.Second)

	t.Log("Mark mailserver as bob trusted")
	rsp, err := c.markTrusted(mailServerEnode)
	t.Log(rsp, err)

	t.Log("extractIdFromEnode")
	mailServerPeerID, err = extractIdFromEnode(mailServerEnode)
	if err != nil {
		t.Fatal(err)
	}

	t.Log("Get bob's symkey for mailserver")
	bobWhisper, _ := backend.NodeManager().WhisperService()
	keyID, err := bobWhisper.AddSymKeyFromPassword("asdfasdf") // mailserver password
	if err != nil {
		t.Fatalf("Failed to create symmetric key for mail request: %s", err)
	}
	t.Log("Add symkey by id")
	bobKeyFromPassword, err = bobWhisper.GetSymKey(keyID)
	if err != nil {
		t.Fatalf("Failed to save symmetric key for mail request: %s", err)
	}

	return mailServerPeerID, bobKeyFromPassword
}

func makeBody(r RpcRequest) (io.Reader, error) {
	b, err := json.Marshal(r)
	if err != nil {
		return nil, err
	}
	return bytes.NewReader(b), nil
}

func makeRpcResponse(r io.Reader) (RpcResponse, error) {
	rsp := RpcResponse{}
	err := json.NewDecoder(r).Decode(&rsp)

	if rsp.Error.Message != "" {
		return rsp, errors.New(rsp.Error.Message)
	}
	return rsp, err
}

func startLocalNode(port int) {
	args := os.Args
	defer func() {
		os.Args = args
	}()

	os.Args = append(args, []string{"-httpport=" + strconv.Itoa(port), "-http=true", "-bootstrap=false"}...)
	go main()
	time.Sleep(time.Second)
}

func composeNodesClose(doneFns ...func()) func() {
	return func() {
		for _, doneF := range doneFns {
			doneF()
		}
	}
}

func startNode(name string, binary string, closeCh chan struct{}, args ...string) (doneFn func()) {
	cmd := exec.Command(binary, args...)
	cmd.Dir = getRootDir() + "/" + "../../build/bin/"
	fmt.Println(cmd)

	f, err := os.Create(getRootDir() + "/" + name + ".txt")
	if err != nil {
		fmt.Println(err)
	}

	cmd.Stderr = f
	cmd.Stdout = f
	err = cmd.Start()
	if err != nil {
		fmt.Println(err)
	}

	done := make(chan struct{})
	go func() {
		<-closeCh

		// kill magic
		if err = cmd.Process.Kill(); err != nil {
			fmt.Println(err)
		}

		exitStatus, err := cmd.Process.Wait()
		if err != nil {
			fmt.Println(err)
		}

		fmt.Println("Killed", exitStatus.String(), args)
		defer f.Close()
		close(done)
	}()

	time.Sleep(4 * time.Second)
	doneFn = func() {
		fmt.Println("finishing...")
		<-done
	}

	return doneFn
}

func getRootDir() string {
	_, f, _, _ := runtime.Caller(0)
	return path.Dir(f)
}

func stopLocalNode() error {
	if backend == nil {
		return nil
	}

	backCh, err := backend.StopNode()
	if err != nil {
		return err
	}
	<-backCh
	backend = nil

	return nil
}
