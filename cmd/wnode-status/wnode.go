package main

import (
	"fmt"

	"github.com/ethereum/go-ethereum/p2p/discover"
)

func extractIdFromEnode(s string) ([]byte, error) {
	n, err := discover.ParseNode(s)
	if err != nil {
		return nil, fmt.Errorf("Failed to parse enode: %s", err)
	}
	return n.ID[:], nil
}
