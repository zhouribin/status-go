package api_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	gethcommon "github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/les"
	whisper "github.com/ethereum/go-ethereum/whisper/whisperv5"
	"github.com/status-im/status-go/geth/common"
	"github.com/status-im/status-go/geth/node"
	"github.com/status-im/status-go/geth/params"
	"github.com/status-im/status-go/geth/rpc"
	. "github.com/status-im/status-go/geth/testing"
)

type (
	jsonrpcMessage struct {
		Version string          `json:"jsonrpc"`
		ID      json.RawMessage `json:"id,omitempty"`
		Method  string          `json:"method,omitempty"`
		Params  json.RawMessage `json:"params,omitempty"`
		Error   *jsonError      `json:"error,omitempty"`
		Result  json.RawMessage `json:"result,omitempty"`
	}

	jsonError struct {
		Code    int         `json:"code,omitempty"`
		Message string      `json:"message"`
		Data    interface{} `json:"data,omitempty"`
	}
)

func (s *BackendTestSuite) TestTwoAccountMessages() {
	require := s.Require()
	require.NotNil(s.backend)

	s.StartTestBackend(params.RinkebyNetworkID)
	defer s.StopTestBackend()

	runningNode, err := s.backend.NodeManager().Node()
	require.NoError(err)
	require.NotNil(runningNode)

	nodeConfig, err := s.backend.NodeManager().NodeConfig()
	require.NoError(err)
	require.NotNil(nodeConfig)

	whisperService, err := s.backend.NodeManager().WhisperService()
	require.NoError(err)
	require.NotNil(whisperService)
	whisperAPI := whisper.NewPublicWhisperAPI(whisperService)

	var lesService *les.LightEthereum
	require.NoError(runningNode.Service(&lesService))
	require.NotNil(lesService)

	accountManager := s.backend.AccountManager()

	// ensure that there is still no accounts returned
	accounts := lesService.StatusBackend.AccountManager().Accounts()
	require.Zero(len(accounts), "accounts returned, while there should be none (we haven't logged in yet)")

	// account1
	_, selectedAcct1, err := accountManager.AddressToDecryptedAccount(TestConfig.Account1.Address, TestConfig.Account1.Password)
	require.NoError(err)
	user1KeySign := crypto.FromECDSAPub(&selectedAcct1.PrivateKey.PublicKey)
	user1KeyHex := gethcommon.ToHex(user1KeySign)
	_, user1Err := whisperService.AddKeyPair(selectedAcct1.PrivateKey)
	require.NoError(user1Err, fmt.Sprintf("user1 identity not injected: %v", user1KeyHex))
	require.True(whisperAPI.HasKeyPair(context.Background(), user1KeyHex), "Should have user 1 id")

	// account2
	_, selectedAcct2, err := accountManager.AddressToDecryptedAccount(TestConfig.Account2.Address, TestConfig.Account2.Password)
	require.NoError(err)
	user2KeySign := crypto.FromECDSAPub(&selectedAcct2.PrivateKey.PublicKey)
	user2KeyHex := gethcommon.ToHex(user2KeySign)
	_, user2Err := whisperService.AddKeyPair(selectedAcct2.PrivateKey)
	require.NoError(user2Err, fmt.Sprintf("user2 identity not injected: %v", user1KeyHex))
	require.True(whisperAPI.HasKeyPair(context.Background(), user2KeyHex), "Should have user 2 id")

	client, err := rpc.NewClient(runningNode, nodeConfig.UpstreamConfig)
	require.NoError(err)
	require.NotNil(client)

	node.SetDefaultNodeNotificationHandler(func(jsonEvent string) { // nolint :dupl
		var envelope node.SignalEnvelope
		err := json.Unmarshal([]byte(jsonEvent), &envelope)
		s.NoError(err, fmt.Sprintf("cannot unmarshal JSON: %s", jsonEvent))
		fmt.Printf("Notification: %#v\n", envelope)
	})

	// setup filter
	filterID, err := whisperAPI.NewMessageFilter(whisper.Criteria{
		Sig:          []byte(user1KeySign),
		PrivateKeyID: user2KeyHex,
		Topics: []whisper.TopicType{
			whisper.BytesToTopic([]byte("0x77686973")),
		},
	})
	require.NoError(err, "new filter encountered error")
	require.NotEmpty(filterID, "subscription filter id should not be empty")

	message := `{
		"id": 14,
		"jsonrpc": "2.0",
		"method": "shh_post",
		"params": [{
			"sig": "` + user1KeyHex + `",
			"pubKey": "` + user2KeyHex + `",
			"topics": "0x77686973",
			"payload": "0x776869737065722d636861742d636c69656e74",
			"priority": 1,
			"ttl": 50,
			"powTarget": 0.01,
			"powTime": 20 
		}]
	}`

	var messageDelr jsonrpcMessage
	messageResponse := client.CallRaw(message)
	err = json.Unmarshal([]byte(messageResponse), &messageDelr)
	require.NoError(err)
	require.Nil(messageDelr.Error)

	// Validate we have message in whisper api.
	whisperMsgs := whisperService.Messages(filterID)
	require.Len(whisperMsgs, 1, "Should have 1 pending messages")

	recmessage := `{
		"id": 15,
		"jsonrpc": "2.0",
		"method": "shh_getFilterMessages",
		"params": ["` + filterID + `"]
	}`

	var messageRec jsonrpcMessage
	recResponse := client.CallRaw(recmessage)
	err = json.Unmarshal([]byte(recResponse), &messageRec)
	require.NoError(err)
	require.Nil(messageRec.Error)
}

