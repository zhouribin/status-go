package profiling

import (
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/whisper/whisperv5"
	"github.com/status-im/status-go/e2e"
	"github.com/status-im/status-go/geth/api"
	"github.com/status-im/status-go/geth/common"
	"github.com/status-im/status-go/geth/rpc"
	. "github.com/status-im/status-go/testing"
	"io/ioutil"
	"math/rand"
	"os"
	"os/exec"
	"path"
	"runtime"
	"runtime/pprof"
	"strconv"
	"testing"
	"time"
)

const (
	DURATION = 60 * time.Second
)

var baseTopic = whisperv5.BytesToTopic([]byte("test"))

//60 chats 1-1. receiving messages(10rpm per chat)
//10 group chats(60 rpm per chat)
//100 chats 1-1. 30 group chats. receiving historic messages for last day(100 messages per chat)

//node was started in background(sync was ended)
func BenchmarkNodeSyncingShhOff(b *testing.B) {
	datadir := "../../.ethereumtest/whisperpref/" + "SyncBackgroundShhOff_" + strconv.Itoa(GetNetworkID())
	defer os.RemoveAll(datadir)

	backend := api.NewStatusBackend()
	nodeConfig, err := e2e.MakeTestNodeConfig(GetNetworkID())
	nodeConfig.WhisperConfig.Enabled = false
	if err != nil {
		b.Fatal(err.Error())
	}
	nodeConfig.DataDir = datadir
	nodeConfig.KeyStoreDir = datadir

	nodeStarted, err := backend.StartNode(nodeConfig)
	if err != nil {
		b.Fatal(err.Error())
	}
	<-nodeStarted // wait till node is started
	profFile := getDir() + "/BenchmarkNodeSyncBackgroundShhOff.out"
	f, err := os.OpenFile(profFile, os.O_CREATE, os.ModePerm)
	if err != nil {
		b.Fatal(err)
	}
	defer f.Close()
	//wait full sync
	EnsureNodeSync(backend.NodeManager())

	err = pprof.StartCPUProfile(f)
	if err != nil {
		b.Fatal(err)
	}

	time.Sleep(DURATION)
	pprof.StopCPUProfile()
}

//node was started in background(sync was ended), whisper enabled
func BenchmarkNodeSyncBackgroundShhOn(b *testing.B) {
	datadir := "../../.ethereumtest/whisperpref/" + "SyncBackgroundShhOn_" + strconv.Itoa(GetNetworkID())
	defer os.RemoveAll(datadir)

	backend := api.NewStatusBackend()
	nodeConfig, err := e2e.MakeTestNodeConfig(GetNetworkID())

	if err != nil {
		b.Fatal(err.Error())
	}
	nodeConfig.DataDir = datadir
	nodeConfig.KeyStoreDir = datadir

	nodeStarted, err := backend.StartNode(nodeConfig)
	if err != nil {
		b.Fatal(err.Error())
	}
	<-nodeStarted // wait till node is started
	profFile := getDir() + "/BenchmarkNodeSyncBackgroundShhOn.out"
	f, err := os.OpenFile(profFile, os.O_CREATE, os.ModePerm)
	if err != nil {
		b.Fatal(err)
	}
	defer f.Close()

	//wait full sync
	EnsureNodeSync(backend.NodeManager())
	err = pprof.StartCPUProfile(f)
	if err != nil {
		b.Fatal(err)
	}
	defer pprof.StopCPUProfile()
	time.Sleep(DURATION)
}

//node is running
func BenchmarkNodeSync(b *testing.B) {
	datadir := "../../.ethereumtest/whisperpref/" + "Syncing_" + strconv.Itoa(GetNetworkID())
	defer os.RemoveAll(datadir)

	backend := api.NewStatusBackend()
	nodeConfig, err := e2e.MakeTestNodeConfig(GetNetworkID())
	nodeConfig.WhisperConfig.Enabled = false
	if err != nil {
		b.Fatal(err.Error())
	}
	nodeConfig.DataDir = datadir
	nodeConfig.KeyStoreDir = datadir

	nodeStarted, err := backend.StartNode(nodeConfig)
	if err != nil {
		b.Fatal(err.Error())
	}
	<-nodeStarted // wait till node is started
	profFile := getDir() + "/Syncing.out"
	f, err := os.OpenFile(profFile, os.O_CREATE, os.ModePerm)
	if err != nil {
		b.Fatal(err)
	}
	defer f.Close()

	err = pprof.StartCPUProfile(f)
	if err != nil {
		b.Fatal(err)
	}
	defer pprof.StopCPUProfile()

	//wait full sync
	EnsureNodeSync(backend.NodeManager())
}

