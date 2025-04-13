package collector

import (
	"context"
	"log"
	"strings"
	"time"

	cmthttp "github.com/cometbft/cometbft/rpc/client/http"
	querytypes "github.com/cosmos/cosmos-sdk/types/query"
	banktypes "github.com/cosmos/cosmos-sdk/x/bank/types"
	minttypes "github.com/cosmos/cosmos-sdk/x/mint/types"
	stakingtypes "github.com/cosmos/cosmos-sdk/x/staking/types"
	types "github.com/forbole/cosmos-exporter/types"
	"google.golang.org/grpc"
)

type SDKVersion string

const (
	SDKVersionLegacy  SDKVersion = "legacy"  // Pre-v0.50.x with Tendermint
	SDKVersionCurrent SDKVersion = "current" // v0.50.x with CometBFT
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
	sdkVersion       SDKVersion
}

// Detect SDK version based on API behavior
func detectSDKVersion(grpcConn *grpc.ClientConn, rpcConn string) SDKVersion {
	// First check the Params API format which differs between versions
	stakingClient := stakingtypes.NewQueryClient(grpcConn)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*3)
	defer cancel()

	// Try to get staking params
	res, err := stakingClient.Params(ctx, &stakingtypes.QueryParamsRequest{})
	if err == nil && res != nil {
		// Check response format details that differ between versions
		respStr := res.String()

		// v0.50.x chains typically have these patterns
		if strings.Contains(respStr, "cosmos.staking.v1beta1") ||
			strings.Contains(respStr, "historical_entries:") {
			log.Printf("Detected current SDK version (v0.50.x) from staking params")
			return SDKVersionCurrent
		}
	}

	// Additional check using bank API
	bankClient := banktypes.NewQueryClient(grpcConn)

	// This approach uses different behaviors of pagination in different versions
	_, err = bankClient.DenomsMetadata(ctx, &banktypes.QueryDenomsMetadataRequest{
		Pagination: &querytypes.PageRequest{
			CountTotal: true,
		},
	})

	if err != nil {
		if strings.Contains(err.Error(), "DecProto") ||
			strings.Contains(err.Error(), "LegacyDec") ||
			strings.Contains(err.Error(), "not found in table") ||
			strings.Contains(err.Error(), "cosmos.base") {
			// If we get specific errors related to older type systems, it's likely a pre-v0.50.x chain
			log.Printf("Detected legacy SDK version (pre-v0.50.x) from specific error pattern")
			return SDKVersionLegacy
		}
	}

	// Test for mint module API compatibility
	mintClient := minttypes.NewQueryClient(grpcConn)
	_, mintErr := mintClient.AnnualProvisions(ctx, &minttypes.QueryAnnualProvisionsRequest{})

	if mintErr != nil {
		if strings.Contains(mintErr.Error(), "invalid Go type math.LegacyDec") {
			// This error specifically happens with v0.50.x chains
			log.Printf("Detected current SDK version (v0.50.x) from mint error")
			return SDKVersionCurrent
		}

		// If we get server errors that might indicate older SDKs
		if strings.Contains(mintErr.Error(), "not implemented") ||
			strings.Contains(mintErr.Error(), "no RPC service") {
			log.Printf("Detected likely legacy SDK version from mint API absence")
			return SDKVersionLegacy
		}
	}

	log.Printf("SDK version not definitively detected, attempting one more check...")

	// As a final check, try to determine version based on CometBFT/Tendermint response format
	if client, err := cmthttp.New(rpcConn, "/websocket"); err == nil {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second*3)
		defer cancel()

		if status, err := client.Status(ctx); err == nil {
			if status.NodeInfo.Version != "" {
				version := status.NodeInfo.Version
				// Check if it's CometBFT (v0.50.x) or Tendermint (pre-v0.50.x)
				if strings.Contains(strings.ToLower(version), "comet") ||
					strings.HasPrefix(version, "0.37.") ||
					strings.HasPrefix(version, "0.38.") {
					log.Printf("Detected current SDK version (v0.50.x) based on CometBFT version")
					return SDKVersionCurrent
				} else if strings.HasPrefix(version, "0.34.") ||
					strings.Contains(strings.ToLower(version), "tendermint") {
					log.Printf("Detected legacy SDK version based on Tendermint version")
					return SDKVersionLegacy
				}
			}
		}
	}

	// Fallback - for safety, we'll assume current
	log.Printf("SDK version not definitively detected, assuming current v0.50.x")
	return SDKVersionCurrent
}

