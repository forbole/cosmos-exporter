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

type ValidatorCommissionGauge struct {
	ChainID          string
	Desc             *prometheus.Desc
	DenomMetadata    map[string]types.DenomMetadata
	GrpcConn         *grpc.ClientConn
	ValidatorAddress string
}

func NewValidatorCommissionGauge(grpcConn *grpc.ClientConn, validatorAddress string, chainID string, denomMetadata map[string]types.DenomMetadata) *ValidatorCommissionGauge {
	return &ValidatorCommissionGauge{
		ChainID: chainID,
		Desc: prometheus.NewDesc(
			"tendermint_validator_commission_total",
			"Commission of the validator",
			[]string{"validator_address", "chain_id", "denom"},
			nil,
		),
		DenomMetadata:    denomMetadata,
		GrpcConn:         grpcConn,
		ValidatorAddress: validatorAddress,
	}
}

func (collector *ValidatorCommissionGauge) Describe(ch chan<- *prometheus.Desc) {
	ch <- collector.Desc
}

func (collector *ValidatorCommissionGauge) Collect(ch chan<- prometheus.Metric) {
	distributionClient := distributiontypes.NewQueryClient(collector.GrpcConn)
	distributionRes, err := distributionClient.ValidatorCommission(
		context.Background(),
		&distributiontypes.QueryValidatorCommissionRequest{ValidatorAddress: collector.ValidatorAddress},
	)
	if err != nil {
		ch <- prometheus.NewInvalidMetric(collector.Desc, err)
		return
	}

	for _, commission := range distributionRes.Commission.Commission {
		if value, err := strconv.ParseFloat(commission.Amount.String(), 64); err != nil {
			ch <- prometheus.NewInvalidMetric(collector.Desc, err)
		} else {
			baseDenom, found := collector.DenomMetadata[commission.Denom]
			if !found {
				continue
			}
			commissionFromBaseToDisplay := value / math.Pow10(int(baseDenom.Exponent))

			ch <- prometheus.MustNewConstMetric(collector.Desc, prometheus.GaugeValue, commissionFromBaseToDisplay, collector.ValidatorAddress, collector.ChainID, baseDenom.Display)
		}
	}

}