func (s *BackendTestSuite) TestAccountsList() {
	require := s.Require()
	require.NotNil(s.backend)

	s.StartTestBackend(params.RinkebyNetworkID)
	defer s.StopTestBackend()

	runningNode, err := s.backend.NodeManager().Node()
	require.NoError(err)
	require.NotNil(runningNode)

	var lesService *les.LightEthereum
	require.NoError(runningNode.Service(&lesService))
	require.NotNil(lesService)

	accounts := lesService.StatusBackend.AccountManager().Accounts()
	for _, acc := range accounts {
		fmt.Println(acc.Hex())
	}

	// make sure that we start with empty accounts list (nobody has logged in yet)
	require.Zero(len(accounts), "accounts returned, while there should be none (we haven't logged in yet)")

	// create an account
	address, _, _, err := s.backend.AccountManager().CreateAccount(TestConfig.Account1.Password)
	require.NoError(err)

	// ensure that there is still no accounts returned
	accounts = lesService.StatusBackend.AccountManager().Accounts()
	require.Zero(len(accounts), "accounts returned, while there should be none (we haven't logged in yet)")

	// select account (sub-accounts will be created for this key)
	err = s.backend.AccountManager().SelectAccount(address, TestConfig.Account1.Password)
	require.NoError(err, "account selection failed")

	// at this point main account should show up
	accounts = lesService.StatusBackend.AccountManager().Accounts()
	require.Equal(1, len(accounts), "exactly single account is expected (main account)")
	require.Equal(string(accounts[0].Hex()), "0x"+address,
		fmt.Sprintf("main account is not retured as the first key: got %s, expected %s", accounts[0].Hex(), "0x"+address))

	// create sub-account 1
	subAccount1, subPubKey1, err := s.backend.AccountManager().CreateChildAccount("", TestConfig.Account1.Password)
	require.NoError(err, "cannot create sub-account")

	// now we expect to see both main account and sub-account 1
	accounts = lesService.StatusBackend.AccountManager().Accounts()
	require.Equal(2, len(accounts), "exactly 2 accounts are expected (main + sub-account 1)")
	require.Equal(string(accounts[0].Hex()), "0x"+address, "main account is not retured as the first key")
	require.Equal(string(accounts[1].Hex()), "0x"+subAccount1, "subAcount1 not returned")

	// create sub-account 2, index automatically progresses
	subAccount2, subPubKey2, err := s.backend.AccountManager().CreateChildAccount("", TestConfig.Account1.Password)
	require.NoError(err, "cannot create sub-account")
	require.False(subAccount1 == subAccount2 || subPubKey1 == subPubKey2, "sub-account index auto-increament failed")

	// finally, all 3 accounts should show up (main account, sub-accounts 1 and 2)
	accounts = lesService.StatusBackend.AccountManager().Accounts()
	require.Equal(3, len(accounts), "unexpected number of accounts")
	require.Equal(string(accounts[0].Hex()), "0x"+address, "main account is not retured as the first key")

	subAccount1MatchesKey1 := string(accounts[1].Hex()) != "0x"+subAccount1
	subAccount1MatchesKey2 := string(accounts[2].Hex()) != "0x"+subAccount1
	require.False(!subAccount1MatchesKey1 && !subAccount1MatchesKey2, "subAcount1 not returned")

	subAccount2MatchesKey1 := string(accounts[1].Hex()) != "0x"+subAccount2
	subAccount2MatchesKey2 := string(accounts[2].Hex()) != "0x"+subAccount2
	require.False(!subAccount2MatchesKey1 && !subAccount2MatchesKey2, "subAcount2 not returned")
}

