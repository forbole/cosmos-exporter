package collector

import (
	"context"

	querytypes "github.com/cosmos/cosmos-sdk/types/query"
	banktypes "github.com/cosmos/cosmos-sdk/x/bank/types"
	minttypes "github.com/cosmos/cosmos-sdk/x/mint/types"
	stakingtypes "github.com/cosmos/cosmos-sdk/x/staking/types"
	types "github.com/forbole/cosmos-exporter/types"
	tmrpc "github.com/tendermint/tendermint/rpc/client/http"
	"google.golang.org/grpc"
)

type CosmosSDKCollector struct {
	grpcConn *grpc.ClientConn
	//https://docs.cosmos.network/master/basics/accounts.html
	valAddress       string
	accAddresses     []string
	chainID          string
	denomMetadata    map[string]types.DenomMetadata
	defaultBondDenom string
	defaultMintDenom string
}

func NewCosmosSDKCollector(grpcConn *grpc.ClientConn, rpcConn string, valAddress string, accAddresses []string, customDenomData types.DenomMetadata) CosmosSDKCollector {
	chainID := getChainID(rpcConn)
	denomsMetadata := make(map[string]types.DenomMetadata)
	addDenomsMetadata(grpcConn, denomsMetadata)
	addCustomDenomMetadata(customDenomData, denomsMetadata)

	var defaultMintDenom string
	var defaultBondDenom string
	if denom, err := getMintDenom(grpcConn); err != nil {
		defaultMintDenom = customDenomData.Base
	} else {
		defaultMintDenom = denom
	}
	if denom, err := getBondDenom(grpcConn); err != nil {
		defaultBondDenom = customDenomData.Base
	} else {
		defaultBondDenom = denom
	}

	return CosmosSDKCollector{
		grpcConn:         grpcConn,
		chainID:          chainID,
		valAddress:       valAddress,
		accAddresses:     accAddresses,
		denomMetadata:    denomsMetadata,
		defaultBondDenom: defaultBondDenom,
		defaultMintDenom: defaultMintDenom,
	}
}

func (c *CosmosSDKCollector) CollectChainMetrics() {
	c.CollectActiveProposal()
	c.CollectAvailableBalance()
	c.CollectDeleatorReward()
	c.CollecDelegatorStake()
	c.CollectValidatorCommissionGauge()
	c.CollectValidatorDelegationGauge()
	c.CollectValidatorStat()
	c.CollectValidatorsStat()
	c.CollectCirculatingSupply()
	c.CollectInflationRate()
}

// Find Chain id to add as metrics lable
func getChainID(rpc string) string {
	client, err := tmrpc.New(rpc, "/websocket")
	if err != nil {
		panic(err)
	}

	status, err := client.Status(context.Background())
	if err != nil {
		panic(err)
	}

	return status.NodeInfo.Network
}

// Find Denom metadata to convert to human-readable unit (eg. udsm -> dsm)
func addDenomsMetadata(grpcConn *grpc.ClientConn, denomsMetadata map[string]types.DenomMetadata) {
	bankClient := banktypes.NewQueryClient(grpcConn)
	denomsRes, err := bankClient.DenomsMetadata(
		context.Background(),
		&banktypes.QueryDenomsMetadataRequest{
			Pagination: &querytypes.PageRequest{
				Limit: 1000,
			},
		},
	)
	if err != nil {
		panic(err)
	}

	for _, metadata := range denomsRes.Metadatas {
		var exponent uint32
		for _, denom := range metadata.DenomUnits {
			if denom.Denom == metadata.Display {
				exponent = denom.Exponent
			}
		}
		denomsMetadata[metadata.Base] = types.NewDenomMetadata(metadata.Base, metadata.Display, exponent)
	}
}

// In some chains, DenomsMetadata request return empty so needs to add manually
func addCustomDenomMetadata(cfgDenom types.DenomMetadata, denomsMetadata map[string]types.DenomMetadata) {
	if !cfgDenom.IsStructureEmpty() && (cfgDenom.Base != "" && cfgDenom.Display != "" && cfgDenom.Exponent != 0) {
		denomsMetadata[cfgDenom.Base] = types.NewDenomMetadata(cfgDenom.Base, cfgDenom.Display, cfgDenom.Exponent)
	}
}

func getMintDenom(grpcConn *grpc.ClientConn) (string, error) {
	mintClient := minttypes.NewQueryClient(grpcConn)
	mintParamsRes, err := mintClient.Params(
		context.Background(),
		&minttypes.QueryParamsRequest{},
	)

	if err != nil {
		return "", err
	}

	return mintParamsRes.Params.MintDenom, nil
}

func getBondDenom(grpcConn *grpc.ClientConn) (string, error) {
	stakingClient := stakingtypes.NewQueryClient(grpcConn)
	stakingParamsRes, err := stakingClient.Params(
		context.Background(),
		&stakingtypes.QueryParamsRequest{},
	)
	if err != nil {
		return "", err
	}

	return stakingParamsRes.Params.BondDenom, nil
}
