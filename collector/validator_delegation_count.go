package collector

import (
	"context"
	"log"
	"math"

	querytypes "github.com/cosmos/cosmos-sdk/types/query"
	stakingtypes "github.com/cosmos/cosmos-sdk/x/staking/types"
)

const MaxLimit = math.MaxUint64

func (collector *CosmosSDKCollector) CollectValidatorDelegationGauge() {
	stakingClient := stakingtypes.NewQueryClient(collector.grpcConn)
	stakingRes, err := stakingClient.ValidatorDelegations(
		context.Background(),
		&stakingtypes.QueryValidatorDelegationsRequest{
			ValidatorAddr: collector.valAddress,
			Pagination: &querytypes.PageRequest{
				CountTotal: true,
			},
		},
	)
	if err != nil {
		ErrorGauge.WithLabelValues("tendermint_validator_delegators_total").Inc()
		log.Print(err)
		return
	}

	delegationsCount := float64(stakingRes.Pagination.Total)
	ValidatorDelegationGauge.WithLabelValues(collector.valAddress, collector.chainID).Set(delegationsCount)
}
