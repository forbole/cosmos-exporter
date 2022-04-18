package main

import (
	"context"
	"crypto/tls"
	"fmt"
	"log"
	"net/http"
	"os"
	"regexp"

	querytypes "github.com/cosmos/cosmos-sdk/types/query"
	banktypes "github.com/cosmos/cosmos-sdk/x/bank/types"
	minttypes "github.com/cosmos/cosmos-sdk/x/mint/types"
	stakingtypes "github.com/cosmos/cosmos-sdk/x/staking/types"
	"github.com/forbole/cosmos-exporter/collector"
	types "github.com/forbole/cosmos-exporter/types"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/spf13/cobra"
	tmrpc "github.com/tendermint/tendermint/rpc/client/http"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
)

const (
	flagGRPC             = "grpc"
	flagRPC              = "rpc"
	flagSecure           = "secure"
	flagPort             = "port"
	flagValidatorAddress = "validator_address"
	flagExponent         = "exponent"
	flagBaseDenom        = "base_denom"
	flagDisplayDenom     = "display_denom"
	flagDelegatorAddress = "delegator_address"
)

var (
	HTTPProtocols = regexp.MustCompile("https?://")
)

func main() {
	cmd := NewRootCommand()
	err := cmd.Execute()
	if err != nil {
		os.Exit(1)
	}
}

func NewRootCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "cosmos-exporter",
		Short: "Export validator's voting power, rewards and jailed status",
		RunE:  Executor,
	}

	cmd.Flags().Uint32(flagExponent, 0, "Exponent")
	cmd.Flags().String(flagBaseDenom, "", "base denom unit")
	cmd.Flags().String(flagDisplayDenom, "", "display denom unit")
	cmd.Flags().String(flagGRPC, "localhost:9090", "GRPC listen address. Port required")
	cmd.Flags().String(flagRPC, "http://localhost:26657", "RPC listen address. Port required")
	cmd.Flags().Bool(flagSecure, false, "Activate secure connections")
	cmd.Flags().String(flagPort, ":26661", "Port to be used to expose the service")
	cmd.Flags().String(flagValidatorAddress, "", "Validator address")
	cmd.Flags().String(flagDelegatorAddress, "", "Delegator address")
	return cmd
}

func Executor(cmd *cobra.Command, args []string) error {
	baseDenom, _ := cmd.Flags().GetString(flagBaseDenom)
	displayDenom, _ := cmd.Flags().GetString(flagDisplayDenom)
	exponent, _ := cmd.Flags().GetUint32(flagExponent)
	gRPC, _ := cmd.Flags().GetString(flagGRPC)
	rpc, _ := cmd.Flags().GetString(flagRPC)
	delegatorAddress, _ := cmd.Flags().GetString(flagDelegatorAddress)
	port, _ := cmd.Flags().GetString(flagPort)
	validatorAddress, _ := cmd.Flags().GetString(flagValidatorAddress)
	secure, _ := cmd.Flags().GetBool(flagSecure)

	var grpcOpts []grpc.DialOption

	if secure {
		grpcOpts = append(grpcOpts, grpc.WithTransportCredentials(credentials.NewTLS(&tls.Config{})))
	} else {
		grpcOpts = append(grpcOpts, grpc.WithInsecure())
	}

	address := HTTPProtocols.ReplaceAllString(gRPC, "")
	grpcConn, err := grpc.Dial(address, grpcOpts...)
	if err != nil {
		panic(err)
	}
	defer grpcConn.Close()

	chainID := getChainID(rpc)
	denomsMetadata := make(map[string]types.DenomMetadata)
	addDenomsMetadata(grpcConn, denomsMetadata)
	addCustomDenomMetadata(baseDenom, displayDenom, exponent, denomsMetadata)

	var defaultMintDenom string
	var defaultBondDenom string
	if denom, err := getMintDenom(grpcConn); err != nil {
		defaultMintDenom = baseDenom
	} else {
		defaultMintDenom = denom
	}
	if denom, err := getBondDenom(grpcConn); err != nil {
		defaultBondDenom = baseDenom
	} else {
		defaultBondDenom = denom
	}

	registry := prometheus.NewPedanticRegistry()
	registry.MustRegister(
		collector.NewActiveProposalGauge(grpcConn, delegatorAddress, chainID),
		collector.NewDelegatorRewardGauge(grpcConn, delegatorAddress, chainID, denomsMetadata, defaultMintDenom),
		collector.NewDelegatorStakeGauge(grpcConn, delegatorAddress, chainID, denomsMetadata, defaultBondDenom),
		collector.NewValidatorCommissionGauge(grpcConn, validatorAddress, chainID, denomsMetadata),
		collector.NewValidatorDelegationGauge(grpcConn, validatorAddress, chainID),
		collector.NewValidatorStatus(grpcConn, validatorAddress, chainID, denomsMetadata, defaultBondDenom),
		collector.NewValidatorsStatus(grpcConn, validatorAddress, chainID, denomsMetadata, defaultBondDenom),
	)

	handler := promhttp.HandlerFor(registry, promhttp.HandlerOpts{
		ErrorLog:      log.New(os.Stderr, log.Prefix(), log.Flags()),
		ErrorHandling: promhttp.ContinueOnError,
	})

	http.Handle("/metrics", handler)
	log.Fatal(http.ListenAndServe(port, nil))
	fmt.Printf("Start listening on port %s", port)

	return nil
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
		denoms := make(map[string]types.DenomUnit)
		for _, denom := range metadata.DenomUnits {
			denom := types.NewDenomUnit(denom.Denom, denom.Exponent)
			denoms[denom.Denom] = denom
		}
		denomsMetadata[metadata.Base] = types.NewDenomMetadata(metadata.Base, metadata.Display, denoms)
	}
}

// In some chains, DenomsMetadata request return empty so needs to add manually
func addCustomDenomMetadata(baseDenom string, displayDenom string, exponent uint32, denomsMetadata map[string]types.DenomMetadata) {
	if baseDenom != "" && displayDenom != "" && exponent > 0 {
		denoms := make(map[string]types.DenomUnit)
		denomUnit := types.NewDenomUnit(displayDenom, exponent)
		denoms[displayDenom] = denomUnit
		denomsMetadata[baseDenom] = types.NewDenomMetadata(baseDenom, displayDenom, denoms)
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
