package collector

import (
	"context"
	"math"
	"strconv"

	distributiontypes "github.com/cosmos/cosmos-sdk/x/distribution/types"
	types "github.com/forbole/cosmos-exporter/types"
	"github.com/prometheus/client_golang/prometheus"
	"google.golang.org/grpc"
)

type DelegatorRewardGauge struct {
	ChainID          string
	Desc             *prometheus.Desc
	DenomMetadata    map[string]types.DenomMetadata
	DefaultMintDenom string
	GrpcConn         *grpc.ClientConn
	DelegatorAddress string
}

func NewDelegatorRewardGauge(grpcConn *grpc.ClientConn, delegatorAddress string, chainID string, denomMetadata map[string]types.DenomMetadata, defaultMintDenom string) *DelegatorRewardGauge {
	return &DelegatorRewardGauge{
		ChainID: chainID,
		Desc: prometheus.NewDesc(
			"reward_amount",
			"Rewards of the delegator address from validator",
			[]string{"delegator_address", "validator_address", "chain_id", "denom"},
			nil,
		),
		DenomMetadata:    denomMetadata,
		DefaultMintDenom: defaultMintDenom,
		GrpcConn:         grpcConn,
		DelegatorAddress: delegatorAddress,
	}
}

func (collector *DelegatorRewardGauge) Describe(ch chan<- *prometheus.Desc) {
	ch <- collector.Desc
}

func (collector *DelegatorRewardGauge) Collect(ch chan<- prometheus.Metric) {
	distributionClient := distributiontypes.NewQueryClient(collector.GrpcConn)
	distributionRes, err := distributionClient.DelegationTotalRewards(
		context.Background(),
		&distributiontypes.QueryDelegationTotalRewardsRequest{DelegatorAddress: collector.DelegatorAddress},
	)
	if err != nil {
		ch <- prometheus.NewInvalidMetric(collector.Desc, err)
		return
	}

	for _, reward := range distributionRes.Rewards {
		baseDenom, found := collector.DenomMetadata[collector.DefaultMintDenom]
		if !found {
			ch <- prometheus.NewInvalidMetric(collector.Desc, &types.DenomNotFound{})
			return
		}

		if len(reward.Reward) == 0 {
			rewardfromBaseToDisplay := float64(0)
			ch <- prometheus.MustNewConstMetric(collector.Desc, prometheus.GaugeValue, rewardfromBaseToDisplay, collector.DelegatorAddress, reward.ValidatorAddress, collector.ChainID, baseDenom.Display)
		} else {
			for _, entry := range reward.Reward {
				var rewardfromBaseToDisplay float64
				if value, err := strconv.ParseFloat(entry.Amount.String(), 64); err != nil {
					rewardfromBaseToDisplay = 0
				} else {
					rewardfromBaseToDisplay = value / math.Pow10(int(baseDenom.Exponent))
				}
				ch <- prometheus.MustNewConstMetric(collector.Desc, prometheus.GaugeValue, rewardfromBaseToDisplay, collector.DelegatorAddress, reward.ValidatorAddress, collector.ChainID, baseDenom.Display)
			}
		}
	}
}
