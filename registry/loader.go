package registry

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// ChainRegistryChain minimal struct for endpoints
type ChainRegistryChain struct {
	ChainName string `json:"chain_name"`
	ChainID   string `json:"chain_id"`
	APIs      struct {
		RPC []struct {
			Address string `json:"address"`
		} `json:"rpc"`
		GRPC []struct {
			Address string `json:"address"`
		} `json:"grpc"`
	} `json:"apis"`
}

// LoadChainRegistryChain loads chain.json from either:
// 1. basePath/<chainName>/chain.json (preferred when --chain-registry-path points directly to the repo root)
// 2. basePath/chain-registry/<chainName>/chain.json (backward compatibility when basePath is project root)
func LoadChainRegistryChain(basePath, chainName string) (*ChainRegistryChain, error) {
	primary := filepath.Join(basePath, chainName, "chain.json")
	var data []byte
	if b, e := os.ReadFile(primary); e == nil {
		data = b
	} else {
		fallback := filepath.Join(basePath, "chain-registry", chainName, "chain.json")
		if b2, e2 := os.ReadFile(fallback); e2 == nil {
			data = b2
		} else {
			return nil, e // return original primary error for clarity
		}
	}
	var c ChainRegistryChain
	if err := json.Unmarshal(data, &c); err != nil {
		return nil, err
	}
	return &c, nil
}
