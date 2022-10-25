package collector

import (
	"context"
	"log"

	minttypes "github.com/cosmos/cosmos-sdk/x/mint/types"
)

func (collector *CosmosSDKCollector) CollectInflationRate() {
	mintClient := minttypes.NewQueryClient(collector.grpcConn)
	mintRes, err := mintClient.Inflation(
		context.Background(),
		&minttypes.QueryInflationRequest{},
	)
	if err != nil {
		ErrorGauge.WithLabelValues("tendermint_inflation_rate").Inc()
		log.Print(err)
		return
	}

	inflationRate, err := mintRes.Inflation.Float64()

	if err != nil {
		ErrorGauge.WithLabelValues("tendermint_inflation_rate").Inc()
		log.Print(err)
		return
	}

	InflationRate.WithLabelValues(collector.chainID).Set(inflationRate)
}
