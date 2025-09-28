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
    APIs struct {
        RPC  []struct { Address string `json:"address"` } `json:"rpc"`
        GRPC []struct { Address string `json:"address"` } `json:"grpc"`
    } `json:"apis"`
}

func LoadChainRegistryChain(repoRoot, chainName string) (*ChainRegistryChain, error) {
    p := filepath.Join(repoRoot, "chain-registry", chainName, "chain.json")
    b, err := os.ReadFile(p)
    if err != nil { return nil, err }
    var c ChainRegistryChain
    if err := json.Unmarshal(b, &c); err != nil { return nil, err }
    return &c, nil
}