func (s *BackendTestSuite) TestCreateChildAccount() {
	require := s.Require()
	require.NotNil(s.backend)

	s.StartTestBackend(params.RinkebyNetworkID)
	defer s.StopTestBackend()

	keyStore, err := s.backend.NodeManager().AccountKeyStore()
	require.NoError(err)
	require.NotNil(keyStore)

	// create an account
	address, pubKey, mnemonic, err := s.backend.AccountManager().CreateAccount(TestConfig.Account1.Password)
	require.NoError(err)
	s.T().Logf("Account created: {address: %s, key: %s, mnemonic:%s}", address, pubKey, mnemonic)

	account, err := common.ParseAccountString(address)
	require.NoError(err, "can not get account from address")

	// obtain decrypted key, and make sure that extended key (which will be used as root for sub-accounts) is present
	_, key, err := keyStore.AccountDecryptedKey(account, TestConfig.Account1.Password)
	require.NoError(err, "can not obtain decrypted account key")
	require.NotNil(key.ExtendedKey, "CKD#2 has not been generated for new account")

	// try creating sub-account, w/o selecting main account i.e. w/o login to main account
	_, _, err = s.backend.AccountManager().CreateChildAccount("", TestConfig.Account1.Password)
	require.EqualError(node.ErrNoAccountSelected, err.Error(), "expected error is not returned (tried to create sub-account w/o login)")

	err = s.backend.AccountManager().SelectAccount(address, TestConfig.Account1.Password)
	require.NoError(err, "cannot select account")

	// try to create sub-account with wrong password
	_, _, err = s.backend.AccountManager().CreateChildAccount("", "wrong password")
	expectedErr := errors.New("cannot retrieve a valid key for a given account: could not decrypt key with given passphrase")
	require.EqualError(expectedErr, err.Error(), "create sub-account with wrong password")

	// create sub-account (from implicit parent)
	subAccount1, subPubKey1, err := s.backend.AccountManager().CreateChildAccount("", TestConfig.Account1.Password)
	require.NoError(err, "cannot create sub-account")

	// make sure that sub-account index automatically progresses
	subAccount2, subPubKey2, err := s.backend.AccountManager().CreateChildAccount("", TestConfig.Account1.Password)
	require.NoError(err)
	require.False(subAccount1 == subAccount2 || subPubKey1 == subPubKey2, "sub-account index auto-increament failed")

	// create sub-account (from explicit parent)
	subAccount3, subPubKey3, err := s.backend.AccountManager().CreateChildAccount(subAccount2, TestConfig.Account1.Password)
	require.NoError(err)
	require.False(subAccount1 == subAccount3 || subPubKey1 == subPubKey3 || subAccount2 == subAccount3 || subPubKey2 == subPubKey3)
}

