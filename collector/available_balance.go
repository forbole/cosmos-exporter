package collector

import (
	"context"
	"math"
	"strconv"
	"sync"

	querytypes "github.com/cosmos/cosmos-sdk/types/query"
	banktypes "github.com/cosmos/cosmos-sdk/x/bank/types"
	types "github.com/forbole/cosmos-exporter/types"
	"github.com/prometheus/client_golang/prometheus"
	"google.golang.org/grpc"
)

type AvailableBalanceGauge struct {
	ChainID            string
	Desc               *prometheus.Desc
	DenomMetadata      map[string]types.DenomMetadata
	GrpcConn           *grpc.ClientConn
	DelegatorAddresses []string
}

func NewAvailableBalanceGauge(grpcConn *grpc.ClientConn, delegatorAddresses []string, chainID string, denomMetadata map[string]types.DenomMetadata) *AvailableBalanceGauge {
	return &AvailableBalanceGauge{
		ChainID: chainID,
		Desc: prometheus.NewDesc(
			"tendermint_available_balance",
			"Stake amount of delegator address to validator",
			[]string{"delegator_address", "chain_id", "denom"},
			nil,
		),
		DenomMetadata:      denomMetadata,
		GrpcConn:           grpcConn,
		DelegatorAddresses: delegatorAddresses,
	}
}

func (collector *AvailableBalanceGauge) Describe(ch chan<- *prometheus.Desc) {
	ch <- collector.Desc
}

func (collector *AvailableBalanceGauge) Collect(ch chan<- prometheus.Metric) {
	var wg sync.WaitGroup
	for _, address := range collector.DelegatorAddresses {
		wg.Add(1)
		go func(address string) {
			defer wg.Done()
			bankClient := banktypes.NewQueryClient(collector.GrpcConn)
			bankRes, err := bankClient.AllBalances(
				context.Background(),
				&banktypes.QueryAllBalancesRequest{
					Address: address,
					Pagination: &querytypes.PageRequest{
						Limit: 1000,
					},
				},
			)
			if err != nil {
				ch <- prometheus.NewInvalidMetric(collector.Desc, err)
				return
			}

			for _, balance := range bankRes.Balances {
				baseDenom, found := collector.DenomMetadata[balance.Denom]
				if !found {
					ch <- prometheus.NewInvalidMetric(collector.Desc, &types.DenomNotFound{})
					return
				}

				var balanceFromBaseToDisPlay float64
				if value, err := strconv.ParseFloat(balance.Amount.String(), 64); err != nil {
					balanceFromBaseToDisPlay = 0
				} else {
					balanceFromBaseToDisPlay = value / math.Pow10(int(baseDenom.Exponent))
				}
				ch <- prometheus.MustNewConstMetric(collector.Desc, prometheus.GaugeValue, balanceFromBaseToDisPlay, address, collector.ChainID, baseDenom.Display)
			}
		}(address)
	}
	wg.Wait()
}
