package whisper

import (
	"io/ioutil"
	"math/rand"
	"os"
	"path"
	"runtime"
	"strconv"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/whisper/whisperv5"
	"github.com/status-im/status-go/e2e"
	"github.com/status-im/status-go/geth/api"
	. "github.com/status-im/status-go/testing"
)

const DURATION = 300 * time.Second

func TestWhisperReceive(t *testing.T) {
	var enode string
	receiver, stop := startBackend("receiver")
	defer stop()

	tm := time.After(5 * time.Second)
	for {
		select {
		case <-tm:
			t.Fatal("env benchenode should contains mailbox enode")
		default:
			b, err := ioutil.ReadFile(getEnodeFilePath())
			if err != nil {
				continue
			}
			enode = string(b)

		}
		if enode != "" {
			break
		}
	}
	t.Log(enode)

	err := receiver.NodeManager().AddPeer(enode)
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(time.Second)
	topicS := whisperv5.BytesToTopic([]byte("test topic"))
	//topic:=topicS.String()
	w, err := receiver.NodeManager().WhisperService()
	if err != nil {
		t.Fatal(err)
	}
	keyID, err := w.NewKeyPair()
	if err != nil {
		t.Fatal(err)
	}
	pk, err := w.GetPrivateKey(keyID)
	if err != nil {
		t.Fatal(err)
	}

	f := &whisperv5.Filter{
		KeyAsym:  pk,
		Topics:   [][]byte{topicS[:]},
		AllowP2P: true,
	}

	filterID, err := w.Subscribe(f)
	if err != nil {
		t.Fatalf("Failed to install filter: %s", err)
	}
	defer w.Unsubscribe(filterID)

	t.Log(filterID)

	n, _ := receiver.NodeManager().Node()
	t.Log(n.Server().PeersInfo())
	time.Sleep(DURATION)
	t.Log("env with one topic: ", whisperv5.DuplicatedEnv)
	t.Log("all envs: ", whisperv5.AllEnv)
	t.Log("rps: ", whisperv5.AllEnv/int(DURATION.Seconds()))
}

func TestWhisperSendMessagesOneTopic(t *testing.T) {
	mailbox, stop := startMailboxBackend()
	defer stop()
	node, err := mailbox.NodeManager().Node()
	if err != nil {
		t.Fatal(err)
	}

	enode := node.Server().NodeInfo().Enode
	err = ioutil.WriteFile(getEnodeFilePath(), []byte(enode), os.ModePerm)
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(getEnodeFilePath())

	backends := make([]*api.StatusBackend, 5)
	for i := 0; i < 5; i++ {
		b, cl := startBackend("backend" + strconv.Itoa(i))
		backends[i] = b
		err := backends[i].NodeManager().AddPeer(enode)
		if err != nil {
			t.Fatal(err)
		}
		defer cl()
	}
	time.Sleep(time.Second)

	w, err := backends[0].NodeManager().WhisperService()
	if err != nil {
		t.Fatal(err)
	}
	a, err := w.NewKeyPair()
	if err != nil {
		t.Fatal(err)
	}
	pk, err := w.GetPrivateKey(a)
	if err != nil {
		t.Fatal(err)
	}
	pubkey := hexutil.Bytes(crypto.FromECDSAPub(&pk.PublicKey)).String()

	topicS := whisperv5.BytesToTopic([]byte("test topic"))
	topic := topicS.String()
	payload := hexutil.Encode([]byte("Hello world!"))
	c := time.After(DURATION)
	for {
		select {
		case <-c:
			return
		default:
			message := `{
				"jsonrpc": "2.0",
				"method": "shh_post",
				"params": [
					{
					"pubKey": "` + pubkey + `",
					"topic": "` + topic + `",
					"payload": "` + payload + `",
					"powTarget": 0.001,
					"powTime": 2
					}
				],
				"id": 1}`

			i := rand.Intn(5)
			str := backends[i].CallRPC(message)
			_ = str
			time.Sleep(time.Millisecond * 10)
		}
	}
}

func TestWhisperSendMessagesWithDifferentTopics(t *testing.T) {
	mailbox, stop := startMailboxBackend()
	defer stop()
	node, _ := mailbox.NodeManager().Node()
	enode := node.Server().NodeInfo().Enode
	err := ioutil.WriteFile(getEnodeFilePath(), []byte(enode), os.ModePerm)
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(getEnodeFilePath())

	backends := make([]*api.StatusBackend, 5)
	for i := 0; i < 5; i++ {
		b, cl := startBackend("backend" + strconv.Itoa(i))
		backends[i] = b
		err := backends[i].NodeManager().AddPeer(enode)
		if err != nil {
			t.Fatal(err)
		}
		defer cl()
	}
	time.Sleep(time.Second)
	w, err := backends[0].NodeManager().WhisperService()
	if err != nil {
		t.Fatal(err)
	}
	a, err := w.NewKeyPair()
	if err != nil {
		t.Fatal(err)
	}
	pk, err := w.GetPrivateKey(a)
	if err != nil {
		t.Fatal(err)
	}
	pubkey := hexutil.Bytes(crypto.FromECDSAPub(&pk.PublicKey)).String()
	payload := hexutil.Encode([]byte("Hello world!"))
	c := time.After(DURATION)
	for {
		select {
		case <-c:
			return
		default:
			j := rand.Intn(100)
			topicS := whisperv5.BytesToTopic([]byte(strconv.Itoa(j) + " test"))
			topic := topicS.String()

			message := `{
				"jsonrpc": "2.0",
				"method": "shh_post",
				"params": [
					{
					"pubKey": "` + pubkey + `",
					"topic": "` + topic + `",
					"payload": "` + payload + `",
					"powTarget": 0.001,
					"powTime": 2
					}
				],
				"id": 1}`

			i := rand.Intn(5)
			backends[i].CallRPC(message)
			time.Sleep(time.Millisecond * 10)
		}
	}
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

func getEnodeFilePath() string {
	_, f, _, _ := runtime.Caller(0)
	return path.Dir(f) + "/enode.txt"
}
