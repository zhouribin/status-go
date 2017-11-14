package main

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"os"
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

var c http.Client

func TestGetWisperMessage(t *testing.T) {
	startLocalWhisperNode()
	symkey, err := createSymkey()
	if err != nil {
		t.Fatal(err)
	}

	msgFilterID, err := makeMessageFilter(symkey)
	if err != nil {
		t.Fatal(err)
	}

	_, err = postMessage(symkey)
	if err != nil {
		t.Fatal(err)
	}

	//make get message
	t.Log("get message 1")
	r, err := getFilterMessages(msgFilterID)
	t.Log(err, r)

	_, err = postMessage(symkey)
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(3 * time.Second)
	t.Log("get message 2")
	r, err = getFilterMessages(msgFilterID)
	t.Log(err, r)

}

//create sym key
func createSymkey() (string, error) {
	r, err := makeBody(MakeRpcRequest("shh_newSymKey", nil))
	if err != nil {
		return "", err
	}
	resp, err := c.Post("http://localhost:8536", "application/json", r)
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
func postMessage(symkey string) (RpcResponse, error) {
	r, err := makeBody(MakeRpcRequest("shh_post", []shhPost{{
		SymKeyId:  symkey,
		Topic:     "0xe00123a5",
		Payload:   "0x73656e74206265666f72652066696c7465722077617320616374697665202873796d6d657472696329",
		PowTarget: 0.001,
		PowTime:   2,
		TTL:       2,
	}}))
	if err != nil {
		return RpcResponse{}, err
	}

	resp, err := c.Post("http://localhost:8536", "application/json", r)
	if err != nil {
		return RpcResponse{}, err
	}
	return makeRpcResponse(resp.Body)
}

func makeMessageFilter(symkey string) (string, error) {
	//make filter
	r, err := makeBody(MakeRpcRequest("shh_newMessageFilter", []shhNewMessageFilter{{
		SymKeyId: symkey,
		Topics:   []string{"0xe00123a5"},
	}}))
	if err != nil {
		return "", err
	}

	resp, err := c.Post("http://localhost:8536", "application/json", r)
	if err != nil {
		return "", err
	}
	rsp, err := makeRpcResponse(resp.Body)
	if err != nil {
		return "", err
	}

	return rsp.Result.(string), nil
}

func getFilterMessages(msgFilterID string) (RpcResponse, error) {
	r, err := makeBody(MakeRpcRequest("shh_getFilterMessages", []string{msgFilterID}))
	if err != nil {
		return RpcResponse{}, err
	}

	resp, err := c.Post("http://localhost:8536", "application/json", r)
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

func startLocalWhisperNode() {
	args := os.Args
	os.Args = append(args, []string{"-mailserver=true", "-identity=../../static/keys/wnodekey", "-password=../../static/keys/wnodepassword", "-httpport=8536", "-http=true"}...)
	go main()
	time.Sleep(3 * time.Second)
}
