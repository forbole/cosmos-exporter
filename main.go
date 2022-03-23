package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"

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
)

const (
	flagGRPC             = "grpc"
	flagRPC              = "rpc"
	flagSecure           = "secure"
	flagPort             = "port"
	flagValidatorAddress = "validator_address"
	flagExponent         = "exponent"
	flagDenom            = "denom"
	flagRewardAddress    = "reward_address"
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

	cmd.Flags().Uint(flagExponent, 6, "Exponent")
	cmd.Flags().String(flagDenom, "dsm", "denom_units")
	cmd.Flags().String(flagGRPC, "localhost:9090", "GRPC listen address. Port required")
	cmd.Flags().String(flagRPC, "http://localhost:26657", "RPC listen address. Port required")
	cmd.Flags().Bool(flagSecure, false, "Activate secure connections")
	cmd.Flags().String(flagPort, ":26661", "Port to be used to expose the service")
	cmd.Flags().String(flagValidatorAddress, "", "Validator address")
	cmd.Flags().String(flagRewardAddress, "", "Reward address")
	return cmd
}

func Executor(cmd *cobra.Command, args []string) error {
	// denom, _ := cmd.Flags().GetString(flagDenom)
	// exponent, _ := cmd.Flags().GetUint(flagExponent)
	gRPC, _ := cmd.Flags().GetString(flagGRPC)
	rpc, _ := cmd.Flags().GetString(flagRPC)
	rewardAddress, _ := cmd.Flags().GetString(flagRewardAddress)
	port, _ := cmd.Flags().GetString(flagPort)
	validatorAddress, _ := cmd.Flags().GetString(flagValidatorAddress)

	grpcConn, err := grpc.Dial(
		gRPC,
		grpc.WithInsecure(),
	)
	if err != nil {
		panic(err)
	}
	defer grpcConn.Close()

	chainID := getChainID(rpc)
	denomsMetadata := make(map[string]types.DenomMetadata)
	addCoinMetadata(grpcConn, denomsMetadata)
	defaultMintDenom := getMintDenom(grpcConn)
	defaultBondDenom := getBondDenom(grpcConn)

	registry := prometheus.NewPedanticRegistry()
	registry.MustRegister(
		collector.NewRewardGauge(grpcConn, rewardAddress, chainID, denomsMetadata, defaultMintDenom),
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
func addCoinMetadata(grpcConn *grpc.ClientConn, denomsMetadata map[string]types.DenomMetadata) {

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

func getMintDenom(grpcConn *grpc.ClientConn) string {
	mintClient := minttypes.NewQueryClient(grpcConn)
	mintParamsRes, err := mintClient.Params(
		context.Background(),
		&minttypes.QueryParamsRequest{},
	)
	if err != nil {
		panic(err)
	}

	return mintParamsRes.Params.MintDenom
}

func getBondDenom(grpcConn *grpc.ClientConn) string {
	stakingClient := stakingtypes.NewQueryClient(grpcConn)
	stakingParamsRes, err := stakingClient.Params(
		context.Background(),
		&stakingtypes.QueryParamsRequest{},
	)
	if err != nil {
		panic(err)
	}

	return stakingParamsRes.Params.BondDenom
}
