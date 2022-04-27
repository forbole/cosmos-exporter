package config

import (
	types "github.com/forbole/cosmos-exporter/types"
)

// Config defines all necessary parameters
type Config struct {
	DelegatorAddress string              `mapstructure:"delegator_address"`
	ValidatorAddress string              `mapstructure:"validator_address"`
	Port             string              `mapstructure:"port"`
	DenomMetadata    types.DenomMetadata `mapstructure:"denom_metadata"`
	Node             types.Node          `mapstructure:"node"`
}

// NewConfig builds a new Config instance
func NewConfig(
	delegatorAddress string, validatorAddress string, port string,
	nodeCfg types.Node, denomMetadataCfg types.DenomMetadata,
) Config {
	return Config{
		DelegatorAddress: delegatorAddress,
		ValidatorAddress: validatorAddress,
		Port:             port,
		Node:             nodeCfg,
		DenomMetadata:    denomMetadataCfg,
	}
}
