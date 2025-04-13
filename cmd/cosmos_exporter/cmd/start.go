package cmd

import (
	"crypto/tls"
	"fmt"
	"log"
	"net/http"
	"regexp"
	"time"

	"github.com/forbole/cosmos-exporter/collector"
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
			grpcOpts = append(grpcOpts, grpc.WithTransportCredentials(credentials.NewTLS(&tls.Config{
				InsecureSkipVerify: false,
			})))
		} else {
			grpcOpts = append(grpcOpts, grpc.WithTransportCredentials(insecure.NewCredentials()))
		}

		address := HTTPProtocols.ReplaceAllString(config.Node.GRPC, "")
		grpcConn, err := grpc.Dial(address, grpcOpts...)
		if err != nil {
			panic(err)
		}
		defer grpcConn.Close()

		cosmosSDKCollector := collector.NewCosmosSDKCollector(grpcConn, config.Node.RPC, config.ValidatorAddress, config.DelegatorAddresses, config.DenomMetadata)
		go func() {
			for {
				cosmosSDKCollector.CollectChainMetrics()
				time.Sleep(10 * time.Minute)
			}
		}()
		http.Handle("/metrics", promhttp.Handler())
		log.Printf("Start listening on port %s", config.Port)
		log.Fatal(http.ListenAndServe(config.Port, nil))
		return nil
	},
}
