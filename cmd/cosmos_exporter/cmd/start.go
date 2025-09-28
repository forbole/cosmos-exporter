package cmd

import (
	"context"
	"crypto/tls"
	"fmt"
	"log"
	"net/http"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	cmthttp "github.com/cometbft/cometbft/rpc/client/http"
	stakingtypes "github.com/cosmos/cosmos-sdk/x/staking/types"
	"github.com/forbole/cosmos-exporter/collector"
	"github.com/forbole/cosmos-exporter/registry"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
)

var (
	HTTPProtocols = regexp.MustCompile("https?://")
)

func init() {
	rootCmd.AddCommand(startCmd)
	startCmd.Flags().String("chain-name", "", "Optional chain-registry chain name for fallback RPC/GRPC endpoints")
	startCmd.Flags().String("chain-registry-path", "./chain-registry", "Path to local chain-registry root directory")
	startCmd.Flags().String("listen-address", "", "Optional listen address (e.g. :26647) overriding config.Port")
	startCmd.Flags().String("scrape-interval", "10m", "Scrape interval (e.g. 30s, 1m, 5m)")
}

var startCmd = &cobra.Command{
	Use:   "start",
	Short: "Start exporting cosmos metrics",
	PreRunE: func(cmd *cobra.Command, args []string) error {
		if err := viper.ReadInConfig(); err != nil { // Handle errors reading the config file
			panic(fmt.Errorf("fatal error config file: %w", err))
		}
		err := viper.Unmarshal(&config)
		if err != nil {
			return err
		}
		return nil
	},
	RunE: func(cmd *cobra.Command, args []string) error {
		chainName, _ := cmd.Flags().GetString("chain-name")
		registryPath, _ := cmd.Flags().GetString("chain-registry-path")
		listenAddrFlag, _ := cmd.Flags().GetString("listen-address")
		scrapeIntervalStr, _ := cmd.Flags().GetString("scrape-interval")
		scrapeInterval, err := time.ParseDuration(scrapeIntervalStr)
		if err != nil {
			return fmt.Errorf("invalid --scrape-interval value: %v", err)
		}

		var grpcOpts []grpc.DialOption

		if config.Node.IsSecure {
			grpcOpts = append(grpcOpts, grpc.WithTransportCredentials(credentials.NewTLS(&tls.Config{
				InsecureSkipVerify: false,
			})))
		} else {
			grpcOpts = append(grpcOpts, grpc.WithTransportCredentials(insecure.NewCredentials()))
		}

		candidateGRPC := []string{}
		candidateRPC := []string{}

		// precedence: config first
		if config.Node.GRPC != "" {
			candidateGRPC = append(candidateGRPC, config.Node.GRPC)
		}
		if config.Node.RPC != "" {
			candidateRPC = append(candidateRPC, config.Node.RPC)
		}

		if chainName != "" {
			base, _ := filepath.Abs(registryPath)
			chainData, err := registry.LoadChainRegistryChain(base, chainName)
			if err == nil {
				for _, r := range chainData.APIs.GRPC {
					if r.Address != "" {
						candidateGRPC = append(candidateGRPC, r.Address)
					}
				}
				for _, r := range chainData.APIs.RPC {
					if r.Address != "" {
						candidateRPC = append(candidateRPC, r.Address)
					}
				}
			} else {
				log.Printf("chain registry load failed path=%s chain=%s err=%v", base, chainName, err)
			}
		}

		// deduplicate while preserving order
		dedupe := func(in []string) []string {
			seen := make(map[string]struct{})
			out := []string{}
			for _, v := range in {
				v = strings.TrimSpace(v)
				if v == "" {
					continue
				}
				if _, ok := seen[v]; ok {
					continue
				}
				seen[v] = struct{}{}
				out = append(out, v)
			}
			return out
		}
		candidateGRPC = dedupe(candidateGRPC)
		candidateRPC = dedupe(candidateRPC)

		// gRPC selection with health check
		var grpcConn *grpc.ClientConn
		var dialErr error
		var selectedGRPCEndpoint string
		for i, addr := range candidateGRPC {
			log.Printf("[gRPC] attempting endpoint (%d/%d): %s", i+1, len(candidateGRPC), addr)
			sanitized := HTTPProtocols.ReplaceAllString(addr, "")
			grpcConn, dialErr = grpc.Dial(sanitized, grpcOpts...)
			if dialErr != nil {
				log.Printf("[gRPC] dial failed, fallback: %s err=%v", addr, dialErr)
				continue
			}
			hCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			stakingClient := stakingtypes.NewQueryClient(grpcConn)
			_, hErr := stakingClient.Params(hCtx, &stakingtypes.QueryParamsRequest{})
			cancel()
			if hErr != nil {
				log.Printf("[gRPC] health check failed, fallback: %s err=%v", addr, hErr)
				_ = grpcConn.Close()
				grpcConn = nil
				continue
			}
			selectedGRPCEndpoint = addr
			log.Printf("[gRPC] selected endpoint: %s", addr)
			break
		}
		if grpcConn == nil {
			return fmt.Errorf("all gRPC endpoints (dial+health) failed after %d attempts", len(candidateGRPC))
		}
		defer grpcConn.Close()

		// RPC probing with HTTPS->HTTP downgrade
		selectedRPC := ""
		if len(candidateRPC) == 0 {
			return fmt.Errorf("no RPC endpoints available (config + registry)")
		}
		probe := func(url string) bool {
			client, err := cmthttp.New(url, "/websocket")
			if err != nil {
				log.Printf("[RPC] client init failed url=%s err=%v", url, err)
				return false
			}
			ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
			_, err = client.Status(ctx)
			cancel()
			if err != nil {
				log.Printf("[RPC] status failed url=%s err=%v", url, err)
				return false
			}
			return true
		}
		for i, r := range candidateRPC {
			log.Printf("[RPC] probing endpoint (%d/%d): %s", i+1, len(candidateRPC), r)
			if probe(r) {
				selectedRPC = r
				log.Printf("[RPC] selected endpoint: %s", r)
				break
			}
			if strings.HasPrefix(r, "https://") {
				down := "http://" + strings.TrimPrefix(r, "https://")
				log.Printf("[RPC] attempting downgrade probe: %s", down)
				if probe(down) {
					selectedRPC = down
					log.Printf("[RPC] selected downgraded endpoint: %s", down)
					break
				}
			}
		}
		if selectedRPC == "" {
			return fmt.Errorf("all RPC endpoints failed after %d attempts", len(candidateRPC))
		}

		cosmosSDKCollector := collector.NewCosmosSDKCollector(
			grpcConn,
			selectedRPC,
			config.ValidatorAddress,
			config.DelegatorAddresses,
			config.DenomMetadata,
			candidateGRPC,
			candidateRPC,
		)
		go func() {
			for {
				cosmosSDKCollector.CollectChainMetrics()
				time.Sleep(scrapeInterval)
			}
		}()
		http.Handle("/metrics", promhttp.Handler())
		listenAddr := config.Port
		if listenAddrFlag != "" {
			listenAddr = listenAddrFlag
		}
		log.Printf("Start listening on %s (scrape interval=%s gRPC=%s RPC=%s)", listenAddr, scrapeInterval, selectedGRPCEndpoint, selectedRPC)
		if err := http.ListenAndServe(listenAddr, nil); err != nil {
			return err
		}
		return nil
	},
}
