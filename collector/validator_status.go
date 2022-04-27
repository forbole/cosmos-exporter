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

type ValidatorStatus struct {
	ChainID          string
	DenomMetadata    map[string]types.DenomMetadata
	DefaultBondDenom string
	JailDesc         *prometheus.Desc
	RateDesc         *prometheus.Desc
	VotingDesc       *prometheus.Desc
	GrpcConn         *grpc.ClientConn
	ValidatorAddress string
}

func NewValidatorStatus(grpcConn *grpc.ClientConn, validatorAddress string, chainID string, denomMetadata map[string]types.DenomMetadata, defaultBondDenom string) *ValidatorStatus {
	return &ValidatorStatus{
		GrpcConn:         grpcConn,
		ValidatorAddress: validatorAddress,
		ChainID:          chainID,
		DenomMetadata:    denomMetadata,
		DefaultBondDenom: defaultBondDenom,
		JailDesc: prometheus.NewDesc(
			"validator_jailed",
			"Return 1 if the validator is jailed",
			[]string{"validator_address", "chain_id"},
			nil,
		),
		RateDesc: prometheus.NewDesc(
			"validator_commission_rate",
			"Commission rate of the validator",
			[]string{"validator_address", "chain_id"},
			nil,
		),
		VotingDesc: prometheus.NewDesc(
			"validator_voting_power",
			"Voting power of the validator",
			[]string{"validator_address", "chain_id", "denom"},
			nil,
		),
	}
}

func (collector *ValidatorStatus) Describe(ch chan<- *prometheus.Desc) {
	ch <- collector.JailDesc
	ch <- collector.RateDesc
	ch <- collector.VotingDesc
}

func (collector *ValidatorStatus) Collect(ch chan<- prometheus.Metric) {
	stakingClient := stakingtypes.NewQueryClient(collector.GrpcConn)
	validator, err := stakingClient.Validator(
		context.Background(),
		&stakingtypes.QueryValidatorRequest{ValidatorAddr: collector.ValidatorAddress},
	)
	if err != nil {
		ch <- prometheus.NewInvalidMetric(collector.JailDesc, err)
		ch <- prometheus.NewInvalidMetric(collector.RateDesc, err)
		ch <- prometheus.NewInvalidMetric(collector.VotingDesc, err)
		return
	}

	// Jail handle
	var jailed float64
	if validator.Validator.Jailed {
		jailed = 1
	} else {
		jailed = 0
	}
	ch <- prometheus.MustNewConstMetric(collector.JailDesc, prometheus.GaugeValue, jailed, collector.ValidatorAddress, collector.ChainID)

	// Rate handle

	if rate, err := strconv.ParseFloat(validator.Validator.Commission.CommissionRates.Rate.String(), 64); err != nil {
		ch <- prometheus.NewInvalidMetric(collector.RateDesc, err)
	} else {
		ch <- prometheus.MustNewConstMetric(collector.RateDesc, prometheus.GaugeValue, rate, collector.ValidatorAddress, collector.ChainID)
	}

	// Voting power handle

	if value, err := strconv.ParseFloat(validator.Validator.DelegatorShares.String(), 64); err != nil {
		ch <- prometheus.NewInvalidMetric(collector.VotingDesc, err)
	} else {
		baseDenom, found := collector.DenomMetadata[collector.DefaultBondDenom]
		if !found {
			ch <- prometheus.NewInvalidMetric(collector.VotingDesc, &types.DenomNotFound{})
			return
		}
		fromBaseToDisplay := value / math.Pow10(int(baseDenom.Exponent))

		ch <- prometheus.MustNewConstMetric(collector.VotingDesc, prometheus.GaugeValue, fromBaseToDisplay, collector.ValidatorAddress, collector.ChainID, baseDenom.Display)
	}

}
