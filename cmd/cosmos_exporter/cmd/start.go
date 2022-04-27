package cmd

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
	"github.com/forbole/cosmos-exporter/types"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	tmrpc "github.com/tendermint/tendermint/rpc/client/http"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
)

var (
	HTTPProtocols = regexp.MustCompile("https?://")
)

func init() {
	rootCmd.AddCommand(startCmd)
}

var startCmd = &cobra.Command{
	Use:   "start",
	Short: "Start exporting cosmos metrics",
	PreRunE: func(cmd *cobra.Command, args []string) error {
		if err := viper.ReadInConfig(); err != nil { // Handle errors reading the config file
			panic(fmt.Errorf("Fatal error config file: %w \n", err))
		}
		err := viper.Unmarshal(&config)
		if err != nil {
			return err
		}
		return nil
	},
	RunE: func(cmd *cobra.Command, args []string) error {
		var grpcOpts []grpc.DialOption

		if config.Node.IsSecure {
			grpcOpts = append(grpcOpts, grpc.WithTransportCredentials(credentials.NewTLS(&tls.Config{})))
		} else {
			grpcOpts = append(grpcOpts, grpc.WithInsecure())
		}

		address := HTTPProtocols.ReplaceAllString(config.Node.GRPC, "")
		grpcConn, err := grpc.Dial(address, grpcOpts...)
		if err != nil {
			panic(err)
		}
		defer grpcConn.Close()

		chainID := getChainID(config.Node.RPC)
		denomsMetadata := make(map[string]types.DenomMetadata)
		addDenomsMetadata(grpcConn, denomsMetadata)
		addCustomDenomMetadata(config.DenomMetadata, denomsMetadata)

		var defaultMintDenom string
		var defaultBondDenom string
		if denom, err := getMintDenom(grpcConn); err != nil {
			defaultMintDenom = config.DenomMetadata.Base
		} else {
			defaultMintDenom = denom
		}
		if denom, err := getBondDenom(grpcConn); err != nil {
			defaultBondDenom = config.DenomMetadata.Base
		} else {
			defaultBondDenom = denom
		}

		registry := prometheus.NewPedanticRegistry()
		registry.MustRegister(
			collector.NewActiveProposalGauge(grpcConn, config.DelegatorAddress, chainID),
			collector.NewDelegatorRewardGauge(grpcConn, config.DelegatorAddress, chainID, denomsMetadata, defaultMintDenom),
			collector.NewDelegatorStakeGauge(grpcConn, config.DelegatorAddress, chainID, denomsMetadata, defaultBondDenom),
			collector.NewValidatorCommissionGauge(grpcConn, config.ValidatorAddress, chainID, denomsMetadata),
			collector.NewValidatorDelegationGauge(grpcConn, config.ValidatorAddress, chainID),
			collector.NewValidatorStatus(grpcConn, config.ValidatorAddress, chainID, denomsMetadata, defaultBondDenom),
			collector.NewValidatorsStatus(grpcConn, config.ValidatorAddress, chainID, denomsMetadata, defaultBondDenom),
		)

		handler := promhttp.HandlerFor(registry, promhttp.HandlerOpts{
			ErrorLog:      log.New(os.Stderr, log.Prefix(), log.Flags()),
			ErrorHandling: promhttp.ContinueOnError,
		})

		http.Handle("/metrics", handler)
		log.Fatal(http.ListenAndServe(config.Port, nil))
		fmt.Printf("Start listening on port %s", config.Port)
		return nil
	},
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
	if !cfgDenom.IsStructureEmpty() {
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