//3)node was started in background(upstream)
//3.1) node was started in background(upstream) + whisper
func BenchmarkNodeUpstream(b *testing.B) {
	datadir := "../../.ethereumtest/whisperpref/" + "upstream_" + strconv.Itoa(GetNetworkID())
	defer os.RemoveAll(datadir)

	backend := api.NewStatusBackend()
	nodeConfig, err := e2e.MakeTestNodeConfig(GetNetworkID())
	nodeConfig.UpstreamConfig.Enabled = true
	nodeConfig.WhisperConfig.Enabled = false
	if err != nil {
		b.Fatal(err.Error())
	}
	nodeConfig.DataDir = datadir
	nodeConfig.KeyStoreDir = datadir

	nodeStarted, err := backend.StartNode(nodeConfig)
	if err != nil {
		b.Fatal(err.Error())
	}
	<-nodeStarted // wait till node is started
	profFile := getDir() + "/upstream.out"
	f, err := os.OpenFile(profFile, os.O_CREATE, os.ModePerm)
	if err != nil {
		b.Fatal(err)
	}
	defer f.Close()

	err = pprof.StartCPUProfile(f)
	if err != nil {
		b.Fatal(err)
	}
	defer pprof.StopCPUProfile()

	time.Sleep(DURATION)
}

func BenchmarkNodeReceiving(b *testing.B) {
	datadir := "../../.ethereumtest/whisperpref/" + "receive_" + strconv.Itoa(GetNetworkID())
	defer os.RemoveAll(datadir)

	backend := api.NewStatusBackend()
	nodeConfig, err := e2e.MakeTestNodeConfig(GetNetworkID())
	nodeConfig.UpstreamConfig.Enabled = false
	nodeConfig.LightEthConfig.Enabled = false
	nodeConfig.WhisperConfig.Enabled = true
	if err != nil {
		b.Fatal(err.Error())
	}
	nodeConfig.DataDir = datadir
	nodeConfig.KeyStoreDir = datadir

	nodeStarted, err := backend.StartNode(nodeConfig)
	if err != nil {
		b.Fatal(err.Error())
	}
	<-nodeStarted // wait till node is started

	profFile := getDir() + "/receive.out"
	f, err := os.OpenFile(profFile, os.O_CREATE|os.O_RDWR, os.ModePerm)
	if err != nil {
		b.Fatal(err)
	}
	defer f.Close()

	err = pprof.StartCPUProfile(f)
	if err != nil {
		b.Fatal(err)
	}
	defer pprof.StopCPUProfile()

	node, _ := backend.NodeManager().Node()
	enode := node.Server().NodeInfo().Enode

	w, err := backend.NodeManager().WhisperService()
	if err != nil {
		b.Fatal(err)
	}
	a, err := w.NewKeyPair()
	if err != nil {
		b.Fatal(err)
	}
	pk, err := w.GetPrivateKey(a)
	if err != nil {
		b.Fatal(err)
	}
	pubkey := hexutil.Bytes(crypto.FromECDSAPub(&pk.PublicKey)).String()

	resp := createPrivateChatMessageFilter(backend.NodeManager().RPCClient(), pubkey, baseTopic.String())
	b.Log(resp)

	// run sending messages in background
	senders := exec.Command("go",
		"test", getDir()+"/usecases_test.go",
		"-test.bench", "^BenchmarkNodeSendManyTopic$", "-test.v",
		"-receiver_enode", enode,
		"-num_of_nodes", "10",
		"-network", NetworkSelected,
	)
	senders.Dir = getDir()
	//senders.Stderr=os.Stderr
	//senders.Stdout=os.Stdout

	if err := senders.Start(); err != nil {
		b.Fatal(err)
	}
	defer func() {
		if err := senders.Process.Kill(); err != nil {
			b.Fatal(err)
		}
	}()

	time.Sleep(DURATION)
}

func BenchmarkNodeSendManyTopic(b *testing.B) {
	mailbox, stop := startMailboxBackend()
	err := mailbox.NodeManager().AddPeer(*EnodeReceiver)
	if err != nil {
		b.Fatal(err)
	}

	defer stop()
	b.Log("Start backends")

	ch := make(chan struct{}, 1)
	for i := 0; i < *NumOfNodes; i++ {
		back, cl := startBackend("backend" + strconv.Itoa(i))
		err := back.NodeManager().AddPeer(*EnodeReceiver)
		if err != nil {
			b.Fatal(err)
		}
		defer cl()

		time.Sleep(time.Second)
		w, err := back.NodeManager().WhisperService()
		if err != nil {
			b.Fatal(err)
		}
		a, err := w.NewKeyPair()
		if err != nil {
			b.Fatal(err)
		}
		pk, err := w.GetPrivateKey(a)
		if err != nil {
			b.Fatal(err)
		}
		pubkey := hexutil.Bytes(crypto.FromECDSAPub(&pk.PublicKey)).String()
		go func() {
			b := make([]byte, 200)
			topicBytes := [whisperv5.TopicLength]byte{}
			for {
				select {
				case <-ch:
					return
				default:
					rand.Read(b)
					rand.Read(topicBytes[:])
					payload := hexutil.Encode([]byte(b))
					topicS := whisperv5.TopicType(topicBytes)
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

					back.CallRPC(message)
					time.Sleep(time.Second)
				}
			}
		}()
	}
	time.Sleep(DURATION)
	close(ch)
}