func (s *BackendTestSuite) TestRecoverAccount() {
	require := s.Require()
	require.NotNil(s.backend)

	s.StartTestBackend(params.RinkebyNetworkID)
	defer s.StopTestBackend()

	keyStore, err := s.backend.NodeManager().AccountKeyStore()
	require.NoError(err)
	require.NotNil(keyStore)

	// create an account
	address, pubKey, mnemonic, err := s.backend.AccountManager().CreateAccount(TestConfig.Account1.Password)
	require.NoError(err)
	s.T().Logf("Account created: {address: %s, key: %s, mnemonic:%s}", address, pubKey, mnemonic)

	// try recovering using password + mnemonic
	addressCheck, pubKeyCheck, err := s.backend.AccountManager().RecoverAccount(TestConfig.Account1.Password, mnemonic)
	require.NoError(err, "recover account failed")
	require.False(address != addressCheck || pubKey != pubKeyCheck, "incorrect accound details recovered")

	// now test recovering, but make sure that account/key file is removed i.e. simulate recovering on a new device
	account, err := common.ParseAccountString(address)
	require.NoError(err, "can not get account from address")

	account, key, err := keyStore.AccountDecryptedKey(account, TestConfig.Account1.Password)
	require.NoError(err, "can not obtain decrypted account key")
	extChild2String := key.ExtendedKey.String()

	require.NoError(keyStore.Delete(account, TestConfig.Account1.Password), "cannot remove account")

	addressCheck, pubKeyCheck, err = s.backend.AccountManager().RecoverAccount(TestConfig.Account1.Password, mnemonic)
	require.NoError(err, "recover account failed (for non-cached account)")
	require.False(address != addressCheck || pubKey != pubKeyCheck,
		"incorrect account details recovered (for non-cached account)")

	// make sure that extended key exists and is imported ok too
	_, key, err = keyStore.AccountDecryptedKey(account, TestConfig.Account1.Password)
	require.NoError(err)
	require.Equal(extChild2String, key.ExtendedKey.String(), "CKD#2 key mismatch")

	// make sure that calling import several times, just returns from cache (no error is expected)
	addressCheck, pubKeyCheck, err = s.backend.AccountManager().RecoverAccount(TestConfig.Account1.Password, mnemonic)
	require.NoError(err, "recover account failed (for non-cached account)")
	require.False(address != addressCheck || pubKey != pubKeyCheck,
		"incorrect account details recovered (for non-cached account)")

	// time to login with recovered data
	whisperService := s.WhisperService()

	// make sure that identity is not (yet injected)
	require.False(whisperService.HasKeyPair(pubKeyCheck), "identity already present in whisper")
	require.NoError(s.backend.AccountManager().SelectAccount(addressCheck, TestConfig.Account1.Password))
	require.True(whisperService.HasKeyPair(pubKeyCheck), "identity not injected into whisper")
}