func NewCosmosSDKCollector(grpcConn *grpc.ClientConn, rpcConn string, valAddress string, accAddresses []string, customDenomData types.DenomMetadata) CosmosSDKCollector {
	chainID := getChainID(rpcConn)

	// Detect SDK version
	sdkVersion := detectSDKVersion(grpcConn, rpcConn)

	denomsMetadata := make(map[string]types.DenomMetadata)

	// Use version-appropriate code
	if sdkVersion == SDKVersionLegacy {
		addDenomsMetadataLegacy(grpcConn, denomsMetadata)
	} else {
		addDenomsMetadata(grpcConn, denomsMetadata)
	}

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

	// Ensure we have at least basic metadata even if the RPC fails
	ensureMinimumDenomMetadata(denomsMetadata, customDenomData.Base)

	return CosmosSDKCollector{
		grpcConn:         grpcConn,
		chainID:          chainID,
		valAddress:       valAddress,
		accAddresses:     accAddresses,
		denomMetadata:    denomsMetadata,
		defaultBondDenom: defaultBondDenom,
		defaultMintDenom: defaultMintDenom,
		sdkVersion:       sdkVersion,
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
	c.CollectCommunityTax()
	c.CollectUnbondingTime()
}

// Find Chain id to add as metrics lable
func getChainID(rpc string) string {
	client, err := cmthttp.New(rpc, "/websocket")
	if err != nil {
		log.Printf("Error creating RPC client: %v", err)
		return "unknown-chain"
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second*5)
	defer cancel()

	status, err := client.Status(ctx)
	if err != nil {
		log.Printf("Error getting chain status: %v", err)
		return "unknown-chain"
	}

	return status.NodeInfo.Network
}

// Find Denom metadata to convert to human-readable unit (eg. udsm -> dsm)
func addDenomsMetadata(grpcConn *grpc.ClientConn, denomsMetadata map[string]types.DenomMetadata) {
	bankClient := banktypes.NewQueryClient(grpcConn)

	// In v0.50.x, pagination works differently
	// Use the v1beta1.PageRequest which has been updated
	denomsRes, err := bankClient.DenomsMetadata(
		context.Background(),
		&banktypes.QueryDenomsMetadataRequest{
			Pagination: &querytypes.PageRequest{
				Limit:      1000,
				CountTotal: true,
			},
		},
	)
	if err != nil {
		log.Printf("Error getting denoms metadata: %v", err)
		return
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

	// In v0.50.x, MintDenom might be at a different path in the params response
	// Check if we're using the legacy structure or the new one
	if mintParamsRes != nil && mintParamsRes.Params.MintDenom != "" {
		return mintParamsRes.Params.MintDenom, nil
	}

	// If MintDenom isn't directly accessible, attempt to get it from another query
	// Some chains may have modified the mint module or not have it
	return "", err
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

// Add legacy version of denomsMetadata function
func addDenomsMetadataLegacy(grpcConn *grpc.ClientConn, denomsMetadata map[string]types.DenomMetadata) {
	bankClient := banktypes.NewQueryClient(grpcConn)

	// Use the legacy v1beta1.PageRequest which is compatible with pre-v0.50.x
	denomsRes, err := bankClient.DenomsMetadata(
		context.Background(),
		&banktypes.QueryDenomsMetadataRequest{
			Pagination: &querytypes.PageRequest{
				Limit: 1000,
			},
		},
	)
	if err != nil {
		log.Printf("Error getting denoms metadata: %v", err)
		return
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

// Add this to collector/main.go after addCustomDenomMetadata function
func ensureMinimumDenomMetadata(denomsMetadata map[string]types.DenomMetadata, defaultDenom string) {
	// If we have no denom metadata at all, add some sensible defaults
	if len(denomsMetadata) == 0 {
		log.Printf("No denom metadata found, adding fallbacks for common tokens")

		// Add common denoms with their typical exponents
		commonDenoms := map[string]struct {
			display  string
			exponent uint32
		}{
			"uatom": {"atom", 6},
			"stake": {"stake", 0},
			"inj":   {"inj", 18},
			"ujuno": {"juno", 6},
			"uosmo": {"osmo", 6},
			// Add the default denom from config if not already covered
			defaultDenom: {defaultDenom, 6}, // Assume micro units (10^6) by default
		}

		for denom, info := range commonDenoms {
			// Only add if not already present
			if _, exists := denomsMetadata[denom]; !exists {
				denomsMetadata[denom] = types.NewDenomMetadata(
					denom,
					info.display,
					info.exponent,
				)
				log.Printf("Added fallback metadata for %s", denom)
			}
		}
	}
}
