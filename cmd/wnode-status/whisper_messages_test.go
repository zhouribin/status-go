package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/ethereum/go-ethereum/p2p/discover"
	"github.com/ethereum/go-ethereum/whisper/whisperv5"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path"
	"runtime"
	"strconv"
	"testing"
	"time"
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
	SymKeyId  string  `json:"symKeyID"`
	Topic     string  `json:"topic"`
	Payload   string  `json:"payload"`
	PowTarget float32 `json:"powTarget"`
	PowTime   int     `json:"powTime"`
	TTL       int     `json:"TTL"`
}
type shhNewMessageFilter struct {
	SymKeyId string   `json:"symKeyID"`
	Topics   []string `json:"topics"`
	AllowP2P bool     `json:"allowP2P"`
}

type RpcRequest struct {
	Version string      `json:"jsonrpc"`
	Method  string      `json:"method"`
	Params  interface{} `json:"params"`
	Id      int         `json:"id"`
}

type RpcResponse struct {
	Version string      `json:"jsonrpc"`
	Result  interface{} `json:"result"`
	Error   rpcError    `json:"params"`
	Id      int         `json:"id"`
}
type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func MakeRpcRequest(method string, params interface{}) RpcRequest {
	return RpcRequest{
		Version: "2.0",
		Id:      1,
		Method:  method,
		Params:  params,
	}
}

