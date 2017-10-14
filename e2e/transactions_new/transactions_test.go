package transactions

import (
	"encoding/json"
	"testing"
	"time"

	gethcommon "github.com/ethereum/go-ethereum/common"
	cv "github.com/smartystreets/goconvey/convey"
	e2e "github.com/status-im/status-go/e2e"
	"github.com/status-im/status-go/geth/api"
	"github.com/status-im/status-go/geth/common"
	"github.com/status-im/status-go/geth/params"
	"github.com/status-im/status-go/geth/signal"
	"github.com/status-im/status-go/geth/txqueue"
	. "github.com/status-im/status-go/testing"
)

func TestSendEtherUsingRPC(t *testing.T) {
	backend := api.NewStatusBackend()

	nodeConfig, err := e2e.MakeTestNodeConfig(params.RopstenNetworkID)
	if err != nil {
		t.Error(err)
	}

	err = common.ImportTestAccount(nodeConfig.KeyStoreDir, "test-account1.pk")
	if err != nil {
		t.Error(err)
	}

	nodeStarted, err := backend.StartNode(nodeConfig)
	if err != nil {
		t.Error(err)
	}

	<-nodeStarted

	les, err := backend.NodeManager().LightEthereumService()
	if err != nil {
		t.Error(err)
	}

	for {
		isSyncing := les.Downloader().Synchronising()
		progress := les.Downloader().Progress()

		if !isSyncing && progress.HighestBlock > 0 && progress.CurrentBlock >= progress.HighestBlock {
			break
		}

		time.Sleep(time.Second * 10)
	}

	defer func() {
		nodeStopped, err := backend.StopNode()
		if err != nil {
			t.Error(err)
		}
		<-nodeStopped
	}()

	cv.Convey("Given a backend with a running node", t, func() {
		cv.Convey("When a notification handler is defined and completes the transaction", func(c cv.C) {
			txHash := gethcommon.Hash{}
			transactionCompleted := make(chan error, 1)

			signal.SetDefaultNodeNotificationHandler(func(rawSignal string) {
				var envelope signal.Envelope
				err := json.Unmarshal([]byte(rawSignal), &envelope)
				c.So(err, cv.ShouldBeNil)

				if envelope.Type != txqueue.EventTransactionQueued {
					return
				}

				event := envelope.Event.(map[string]interface{})
				txID := event["id"].(string)
				txHash, err = backend.CompleteTransaction(common.QueuedTxID(txID), TestConfig.Account1.Password)
				transactionCompleted <- err

				// clean up transaction if not completed properly
				if err != nil {
					err := backend.DiscardTransaction(common.QueuedTxID(txID))
					c.So(err, cv.ShouldBeNil)
				}
			})

			c.Convey("Should success when a valid account is selected", func() {
				err := backend.AccountManager().SelectAccount(TestConfig.Account1.Address, TestConfig.Account1.Password)
				c.So(err, cv.ShouldBeNil)

				result := backend.CallRPC(`{
					"jsonrpc": "2.0",
					"id": 1,
					"method": "eth_sendTransaction",
					"params": [{
						"from": "` + TestConfig.Account1.Address + `",
						"to": "` + TestConfig.Account2.Address + `",
						"value": "0x9184e72a"
					}]
				}`)
				c.So(<-transactionCompleted, cv.ShouldBeNil)
				c.So(result, cv.ShouldEqual, `{"jsonrpc":"2.0","id":1,"result":"`+txHash.String()+`"}`)
			})

			c.Convey("Should fail when an invalid account is selected", func() {
				err := backend.AccountManager().SelectAccount(TestConfig.Account2.Address, TestConfig.Account2.Password)
				c.So(err, cv.ShouldBeNil)

				result := backend.CallRPC(`{
					"jsonrpc": "2.0",
					"id": 1,
					"method": "eth_sendTransaction",
					"params": [{
						"from": "` + TestConfig.Account1.Address + `",
						"to": "` + TestConfig.Account2.Address + `",
						"value": "0x9184e72a"
					}]
				}`)
				c.So(<-transactionCompleted, cv.ShouldEqual, txqueue.ErrInvalidCompleteTxSender)
				c.So(result, cv.ShouldEqual, `{"jsonrpc":"2.0","id":1,"error":{"code":-32700,"message":"transaction has been discarded"}}`)
			})
		})
	})
}
