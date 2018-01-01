package whisper

import (
	"bytes"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"encoding/json"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/whisper/whisperv5"
	"github.com/status-im/status-go/e2e"
	"github.com/status-im/status-go/geth/api"
	"github.com/status-im/status-go/geth/rpc"
	. "github.com/status-im/status-go/testing"
)

const (
	uniqueTopicsName   = "uniqueTopics"
	sendingCirclesName = "sendingCount"
)

func BenchmarkWhisperReceiveSingleTopicNoMatch(b *testing.B) {
	benchWhisperReceiveTopics(b, 1, "test unique topic")
}

func BenchmarkWhisperReceiveManyTopicsNoMatch(b *testing.B) {
	benchWhisperReceiveTopics(b, 100, "test unique topic")
}

func BenchmarkWhisperReceiveSingleTopicMatch(b *testing.B) {
	topic := strconv.Itoa(0) + " test"
	benchWhisperReceiveTopics(b, 1, topic)
}

func BenchmarkWhisperReceiveManyTopicsMatch(b *testing.B) {
	topic := strconv.Itoa(0) + " test"
	benchWhisperReceiveTopics(b, 100, topic)
}

func benchWhisperReceiveTopics(b *testing.B, topicsCount int, topicStr string) {
	defer func() {
		log.Println("!!!!!!!! DONE benchWhisperReceiveTopics\n\n")
	}()

	receiver, stop := startBackend("receiver")
	defer stop()

	os.Setenv(uniqueTopicsName, strconv.Itoa(topicsCount))
	os.Setenv(sendingCirclesName, strconv.Itoa(30000))
	defer os.Unsetenv(uniqueTopicsName)
	defer os.Unsetenv(sendingCirclesName)

	currentFile, err := os.Executable()
	if err != nil {
		panic(err)
	}
	currentDir := filepath.Dir(currentFile)

	// run sending messages in background
	senders := exec.Command("go",
		"test", currentDir+"/e2e/whisper/whisper_send_message_test.go",
		"-bench", "BenchmarkWhisperSendMessagesWithDifferentTopics",
		"-benchtime", "300s",
	)
	senders.Dir = currentDir

	if err := senders.Start(); err != nil {
		b.Fatal(err)
	}
	defer func() {
		if err := senders.Process.Kill(); err != nil {
			b.Fatal(err)
		}

		if err := senders.Wait(); err != nil {
			b.Fatal(err)
		}
	}()
	time.Sleep(15 * time.Second)

	mailServerEnode := getMailServerEnode(b)

	err = receiver.NodeManager().AddPeer(mailServerEnode)
	if err != nil {
		b.Fatal(err)
	}
	time.Sleep(time.Second)

	topicS := whisperv5.BytesToTopic([]byte(topicStr))
	topic := topicS.String()
	w, err := receiver.NodeManager().WhisperService()
	if err != nil {
		b.Fatal(err)
	}

	keyID, err := w.NewKeyPair()
	if err != nil {
		b.Fatal(err)
	}

	pk, err := w.GetPrivateKey(keyID)
	if err != nil {
		b.Fatal(err)
	}

	pubkey := hexutil.Bytes(crypto.FromECDSAPub(&pk.PublicKey)).String()

	n, _ := receiver.NodeManager().Node()
	b.Log(n.Server().PeersInfo())

	log.Println("Before recieving:", len(w.Envelopes()))

	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()
	s := createPrivateChatMessageFilter(receiver.NodeManager().RPCClient(), pubkey, topic)
	b.Log(s)

	filterResult := &successfulResponce{}
	json.Unmarshal([]byte(s), filterResult)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		<-ticker.C
		if i == 0 {
			log.Printf("While recieving: envelops %v; messages %v\n", len(w.Envelopes()), len(w.Messages(filterResult.Result)))
		}
	}
	b.StopTimer()

	log.Println("After recieving:", len(w.Envelopes()))
}

func BenchmarkWhisperSendMessagesOneTopic(b *testing.B) {
	mailbox, stop := startMailboxBackend()
	defer stop()
	node, err := mailbox.NodeManager().Node()
	if err != nil {
		b.Fatal(err)
	}

	enode := node.Server().NodeInfo().Enode
	err = ioutil.WriteFile("./enode.txt", []byte(enode), os.ModePerm)
	if err != nil {
		b.Fatal(err)
	}
	defer os.Remove("./enode.txt")

	const backendCount = 5
	backends := make([]*api.StatusBackend, backendCount)
	for i := 0; i < backendCount; i++ {
		bb, cl := startBackend("backend" + strconv.Itoa(i))
		backends[i] = bb
		err := backends[i].NodeManager().AddPeer(enode)
		if err != nil {
			b.Fatal(err)
		}
		defer cl()
	}
	time.Sleep(time.Second)

	uniqueTopicsStr := os.Getenv(uniqueTopicsName)
	var uniqueTopics int

	uniqueTopics, err = strconv.Atoi(uniqueTopicsStr)
	if err != nil {
		uniqueTopics = 100
	}

	sendMessages(backends, uniqueTopics, b)
}

func BenchmarkWhisperSendMessagesWithDifferentTopics(b *testing.B) {
	mailbox, stop := startMailboxBackend()
	defer stop()

	node, _ := mailbox.NodeManager().Node()
	enode := node.Server().NodeInfo().Enode
	err := ioutil.WriteFile("./enode.txt", []byte(enode), os.ModePerm)
	if err != nil {
		b.Fatal(err)
	}
	defer os.Remove("./enode.txt")

	const backendCount = 5
	backends := make([]*api.StatusBackend, backendCount)
	for i := 0; i < backendCount; i++ {
		bb, cl := startBackend("backend" + strconv.Itoa(i))
		backends[i] = bb
		err := backends[i].NodeManager().AddPeer(enode)
		if err != nil {
			b.Fatal(err)
		}
		defer cl()
	}
	time.Sleep(time.Second)

	uniqueTopicsStr := os.Getenv(uniqueTopicsName)
	var uniqueTopics int

	uniqueTopics, err = strconv.Atoi(uniqueTopicsStr)
	if err != nil {
		uniqueTopics = 100
	}

	sendMessages(backends, uniqueTopics, b)
}

