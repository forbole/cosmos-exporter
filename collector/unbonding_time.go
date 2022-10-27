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
		ErrorGauge.WithLabelValues("tendermint_unbonding_time").Inc()
		log.Print(err)
		return
	}

	UnbondingTime.WithLabelValues(collector.chainID).Set(stakeRes.Params.UnbondingTime.Seconds())
}
