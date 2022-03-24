package collector

import (
	"context"
	"math"
	"strconv"

	stakingtypes "github.com/cosmos/cosmos-sdk/x/staking/types"
	types "github.com/forbole/cosmos-exporter/types"
	"github.com/prometheus/client_golang/prometheus"
	"google.golang.org/grpc"
)

type DelegatorStakeGauge struct {
	ChainID          string
	Desc             *prometheus.Desc
	DenomMetadata    map[string]types.DenomMetadata
	DefaultBondDenom string
	GrpcConn         *grpc.ClientConn
	DelegatorAddress string
}

func NewDelegatorStakeGauge(grpcConn *grpc.ClientConn, delegatorAddress string, chainID string, denomMetadata map[string]types.DenomMetadata, defaultBondDenom string) *DelegatorStakeGauge {
	return &DelegatorStakeGauge{
		ChainID: chainID,
		Desc: prometheus.NewDesc(
			"stake_amount",
			"Stake amount of delegator address to validator",
			[]string{"delegator_address", "validator_address", "chain_id", "denom"},
			nil,
		),
		DenomMetadata:    denomMetadata,
		DefaultBondDenom: defaultBondDenom,
		GrpcConn:         grpcConn,
		DelegatorAddress: delegatorAddress,
	}
}

func (collector *DelegatorStakeGauge) Describe(ch chan<- *prometheus.Desc) {
	ch <- collector.Desc
}

func (collector *DelegatorStakeGauge) Collect(ch chan<- prometheus.Metric) {
	stakingClient := stakingtypes.NewQueryClient(collector.GrpcConn)
	stakingRes, err := stakingClient.DelegatorDelegations(
		context.Background(),
		&stakingtypes.QueryDelegatorDelegationsRequest{DelegatorAddr: collector.DelegatorAddress},
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
		displayDenom := baseDenom.Denoms[baseDenom.Display]

		var delegationFromBaseToDisplay float64
		if value, err := strconv.ParseFloat(delegation.Balance.Amount.String(), 64); err != nil {
			delegationFromBaseToDisplay = 0
		} else {
			delegationFromBaseToDisplay = value / math.Pow10(int(displayDenom.Exponent))
		}
		ch <- prometheus.MustNewConstMetric(collector.Desc, prometheus.GaugeValue, delegationFromBaseToDisplay, collector.DelegatorAddress, delegation.Delegation.ValidatorAddress, collector.ChainID, displayDenom.Denom)
	}
}