type messageData struct {
	topic   string
	pubkey  string
	payload string
}

type newMessageData func() *messageData

func manyTopics(n int, b *api.StatusBackend) (newMessageData, error) {
	w, err := b.NodeManager().WhisperService()
	if err != nil {
		return nil, err
	}
	a, err := w.NewKeyPair()
	if err != nil {
		return nil, err
	}
	pk, err := w.GetPrivateKey(a)
	if err != nil {
		return nil, err
	}

	pubkey := hexutil.Bytes(crypto.FromECDSAPub(&pk.PublicKey)).String()
	payload := hexutil.Encode([]byte("Hello world!"))

	var topics []topic
	for i := 0; i < n; i++ {
		tp := topic{}
		tp.topic = whisperv5.BytesToTopic([]byte(strconv.Itoa(i) + " test"))
		tp.str = tp.topic.String()

		topics = append(topics, tp)
	}

	i := -1
	return func() *messageData {
		i++
		return &messageData{
			topics[i%n].str,
			pubkey,
			payload,
		}
	}, nil
}

func sendMessages(backends []*api.StatusBackend, topicsCount int, b *testing.B) {
	getMessageData, err := manyTopics(topicsCount, backends[0])
	if err != nil {
		b.Fatal(err)
	}

	ticker := time.NewTicker(time.Millisecond * 10)
	defer ticker.Stop()

	nStr := os.Getenv(sendingCirclesName)
	n, err := strconv.Atoi(nStr)
	if err == nil {
		b.N = n
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		<-ticker.C

		data := getMessageData()
		message := `{
				"jsonrpc": "2.0",
				"method": "shh_post",
				"params": [
					{
					"pubKey": "` + data.pubkey + `",
					"topic": "` + data.topic + `",
					"payload": "` + data.payload + `",
					"powTarget": 0.001,
					"powTime": 2
					}
				],
				"id": 1}`

		backends[i%len(backends)].CallRPC(message)
	}
	b.StopTimer()
}

func startBackend(name string) (*api.StatusBackend, func()) {
	datadir := "../../.ethereumtest/whisperpref/" + name
	backend := api.NewStatusBackend()
	nodeConfig, err := e2e.MakeTestNodeConfig(GetNetworkID())
	if err != nil {
		panic(err.Error())
	}
	nodeConfig.DataDir = datadir
	nodeStarted, err := backend.StartNode(nodeConfig)
	if err != nil {
		panic(err.Error())
	}
	<-nodeStarted // wait till node is started

	return backend, func() {
		backendStopped, _ := backend.StopNode()
		<-backendStopped
		os.RemoveAll(datadir)
	}
}

func startMailboxBackend() (*api.StatusBackend, func()) {
	mailboxBackend := api.NewStatusBackend()
	mailboxConfig, err := e2e.MakeTestNodeConfig(GetNetworkID())
	if err != nil {
		panic(err.Error())
	}

	datadir := "../../.ethereumtest/whisperpref/mailserver/"

	mailboxConfig.LightEthConfig.Enabled = false
	mailboxConfig.WhisperConfig.Enabled = true
	mailboxConfig.KeyStoreDir = "../../.ethereumtest/whisperpref/mailserver"
	mailboxConfig.WhisperConfig.EnableMailServer = true
	mailboxConfig.WhisperConfig.IdentityFile = "../../static/keys/wnodekey"
	mailboxConfig.WhisperConfig.PasswordFile = "../../static/keys/wnodepassword"
	mailboxConfig.WhisperConfig.DataDir = "../../.ethereumtest/whisperpref/mailserver/data"
	mailboxConfig.DataDir = datadir

	mailboxNodeStarted, err := mailboxBackend.StartNode(mailboxConfig)
	if err != nil {
		panic(err.Error())
	}
	<-mailboxNodeStarted // wait till node is started
	return mailboxBackend, func() {
		backendStopped, _ := mailboxBackend.StopNode()
		<-backendStopped
		os.RemoveAll(datadir)
	}
}

func createPrivateChatMessageFilter(rpcCli *rpc.Client, privateKeyID string, topic string) string {
	resp := rpcCli.CallRaw(`{
			"jsonrpc": "2.0",
			"method": "shh_newMessageFilter", "params": [
				{"privateKeyID": "` + privateKeyID + `", "topics": [ "` + topic + `"], "allowP2P":true}
			],
			"id": 1
		}`)

	return resp
}

type topic struct {
	topic whisperv5.TopicType
	str   string
}

func getMailServerEnode(b *testing.B) string {
	var enode string
	tm := time.After(5 * time.Second)
	for {
		select {
		case <-tm:
			b.Fatal("the file 'enode.txt' should contains a mailbox enode")
		default:
			b, err := ioutil.ReadFile("./enode.txt")
			if err != nil {
				continue
			}
			enode = string(b)
		}
		if enode != "" {
			break
		}
	}
	b.Log(enode)

	return enode
}

type successfulResponce struct {
	Jsonrpc float64
	ID      int
	Result  string
}

func readPasswordFile() (string, error) {
	passwordFile := "../../static/keys/wnodepassword"
	password, err := ioutil.ReadFile(passwordFile)
	if err != nil {
		return "", err
	}
	password = bytes.TrimRight(password, "\n")

	return string(password), nil
}
