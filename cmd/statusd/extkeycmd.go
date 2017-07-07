package main

import (
	"fmt"
	"encoding/hex"
	"strings"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/status-im/status-go/extkeys"
	"github.com/status-im/status-go/geth"
	"github.com/status-im/status-go/geth/params"
	"gopkg.in/urfave/cli.v1"
)

var (
	ExtKeyMnemonic = cli.StringFlag{
		Name:  "mnemonic",
		Usage: "File with 12 words of mnemonic string",
	}
	ExtKeyPassword = cli.StringFlag{
		Name:  "password",
		Usage: "File with password to private key",
	}
)

var (
	extKeyCommand = cli.Command{
		Action: extKeyCommandHandler,
		Name:   "extkey",
		Usage:  "Generates extended key JSON file, for a given mnemonic and password",
		Flags: []cli.Flag{
			ExtKeyMnemonic,
			ExtKeyPassword,
		},
	}
)

// extKeyCommandHandler handles `statusd extkey` command
func extKeyCommandHandler(ctx *cli.Context) error {
	config, err := parseExtKeyCommandConfig(ctx)
	if err != nil {
		return fmt.Errorf("can not parse config: %v", err)
	}

	fmt.Println("Starting Status node..")
	if err = geth.CreateAndRunNode(config); err != nil {
		return err
	}

	// create account file
	password := ctx.String(ExtKeyPassword.Name)
	if err != nil {
		return err
	}

	mnemonic := ctx.String(ExtKeyMnemonic.Name)
	if err != nil {
		return err
	}
	addr, _, err := createExtKey(strings.TrimSpace(string(mnemonic)), strings.TrimSpace(string(password)))
	if err != nil {
		return err
	}
	fmt.Println("Account Address: ", addr)

	return nil
}

// parseExtKeyCommandConfig parses incoming CLI options and returns node configuration object
func parseExtKeyCommandConfig(ctx *cli.Context) (*params.NodeConfig, error) {
	nodeConfig, err := makeNodeConfig(ctx)
	if err != nil {
		return nil, err
	}
	// update data store path
	nodeConfig.DataDir = "extkey"

	// select sub-protocols
	nodeConfig.LightEthConfig.Enabled = false
	nodeConfig.WhisperConfig.Enabled = false
	nodeConfig.SwarmConfig.Enabled = false

	// RPC configuration
	nodeConfig.APIModules = "eth"
	nodeConfig.HTTPHost = ""

	// extra options
	nodeConfig.BootClusterConfig.Enabled = false

	return nodeConfig, nil
}

// createExtKey generates and imports into keystore a private key based on a given mnemonic/password
func createExtKey(mnemonic, password string) (address, pubKey string, err error) {
	m := extkeys.NewMnemonic(extkeys.Salt)

	// generate extended master key (see BIP32)
	extKey, err := extkeys.NewMaster(m.MnemonicSeed(mnemonic, password), []byte(extkeys.Salt))
	if err != nil {
		return "", "", fmt.Errorf("can not create master extended key: %v", err)
	}

	// import created key into account keystore
	address, pubKey, err = importExtendedKey(extKey, password)
	if err != nil {
		return "", "", err
	}

	return address, pubKey, nil
}

// importExtendedKey processes incoming extended key, extracts required info and creates corresponding account key.
// Once account key is formed, that key is put (if not already) into keystore i.e. key is *encoded* into key file.
func importExtendedKey(extKey *extkeys.ExtendedKey, password string) (address, pubKey string, err error) {
	keyStore, err := geth.NodeManagerInstance().AccountKeyStore()
	if err != nil {
		return "", "", err
	}

	// imports extended key, create key file (if necessary)
	account, err := keyStore.ImportExtendedKey(extKey, password)
	if err != nil {
		return "", "", err
	}
	address = fmt.Sprintf("%x", account.Address)

	// obtain public key to return
	account, key, err := keyStore.AccountDecryptedKey(account, password)
	if err != nil {
		return address, "", err
	}
	pubKey = common.ToHex(crypto.FromECDSAPub(&key.PrivateKey.PublicKey))

	fmt.Println("Private Key: ", hex.EncodeToString(crypto.FromECDSA(key.PrivateKey)))

	return
}
