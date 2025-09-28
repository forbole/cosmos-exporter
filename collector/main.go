package collector

import (
	"context"
	"log"
	"math/rand"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	cmthttp "github.com/cometbft/cometbft/rpc/client/http"
	querytypes "github.com/cosmos/cosmos-sdk/types/query"
	banktypes "github.com/cosmos/cosmos-sdk/x/bank/types"
	minttypes "github.com/cosmos/cosmos-sdk/x/mint/types"
	stakingtypes "github.com/cosmos/cosmos-sdk/x/staking/types"
	types "github.com/forbole/cosmos-exporter/types"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

type SDKVersion string

const (
	SDKVersionLegacy  SDKVersion = "legacy"  // Pre-v0.50.x with Tendermint
	SDKVersionCurrent SDKVersion = "current" // v0.50.x with CometBFT
)

// CosmosSDKCollector holds state for metrics collection including runtime failover fields
type CosmosSDKCollector struct {
	grpcConn         *grpc.ClientConn
	valAddress       string
	accAddresses     []string
	chainID          string
	lastGoodChainID  string
	denomMetadata    map[string]types.DenomMetadata
	defaultMintDenom string
	defaultBondDenom string
	sdkVersion       SDKVersion

	// vote tracking state
	voteMissCounts  map[string]int     // key: proposalID|address
	lastVoteStatus  map[string]float64 // last emitted value
	proposalAllMiss map[uint64]int     // consecutive scrapes with no votes for a proposal
	stateMu         sync.Mutex

	// runtime endpoint failover (future use)
	grpcEndpoints   []string
	grpcIndex       int
	grpcErrorStreak int
	maxGRPCErrors   int
	rpcEndpoints    []string
	rpcIndex        int
	rpcErrorStreak  int
	maxRPCErrors    int
	rotationMu      sync.Mutex
}

// detectSDKVersion simplified: attempt staking params; if succeeds assume current
func detectSDKVersion(grpcConn *grpc.ClientConn, rpcConn string) SDKVersion {
	stakingClient := stakingtypes.NewQueryClient(grpcConn)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if res, err := stakingClient.Params(ctx, &stakingtypes.QueryParamsRequest{}); err == nil && res != nil {
		if strings.Contains(res.String(), "cosmos.staking.v1beta1") {
			log.Printf("Detected current SDK version (staking params present)")
			return SDKVersionCurrent
		}
	}
	log.Printf("Assuming current SDK version (simplified)")
	return SDKVersionCurrent
}

// recordGRPCError increments error streak and rotates endpoint if threshold exceeded
func (c *CosmosSDKCollector) recordGRPCError(err error) {
	if err == nil {
		return
	}
	c.rotationMu.Lock()
	defer c.rotationMu.Unlock()
	c.grpcErrorStreak++
	log.Printf("[gRPC] error streak=%d/%d err=%v", c.grpcErrorStreak, c.maxGRPCErrors, err)
	if c.grpcErrorStreak >= c.maxGRPCErrors && len(c.grpcEndpoints) > 1 {
		oldIdx := c.grpcIndex
		oldEp := c.grpcEndpoints[oldIdx]
		c.grpcIndex = (c.grpcIndex + 1) % len(c.grpcEndpoints)
		newEp := c.grpcEndpoints[c.grpcIndex]
		log.Printf("[gRPC] rotating endpoint error_streak=%d old=%s new=%s", c.grpcErrorStreak, oldEp, newEp)
		// attempt redial
		sanitized := strings.TrimPrefix(strings.TrimPrefix(newEp, "https://"), "http://")
		conn, dialErr := grpc.Dial(sanitized, grpc.WithTransportCredentials(insecure.NewCredentials()))
		if dialErr != nil {
			log.Printf("[gRPC] rotation dial failed endpoint=%s err=%v (keeping old connection)", newEp, dialErr)
			c.grpcIndex = oldIdx
		} else {
			if c.grpcConn != nil {
				_ = c.grpcConn.Close()
			}
			c.grpcConn = conn
			c.grpcErrorStreak = 0
		}
	}
}

// recordGRPCSuccess resets error streak on success
func (c *CosmosSDKCollector) recordGRPCSuccess() {
	c.rotationMu.Lock()
	c.grpcErrorStreak = 0
	c.rotationMu.Unlock()
}

// recordRPCError increments RPC error streak and rotates selected RPC endpoint (chainID refresh uses this)
func (c *CosmosSDKCollector) recordRPCError(err error) {
	if err == nil {
		return
	}
	c.rotationMu.Lock()
	defer c.rotationMu.Unlock()
	c.rpcErrorStreak++
	if c.rpcErrorStreak >= c.maxRPCErrors && len(c.rpcEndpoints) > 1 {
		oldIdx := c.rpcIndex
		oldEp := c.rpcEndpoints[oldIdx]
		c.rpcIndex = (c.rpcIndex + 1) % len(c.rpcEndpoints)
		newEp := c.rpcEndpoints[c.rpcIndex]
		log.Printf("[RPC] rotating endpoint error_streak=%d old=%s new=%s", c.rpcErrorStreak, oldEp, newEp)
		c.rpcErrorStreak = 0
	}
}

func (c *CosmosSDKCollector) currentRPC() string {
	c.rotationMu.Lock()
	defer c.rotationMu.Unlock()
	if len(c.rpcEndpoints) == 0 {
		return ""
	}
	return c.rpcEndpoints[c.rpcIndex]
}

func NewCosmosSDKCollector(grpcConn *grpc.ClientConn, rpcConn string, valAddress string, accAddresses []string, customDenomData types.DenomMetadata, grpcEndpoints []string, rpcEndpoints []string) CosmosSDKCollector {
	chainID, err := getChainIDWithRetry(rpcConn)
	if err != nil || chainID == "" {
		log.Printf("Could not resolve chain ID after retries: %v", err)
		chainID = "unknown-chain"
	}

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

	// rotation thresholds (env override)
	maxGRPC := 3
	if v := os.Getenv("COSMOS_EXPORTER_MAX_GRPC_ERRORS"); v != "" {
		if n, e := strconv.Atoi(v); e == nil && n > 0 {
			maxGRPC = n
		}
	}
	maxRPC := 3
	if v := os.Getenv("COSMOS_EXPORTER_MAX_RPC_ERRORS"); v != "" {
		if n, e := strconv.Atoi(v); e == nil && n > 0 {
			maxRPC = n
		}
	}

	return CosmosSDKCollector{
		grpcConn: grpcConn,
		chainID:  chainID,
		lastGoodChainID: func() string {
			if chainID != "unknown-chain" {
				return chainID
			}
			return ""
		}(),
		valAddress:       valAddress,
		accAddresses:     accAddresses,
		denomMetadata:    denomsMetadata,
		defaultBondDenom: defaultBondDenom,
		defaultMintDenom: defaultMintDenom,
		sdkVersion:       sdkVersion,
		voteMissCounts:   make(map[string]int),
		lastVoteStatus:   make(map[string]float64),
		proposalAllMiss:  make(map[uint64]int),
		grpcEndpoints:    grpcEndpoints,
		rpcEndpoints:     rpcEndpoints,
		grpcIndex:        0,
		rpcIndex:         0,
		maxGRPCErrors:    maxGRPC,
		maxRPCErrors:     maxRPC,
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
func getChainIDWithRetry(rpc string) (string, error) {
	var lastErr error
	baseDelay := 300 * time.Millisecond
	maxAttempts := 4
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		client, err := cmthttp.New(rpc, "/websocket")
		if err != nil {
			lastErr = err
			log.Printf("chainID attempt %d create client error: %v", attempt, err)
		} else {
			ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
			status, err2 := client.Status(ctx)
			cancel()
			if err2 == nil && status != nil && status.NodeInfo.Network != "" {
				return status.NodeInfo.Network, nil
			}
			lastErr = err2
			log.Printf("chainID attempt %d status error: %v", attempt, err2)
		}
		if attempt < maxAttempts {
			// exponential backoff with jitter
			delay := baseDelay * time.Duration(1<<uint(attempt-1))
			if delay > 2*time.Second {
				delay = 2 * time.Second
			}
			jitter := time.Duration(rand.Int63n(int64(delay / 3)))
			time.Sleep(delay + jitter)
		}
	}
	return "", lastErr
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