func (s *BackendTestSuite) TestSelectAccount() {
	require := s.Require()
	require.NotNil(s.backend)

	s.StartTestBackend(params.RinkebyNetworkID)
	defer s.StopTestBackend()

	// test to see if the account was injected in whisper
	whisperService := s.WhisperService()

	// create an account
	address1, pubKey1, _, err := s.backend.AccountManager().CreateAccount(TestConfig.Account1.Password)
	require.NoError(err)
	s.T().Logf("Account created: {address: %s, key: %s}", address1, pubKey1)

	address2, pubKey2, _, err := s.backend.AccountManager().CreateAccount(TestConfig.Account1.Password)
	require.NoError(err)
	s.T().Logf("Account created: {address: %s, key: %s}", address2, pubKey2)

	// make sure that identity is not (yet injected)
	require.False(whisperService.HasKeyPair(pubKey1), "identity already present in whisper")

	// try selecting with wrong password
	err = s.backend.AccountManager().SelectAccount(address1, "wrongPassword")
	expectedErr := errors.New("cannot retrieve a valid key for a given account: could not decrypt key with given passphrase")
	require.EqualError(expectedErr, err.Error(), "select account is expected to throw error: wrong password used")

	err = s.backend.AccountManager().SelectAccount(address1, TestConfig.Account1.Password)
	require.NoError(err)
	require.True(whisperService.HasKeyPair(pubKey1), "identity not injected into whisper")

	// select another account, make sure that previous account is wiped out from Whisper cache
	require.False(whisperService.HasKeyPair(pubKey2), "identity already present in whisper")
	require.NoError(s.backend.AccountManager().SelectAccount(address2, TestConfig.Account1.Password))
	require.True(whisperService.HasKeyPair(pubKey2), "identity not injected into whisper")
	require.False(whisperService.HasKeyPair(pubKey1), "identity should be removed, but it is still present in whisper")
}

func (s *BackendTestSuite) TestLogout() {
	require := s.Require()
	require.NotNil(s.backend)

	s.StartTestBackend(params.RinkebyNetworkID)
	defer s.StopTestBackend()

	whisperService := s.WhisperService()

	// create an account
	address, pubKey, _, err := s.backend.AccountManager().CreateAccount(TestConfig.Account1.Password)
	require.NoError(err)

	// make sure that identity doesn't exist (yet) in Whisper
	require.False(whisperService.HasKeyPair(pubKey), "identity already present in whisper")
	require.NoError(s.backend.AccountManager().SelectAccount(address, TestConfig.Account1.Password))
	require.True(whisperService.HasKeyPair(pubKey), "identity not injected into whisper")

	require.NoError(s.backend.AccountManager().Logout())
	require.False(whisperService.HasKeyPair(pubKey), "identity not cleared from whisper")
}

