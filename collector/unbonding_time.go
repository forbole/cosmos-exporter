package collector

import (
	"context"
	"log"

	stakingtypes "github.com/cosmos/cosmos-sdk/x/staking/types"
)

func (collector *CosmosSDKCollector) CollectUnbondingTime() {
	stakeClient := stakingtypes.NewQueryClient(collector.grpcConn)
	stakeRes, err := stakeClient.Params(
		context.Background(),
		&stakingtypes.QueryParamsRequest{},
	)
	if err != nil {
		collector.recordGRPCError(err)
		ErrorGauge.WithLabelValues("tendermint_unbonding_time").Inc()
		log.Printf("Error getting staking params for unbonding time: %v", err)
		return
	}

	collector.recordGRPCSuccess()
	// UnbondingTime is a time.Duration value type, not a pointer - always has a value
	UnbondingTime.WithLabelValues(collector.chainID).Set(stakeRes.Params.UnbondingTime.Seconds())
}
