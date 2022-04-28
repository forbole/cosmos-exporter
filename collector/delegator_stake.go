package collector

import (
	"context"
	"math"
	"strconv"
	"sync"

	stakingtypes "github.com/cosmos/cosmos-sdk/x/staking/types"
	types "github.com/forbole/cosmos-exporter/types"
	"github.com/prometheus/client_golang/prometheus"
	"google.golang.org/grpc"
)

type DelegatorStakeGauge struct {
	ChainID            string
	Desc               *prometheus.Desc
	DenomMetadata      map[string]types.DenomMetadata
	DefaultBondDenom   string
	GrpcConn           *grpc.ClientConn
	DelegatorAddresses []string
}

func NewDelegatorStakeGauge(grpcConn *grpc.ClientConn, delegatorAddresses []string, chainID string, denomMetadata map[string]types.DenomMetadata, defaultBondDenom string) *DelegatorStakeGauge {
	return &DelegatorStakeGauge{
		ChainID: chainID,
		Desc: prometheus.NewDesc(
			"tendermint_staking_total",
			"Stake amount of delegator address to validator",
			[]string{"delegator_address", "validator_address", "chain_id", "denom"},
			nil,
		),
		DenomMetadata:      denomMetadata,
		DefaultBondDenom:   defaultBondDenom,
		GrpcConn:           grpcConn,
		DelegatorAddresses: delegatorAddresses,
	}
}

func (collector *DelegatorStakeGauge) Describe(ch chan<- *prometheus.Desc) {
	ch <- collector.Desc
}

func (collector *DelegatorStakeGauge) Collect(ch chan<- prometheus.Metric) {
	var wg sync.WaitGroup
	for _, address := range collector.DelegatorAddresses {
		wg.Add(1)
		go func(address string) {
			defer wg.Done()
			stakingClient := stakingtypes.NewQueryClient(collector.GrpcConn)
			stakingRes, err := stakingClient.DelegatorDelegations(
				context.Background(),
				&stakingtypes.QueryDelegatorDelegationsRequest{DelegatorAddr: address},
			)
			if err != nil {
				ch <- prometheus.NewInvalidMetric(collector.Desc, err)
				return
			}

			for _, delegation := range stakingRes.DelegationResponses {
				baseDenom, found := collector.DenomMetadata[collector.DefaultBondDenom]
				if !found {
					ch <- prometheus.NewInvalidMetric(collector.Desc, &types.DenomNotFound{})
					return
				}

				var delegationFromBaseToDisplay float64
				if value, err := strconv.ParseFloat(delegation.Balance.Amount.String(), 64); err != nil {
					delegationFromBaseToDisplay = 0
				} else {
					delegationFromBaseToDisplay = value / math.Pow10(int(baseDenom.Exponent))
				}
				ch <- prometheus.MustNewConstMetric(collector.Desc, prometheus.GaugeValue, delegationFromBaseToDisplay, address, delegation.Delegation.ValidatorAddress, collector.ChainID, baseDenom.Display)
			}
		}(address)
	}
	wg.Wait()
}
