package collector

import (
	"context"
	"math"
	"sort"

	querytypes "github.com/cosmos/cosmos-sdk/types/query"
	stakingtypes "github.com/cosmos/cosmos-sdk/x/staking/types"
	types "github.com/forbole/cosmos-exporter/types"
	"github.com/prometheus/client_golang/prometheus"
	"google.golang.org/grpc"
)

type ValidatorsStatus struct {
	ChainID          string
	DenomMetadata    map[string]types.DenomMetadata
	DefaultBondDenom string
	GrpcConn         *grpc.ClientConn
	TotalVotingDesc  *prometheus.Desc
	ValidatorAddress string
	ValidatorRanking *prometheus.Desc
}

func NewValidatorsStatus(grpcConn *grpc.ClientConn, validatorAddress string, chainID string, denomMetadata map[string]types.DenomMetadata, defaultBondDenom string) *ValidatorsStatus {
	return &ValidatorsStatus{
		ChainID:          chainID,
		DenomMetadata:    denomMetadata,
		DefaultBondDenom: defaultBondDenom,
		GrpcConn:         grpcConn,
		TotalVotingDesc: prometheus.NewDesc(
			"total_voting_power",
			"Total voting power of validators",
			[]string{"chain_id", "denom"},
			nil,
		),
		ValidatorAddress: validatorAddress,
		ValidatorRanking: prometheus.NewDesc(
			"validator_voting_power_ranking",
			"Ranking of the validator based on voting power",
			[]string{"chain_id"},
			nil,
		),
	}
}

func (collector *ValidatorsStatus) Describe(ch chan<- *prometheus.Desc) {
	ch <- collector.TotalVotingDesc
	ch <- collector.ValidatorRanking
}

func (collector *ValidatorsStatus) Collect(ch chan<- prometheus.Metric) {
	stakingClient := stakingtypes.NewQueryClient(collector.GrpcConn)
	validatorsResponse, err := stakingClient.Validators(
		context.Background(),
		&stakingtypes.QueryValidatorsRequest{
			Pagination: &querytypes.PageRequest{
				Limit: 1000,
			},
		},
	)
	if err != nil {
		ch <- prometheus.NewInvalidMetric(collector.TotalVotingDesc, err)
		return
	}

	var totalVotingPower float64
	var validatorRanking int
	validators := validatorsResponse.Validators

	// Sort to get validator ranking.
	sort.Slice(validators, func(i, j int) bool {
		return validators[i].DelegatorShares.GT(validators[j].DelegatorShares)
	})

	for index, validator := range validators {
		votingPower, err := validator.DelegatorShares.Float64()
		if err != nil {
			panic(err)
		}
		totalVotingPower += votingPower
		if validator.OperatorAddress == collector.ValidatorAddress {
			validatorRanking = index + 1
		}
	}

	baseDenom, found := collector.DenomMetadata[collector.DefaultBondDenom]
	if !found {
		ch <- prometheus.NewInvalidMetric(collector.TotalVotingDesc, &types.DenomNotFound{})
		return
	}
	fromBaseToDisplay := totalVotingPower / math.Pow10(int(baseDenom.Exponent))

	ch <- prometheus.MustNewConstMetric(collector.TotalVotingDesc, prometheus.GaugeValue, fromBaseToDisplay, collector.ChainID, baseDenom.Display)
	ch <- prometheus.MustNewConstMetric(collector.ValidatorRanking, prometheus.GaugeValue, float64(validatorRanking), collector.ChainID)
}
