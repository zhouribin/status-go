package node

import (
	"fmt"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/node"
	"github.com/ethereum/go-ethereum/swarm"
	bzzapi "github.com/ethereum/go-ethereum/swarm/api"
	"github.com/status-im/status-go/params"
)

var (
	// TODO: move this to the config or appropriate place
	bzzdir  = "/tmp"
	bzzport = 8542
)

func activatePssService(stack *node.Node, config *params.NodeConfig) (err error) {
	/*
		if config.WhisperConfig == nil || !config.WhisperConfig.Enabled {
			logger.Info("SHH protocol is disabled")
			return nil
		}
	*/

	return stack.Register(func(ctx *node.ServiceContext) (node.Service, error) {
		// generate a new private key
		privkey, err := crypto.GenerateKey()
		if err != nil {
			logger.Crit("private key generate fail", "error", err)
		}

		// create necessary swarm params
		bzzconfig := bzzapi.NewConfig()
		bzzconfig.Path = bzzdir
		bzzconfig.Init(privkey)
		bzzconfig.Port = fmt.Sprintf("%d", bzzport)

		return swarm.NewSwarm(bzzconfig, nil)
	})
}