func (s *BackendTestSuite) TestSelectedAccountOnRestart() {
	require := s.Require()
	require.NotNil(s.backend)

	s.StartTestBackend(params.RinkebyNetworkID)

	// we need to make sure that selected account is injected as identity into Whisper
	whisperService := s.WhisperService()

	// create test accounts
	address1, pubKey1, _, err := s.backend.AccountManager().CreateAccount(TestConfig.Account1.Password)
	require.NoError(err)
	address2, pubKey2, _, err := s.backend.AccountManager().CreateAccount(TestConfig.Account1.Password)
	require.NoError(err)

	// make sure that identity is not (yet injected)
	require.False(whisperService.HasKeyPair(pubKey1), "identity already present in whisper")

	// make sure that no account is selected by default
	selectedAccount, err := s.backend.AccountManager().SelectedAccount()
	require.EqualError(node.ErrNoAccountSelected, err.Error(), "account selected, but should not be")
	require.Nil(selectedAccount)

	// select account
	err = s.backend.AccountManager().SelectAccount(address1, "wrongPassword")
	expectedErr := errors.New("cannot retrieve a valid key for a given account: could not decrypt key with given passphrase")
	require.EqualError(expectedErr, err.Error())

	require.NoError(s.backend.AccountManager().SelectAccount(address1, TestConfig.Account1.Password))
	require.True(whisperService.HasKeyPair(pubKey1), "identity not injected into whisper")

	// select another account, make sure that previous account is wiped out from Whisper cache
	require.False(whisperService.HasKeyPair(pubKey2), "identity already present in whisper")
	require.NoError(s.backend.AccountManager().SelectAccount(address2, TestConfig.Account1.Password))
	require.True(whisperService.HasKeyPair(pubKey2), "identity not injected into whisper")
	require.False(whisperService.HasKeyPair(pubKey1), "identity should be removed, but it is still present in whisper")

	// stop node (and all of its sub-protocols)
	nodeConfig, err := s.backend.NodeManager().NodeConfig()
	require.NoError(err)
	require.NotNil(nodeConfig)
	preservedNodeConfig := *nodeConfig
	nodeStoped, err := s.backend.StopNode()
	require.NoError(err)
	<-nodeStoped

	// make sure that account is still selected
	selectedAccount, err = s.backend.AccountManager().SelectedAccount()
	require.NoError(err)
	require.NotNil(selectedAccount)
	require.Equal(selectedAccount.Address.Hex(), "0x"+address2, "incorrect address selected")

	// resume node
	nodeStarted, err := s.backend.StartNode(&preservedNodeConfig)
	require.NoError(err)
	<-nodeStarted

	// re-check selected account (account2 MUST be selected)
	selectedAccount, err = s.backend.AccountManager().SelectedAccount()
	require.NoError(err)
	require.NotNil(selectedAccount)
	require.Equal(selectedAccount.Address.Hex(), "0x"+address2, "incorrect address selected")

	// make sure that Whisper gets identity re-injected
	whisperService = s.WhisperService()
	require.True(whisperService.HasKeyPair(pubKey2), "identity not injected into whisper")
	require.False(whisperService.HasKeyPair(pubKey1), "identity should not be present, but it is still present in whisper")

	// now restart node using RestartNode() method, and make sure that account is still available
	s.RestartTestNode()
	defer s.StopTestBackend()

	whisperService = s.WhisperService()
	require.True(whisperService.HasKeyPair(pubKey2), "identity not injected into whisper")
	require.False(whisperService.HasKeyPair(pubKey1), "identity should not be present, but it is still present in whisper")

	// now logout, and make sure that on restart no account is selected (i.e. logout works properly)
	require.NoError(s.backend.AccountManager().Logout())
	s.RestartTestNode()
	whisperService = s.WhisperService()
	require.False(whisperService.HasKeyPair(pubKey2), "identity not injected into whisper")
	require.False(whisperService.HasKeyPair(pubKey1), "identity should not be present, but it is still present in whisper")

	selectedAccount, err = s.backend.AccountManager().SelectedAccount()
	require.EqualError(node.ErrNoAccountSelected, err.Error())
	require.Nil(selectedAccount)
}

func (s *BackendTestSuite) TestRPCEthAccounts() {
	require := s.Require()

	s.StartTestBackend(params.RopstenNetworkID)
	defer s.StopTestBackend()

	// log into test account
	err := s.backend.AccountManager().SelectAccount(TestConfig.Account1.Address, TestConfig.Account1.Password)
	require.NoError(err)

	rpcClient := s.backend.NodeManager().RPCClient()

	expectedResponse := `{"jsonrpc":"2.0","id":1,"result":["` + TestConfig.Account1.Address + `"]}`
	resp := rpcClient.CallRaw(`{
              "jsonrpc": "2.0",
              "id": 1,
              "method": "eth_accounts",
              "params": []
      }`)
	require.Equal(expectedResponse, resp)
}

func (s *BackendTestSuite) TestRPCEthAccountsWithUpstream() {
	require := s.Require()

	s.StartTestBackend(params.RopstenNetworkID, WithUpstream("https://ropsten.infura.io/z6GCTmjdP3FETEJmMBI4"))
	defer s.StopTestBackend()

	// log into test account
	err := s.backend.AccountManager().SelectAccount(TestConfig.Account1.Address, TestConfig.Account1.Password)
	require.NoError(err)

	rpcClient := s.backend.NodeManager().RPCClient()

	expectedResponse := `{"jsonrpc":"2.0","id":1,"result":["` + TestConfig.Account1.Address + `"]}`
	resp := rpcClient.CallRaw(`{
              "jsonrpc": "2.0",
              "id": 1,
              "method": "eth_accounts",
              "params": []
      }`)
	require.Equal(expectedResponse, resp)
}
