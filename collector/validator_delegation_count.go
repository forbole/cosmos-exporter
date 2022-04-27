package collector

import (
	"context"
	"math"

	querytypes "github.com/cosmos/cosmos-sdk/types/query"
	stakingtypes "github.com/cosmos/cosmos-sdk/x/staking/types"
	"github.com/prometheus/client_golang/prometheus"
	"google.golang.org/grpc"
)

type ValidatorDelegationGauge struct {
	ChainID          string
	Desc             *prometheus.Desc
	GrpcConn         *grpc.ClientConn
	ValidatorAddress string
}

const MaxLimit = math.MaxUint64

func NewValidatorDelegationGauge(grpcConn *grpc.ClientConn, validatorAddress string, chainID string) *ValidatorDelegationGauge {
	return &ValidatorDelegationGauge{
		GrpcConn:         grpcConn,
		ValidatorAddress: validatorAddress,
		ChainID:          chainID,
		Desc: prometheus.NewDesc(
			"tendermint_validator_delegators_total",
			"Number of delegators to the validator",
			[]string{"validator_address", "chain_id"},
			nil,
		),
	}
}

func (collector *ValidatorDelegationGauge) Describe(ch chan<- *prometheus.Desc) {
	ch <- collector.Desc
}

func (collector *ValidatorDelegationGauge) Collect(ch chan<- prometheus.Metric) {
	stakingClient := stakingtypes.NewQueryClient(collector.GrpcConn)
	stakingRes, err := stakingClient.ValidatorDelegations(
		context.Background(),
		&stakingtypes.QueryValidatorDelegationsRequest{
			ValidatorAddr: collector.ValidatorAddress,
			Pagination: &querytypes.PageRequest{
				CountTotal: true,
			},
		},
	)
	if err != nil {
		ch <- prometheus.NewInvalidMetric(collector.Desc, err)
		return
	}

	delegationsCount := float64(stakingRes.Pagination.Total)

	ch <- prometheus.MustNewConstMetric(collector.Desc, prometheus.GaugeValue, delegationsCount, collector.ValidatorAddress, collector.ChainID)
}