//
//func TestWhisperReceive(t *testing.T) {
//	var enode string
//	receiver, stop := startBackend("receiver")
//	defer stop()
//
//	tm := time.After(5 * time.Second)
//	for {
//		select {
//		case <-tm:
//			t.Fatal("env benchenode should contains mailbox enode")
//		default:
//			b, err := ioutil.ReadFile(getEnodeFilePath())
//			if err != nil {
//				continue
//			}
//			enode = string(b)
//
//		}
//		if enode != "" {
//			break
//		}
//	}
//	t.Log(enode)
//
//	err := receiver.NodeManager().AddPeer(enode)
//	if err != nil {
//		t.Fatal(err)
//	}
//	time.Sleep(time.Second)
//	topicS := whisperv5.BytesToTopic([]byte("test topic"))
//	//topic:=topicS.String()
//	w, err := receiver.NodeManager().WhisperService()
//	if err != nil {
//		t.Fatal(err)
//	}
//	keyID, err := w.NewKeyPair()
//	if err != nil {
//		t.Fatal(err)
//	}
//	pk, err := w.GetPrivateKey(keyID)
//	if err != nil {
//		t.Fatal(err)
//	}
//
//	f := &whisperv5.Filter{
//		KeyAsym:  pk,
//		Topics:   [][]byte{topicS[:]},
//		AllowP2P: true,
//	}
//
//	filterID, err := w.Subscribe(f)
//	if err != nil {
//		t.Fatalf("Failed to install filter: %s", err)
//	}
//	defer w.Unsubscribe(filterID)
//
//	t.Log(filterID)
//
//	n, _ := receiver.NodeManager().Node()
//	t.Log(n.Server().PeersInfo())
//	time.Sleep(DURATION)
//	//t.Log("env with one topic: ", whisperv5.DuplicatedEnv)
//	//t.Log("opened envelopes: ", whisperv5.Opened)
//	//t.Log("all envelopes: ", whisperv5.AllEnv)
//	//t.Log("rps: ", whisperv5.AllEnv/int(DURATION.Seconds()))
//}
//
//func TestWhisperReceive_FilterOverHttp(t *testing.T) {
//	receiver, stop := startBackend("receiver")
//	defer stop()
//
//	time.Sleep(time.Second)
//	topicS := whisperv5.BytesToTopic([]byte("test topic"))
//	topic := topicS.String()
//	w, err := receiver.NodeManager().WhisperService()
//	if err != nil {
//		t.Fatal(err)
//	}
//	keyID, err := w.NewKeyPair()
//	if err != nil {
//		t.Fatal(err)
//	}
//	pk, err := w.GetPrivateKey(keyID)
//	if err != nil {
//		t.Fatal(err)
//	}
//	pubkey := hexutil.Bytes(crypto.FromECDSAPub(&pk.PublicKey)).String()
//	filterID := createPrivateChatMessageFilter(receiver.NodeManager().RPCClient(), pubkey, topic)
//	t.Log(filterID)
//
//	n, _ := receiver.NodeManager().Node()
//	t.Log(n.Server().PeersInfo())
//	//topics := w.Filters().Topics()
//	//for i := range topics {
//	//	t.Log("topics:", i, topics[i])
//	//}
//	//time.Sleep(time.Second)
//	//t.Log("env with one topic: ", whisperv5.DuplicatedEnv)
//	//t.Log("opened envelopes: ", whisperv5.Opened)
//	//t.Log("all envelopes: ", whisperv5.AllEnv)
//	//t.Log("rps: ", whisperv5.AllEnv/int(DURATION.Seconds()))
//}
//
//func TestWhisperSendMessagesOneTopic(t *testing.T) {
//	mailbox, stop := startMailboxBackend()
//	defer stop()
//	node, err := mailbox.NodeManager().Node()
//	if err != nil {
//		t.Fatal(err)
//	}
//
//	enode := node.Server().NodeInfo().Enode
//	err = ioutil.WriteFile(getEnodeFilePath(), []byte(enode), os.ModePerm)
//	if err != nil {
//		t.Fatal(err)
//	}
//	defer os.Remove(getEnodeFilePath())
//
//	backends := make([]*api.StatusBackend, 5)
//	for i := 0; i < 5; i++ {
//		b, cl := startBackend("backend" + strconv.Itoa(i))
//		backends[i] = b
//		err := backends[i].NodeManager().AddPeer(enode)
//		if err != nil {
//			t.Fatal(err)
//		}
//		defer cl()
//	}
//	time.Sleep(time.Second)
//
//	w, err := backends[0].NodeManager().WhisperService()
//	if err != nil {
//		t.Fatal(err)
//	}
//	a, err := w.NewKeyPair()
//	if err != nil {
//		t.Fatal(err)
//	}
//	pk, err := w.GetPrivateKey(a)
//	if err != nil {
//		t.Fatal(err)
//	}
//	pubkey := hexutil.Bytes(crypto.FromECDSAPub(&pk.PublicKey)).String()
//
//	topicS := whisperv5.BytesToTopic([]byte("test topic"))
//	topic := topicS.String()
//	payload := hexutil.Encode([]byte("Hello world!"))
//	c := time.After(DURATION)
//	for {
//		select {
//		case <-c:
//			return
//		default:
//			message := `{
//				"jsonrpc": "2.0",
//				"method": "shh_post",
//				"params": [
//					{
//					"pubKey": "` + pubkey + `",
//					"topic": "` + topic + `",
//					"payload": "` + payload + `",
//					"powTarget": 0.001,
//					"powTime": 2
//					}
//				],
//				"id": 1}`
//
//			i := rand.Intn(5)
//			str := backends[i].CallRPC(message)
//			_ = str
//			time.Sleep(time.Millisecond * 10)
//		}
//	}
//}
//
//func TestWhisperSendMessagesWithDifferentTopics(t *testing.T) {
//	mailbox, stop := startMailboxBackend()
//	defer stop()
//	node, _ := mailbox.NodeManager().Node()
//	enode := node.Server().NodeInfo().Enode
//	err := ioutil.WriteFile(getEnodeFilePath(), []byte(enode), os.ModePerm)
//	if err != nil {
//		t.Fatal(err)
//	}
//	defer os.Remove(getEnodeFilePath())
//
//	backends := make([]*api.StatusBackend, 5)
//	for i := 0; i < 5; i++ {
//		b, cl := startBackend("backend" + strconv.Itoa(i))
//		backends[i] = b
//		err := backends[i].NodeManager().AddPeer(enode)
//		if err != nil {
//			t.Fatal(err)
//		}
//		defer cl()
//	}
//	time.Sleep(time.Second)
//	w, err := backends[0].NodeManager().WhisperService()
//	if err != nil {
//		t.Fatal(err)
//	}
//	a, err := w.NewKeyPair()
//	if err != nil {
//		t.Fatal(err)
//	}
//	pk, err := w.GetPrivateKey(a)
//	if err != nil {
//		t.Fatal(err)
//	}
//	pubkey := hexutil.Bytes(crypto.FromECDSAPub(&pk.PublicKey)).String()
//	payload := hexutil.Encode([]byte("Hello world!"))
//	c := time.After(DURATION)
//	for {
//		select {
//		case <-c:
//			return
//		default:
//			j := rand.Intn(100)
//			topicS := whisperv5.BytesToTopic([]byte(strconv.Itoa(j) + " test"))
//			topic := topicS.String()
//
//			message := `{
//				"jsonrpc": "2.0",
//				"method": "shh_post",
//				"params": [
//					{
//					"pubKey": "` + pubkey + `",
//					"topic": "` + topic + `",
//					"payload": "` + payload + `",
//					"powTarget": 0.001,
//					"powTime": 2
//					}
//				],
//				"id": 1}`
//
//			i := rand.Intn(5)
//			backends[i].CallRPC(message)
//			time.Sleep(time.Millisecond * 10)
//		}
//	}
//}

func startBackend(name string) (*api.StatusBackend, func()) {
	datadir := "../../.ethereumtest/whisperpref/" + name
	backend := api.NewStatusBackend()
	nodeConfig, err := e2e.MakeTestNodeConfig(GetNetworkID())
	if err != nil {
		panic(err.Error())
	}
	nodeConfig.LightEthConfig.Enabled = false

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
func getDir() string {
	_, f, _, _ := runtime.Caller(0)
	return path.Dir(f)
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

func setEnode(n common.NodeManager) (error, func()) {
	node, _ := n.Node()
	enode := node.Server().NodeInfo().Enode
	err := ioutil.WriteFile(getEnodeFilePath(), []byte(enode), os.ModePerm)
	return err, func() {
		os.Remove(getEnodeFilePath())
	}
}
