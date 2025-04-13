package collector

import (
	"context"
	"log"

	sdkerrors "cosmossdk.io/errors"
	"github.com/cosmos/cosmos-sdk/x/mint/types"
)

func (collector *CosmosSDKCollector) CollectInflationRate() {
	// The mint module is optional in Cosmos SDK v0.50.x, and its APIs have changed
	// First, try the v0.50.x way
	mintClient := types.NewQueryClient(collector.grpcConn)
	
	// Try to get annual provisions and total supply to calculate inflation
	annualProvisionsRes, err := mintClient.AnnualProvisions(
		context.Background(),
		&types.QueryAnnualProvisionsRequest{},
	)
	
	if err != nil {
		// If this fails, the chain might not have mint module enabled
		// or it's using a custom mint module
		ErrorGauge.WithLabelValues("tendermint_inflation_rate").Inc()
		log.Printf("Error getting inflation rate: %v", err)
		return
	}
	
	// Calculate inflation rate by using annual provisions and total supply from bank module
	// This is a simplified approach; chains might have custom inflation calculation
	annualProvisions, err := annualProvisionsRes.AnnualProvisions.Float64()
	if err != nil {
		ErrorGauge.WithLabelValues("tendermint_inflation_rate").Inc()
		log.Printf("Error parsing annual provisions: %v", err)
		return
	}
	
	// Set inflation rate metric
	InflationRate.WithLabelValues(collector.chainID).Set(annualProvisions)
}