func TestAliceSendMessageToBobWithSymkeyAndTopicAndBobReceiveThisMessage_Success(t *testing.T) {
	alice := Cli{addr: "http://localhost:8536"}
	bob := Cli{addr: "http://localhost:8537"}

	t.Log("Start nodes")
	startLocalNode(8536)

	closeCh := make(chan struct{})
	doneFn := composeNodesClose(
		startNode(closeCh, "-httpport=8537", "-http=true", "-datadir=w1"),
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

	t.Log("alice send to bob symkey and topic: %v  -  %v", topic, symkey)

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
	_, err = alice.postMessage(alicesymkeyID, topic.String(), 4)
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

func TestGetWhisperMessageMailServer(t *testing.T) {
	senderNode := Cli{addr: "http://localhost:8537"}
	receiverNode := Cli{addr: "http://localhost:8536"}
	nMail := Cli{addr: "http://localhost:8538"}
	_ = nMail
	topic := "0xe00123a5"

	t.Log("Start nodes")
	closeCh := make(chan struct{})
	doneFn := composeNodesClose(
		startNode(closeCh, "-httpport=8538", "-http=true", "-mailserver=true", "-identity=../../static/keys/wnodekey", "-password=../../static/keys/wnodepassword", "-datadir=w2"),
		startNode(closeCh, "-httpport=8537", "-http=true", "-datadir=w1"),
	)
	time.Sleep(4 * time.Second)
	defer func() {
		close(closeCh)
		doneFn()
	}()

	t.Log("Create symkeyID1")
	symkeyID1, err := senderNode.createSymkey()
	if err != nil {
		t.Fatal(err)
	}

	t.Log("post message to 1")
	_, err = senderNode.postMessage(symkeyID1, topic, 4)
	if err != nil {
		t.Fatal(err)
	}

	time.Sleep(10 * time.Second)
	// start receiver node
	startLocalNode(8536)
	time.Sleep(4 * time.Second)

	symkey, err := senderNode.getSymkey(symkeyID1)
	if err != nil {
		t.Fatal(err)
	}

	symkeyID2, err := receiverNode.addSymkey(symkey)
	if err != nil {
		t.Fatal(err)
	}

	t.Log("Make filter")
	msgFilterID2, err := receiverNode.makeMessageFilter(symkeyID2, topic)
	if err != nil {
		t.Fatal(err)
	}

	time.Sleep(1 * time.Second)
	t.Log("get message 1 from 2")
	r, err := receiverNode.getFilterMessages(msgFilterID2)
	if len(r.Result.([]interface{})) != 0 {
		t.Fatal("Has got a messages")
	}
	t.Log(err, r)

	w, _ := backend.NodeManager().WhisperService()
	mailServerEnode := "enode://7ef1407cccd16c90d01bfd8245b4b93c2f78e7d19769dc310cf46628d614d8aa7259005ef532d426092fa14ef0010ff7d83d5bfd108614d447b0b07499ffda78@127.0.0.1:30303"

	err = backend.NodeManager().AddPeer(mailServerEnode)
	if err != nil {
		t.Fatal(err)
	}

	n, err := backend.NodeManager().Node()
	if err != nil {
		t.Fatal(err)
	}

	mailNode, err := discover.ParseNode(mailServerEnode)
	if err != nil {
		t.Fatal(err)
	}
	n.Server().AddPeer(mailNode)

	time.Sleep(5 * time.Second)
	go func() {
		err = requestExpiredMessagesLoop(w, topic[2:], mailServerEnode, "asdfasdf",
			0, uint32(time.Now().Add(2*time.Minute).Unix()), closeCh)

		if err != nil {
			t.Fatal("error in requestExpiredMessagesLoop", err)
		}
	}()

	time.Sleep(60 * time.Second)

	t.Log("get message 1 from 2")
	r, err = receiverNode.getFilterMessages(msgFilterID2)
	if len(r.Result.([]interface{})) == 0 {
		t.Fatal("Hasnt got any messages")
	}
	t.Log(err, r)

}
func TestAliceSendsMessageAndMessageExistsOnMailserverNode(t *testing.T) {
	alice := Cli{addr: "http://localhost:8537"}
	nMail := Cli{addr: "http://localhost:8538"}

	t.Log("Create topic")
	topic := whisperv5.BytesToTopic([]byte("TestAliceSendsMessageAndMessageExistsOnMailserverNode topic "))

	t.Log("Start nodes")
	closeCh := make(chan struct{})
	doneFn := composeNodesClose(
		startNode(closeCh, "-httpport=8538", "-http=true", "-mailserver=true", "-identity=../../static/keys/wnodekey", "-password=../../static/keys/wnodepassword", "-datadir=w2"),
		startNode(closeCh, "-httpport=8537", "-http=true", "-datadir=w1"),
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
	_, err = alice.postMessage(symkeyID1, topic.String(), 4)
	if err != nil {
		t.Fatal(err)
	}

	t.Log("Alice get symkey by symkeyID")
	symkey, err := alice.getSymkey(symkeyID1)
	if err != nil {
		t.Fatal(err)
	}

	t.Log("Alice add symkey to mailserver")
	symkeyIDMailserver, err := nMail.addSymkey(symkey)
	if err != nil {
		t.Fatal(err)
	}

	t.Log("Alice make filter on mailserver")
	msgFilterID2, err := nMail.makeMessageFilter(symkeyIDMailserver, topic.String())
	if err != nil {
		t.Fatal(err)
	}

	time.Sleep(time.Second)
	t.Log("Check messages by filter")
	r, err := nMail.getFilterMessages(msgFilterID2)
	if len(r.Result.([]interface{})) == 0 {
		t.Fatal("Hasn't got a messages")
	}
}

type Cli struct {
	addr string
	c    http.Client
}

//create sym key
func (c Cli) createSymkey() (string, error) {
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

//get sym key
func (c Cli) getSymkey(s string) (string, error) {
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
func (c Cli) addSymkey(s string) (string, error) {
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

//post wisper message
func (c Cli) postMessage(symKeyID string, topic string, ttl int) (RpcResponse, error) {
	r, err := makeBody(MakeRpcRequest("shh_post", []shhPost{{
		SymKeyId:  symKeyID,
		Topic:     topic,
		Payload:   "0x73656e74206265666f72652066696c7465722077617320616374697665202873796d6d657472696329",
		PowTarget: 0.001,
		PowTime:   1,
		TTL:       ttl,
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

func (c Cli) makeMessageFilter(symKeyID string, topic string) (string, error) {
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

func (c Cli) getFilterMessages(msgFilterID string) (RpcResponse, error) {
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

func startNode(closeCh chan struct{}, args ...string) (doneFn func()) {
	cmd := exec.Command("./wnode-status", args...)
	cmd.Dir = getRootDir() + "/../../build/bin/"
	fmt.Println(cmd)

	cmd.Stderr = os.Stdout
	cmd.Stdout = os.Stdout
	err := cmd.Start()
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
