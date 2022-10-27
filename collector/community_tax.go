package collector

import (
	"context"
	"log"

	distributiontypes "github.com/cosmos/cosmos-sdk/x/distribution/types"
)

func (collector *CosmosSDKCollector) CollectCommunityTax() {
	distributionClient := distributiontypes.NewQueryClient(collector.grpcConn)
	distributionRes, err := distributionClient.Params(
		context.Background(),
		&distributiontypes.QueryParamsRequest{},
	)
	if err != nil {
		ErrorGauge.WithLabelValues("&distributiontypes.").Inc()
		log.Print(err)
		return
	}

	CommunityTax.WithLabelValues(collector.chainID).Set(distributionRes.Params.CommunityTax.MustFloat64())
}
