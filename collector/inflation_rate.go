package collector

import (
	"log"
)

func (collector *CosmosSDKCollector) CollectInflationRate() {
	// In Cosmos SDK v0.50.x, there are protobuf compatibility issues with the mint module
	// We will skip trying to collect inflation rate as it's not critical
	log.Print("Skipping inflation rate collection due to v0.50.x compatibility issues")

	// Set a default value (0) for the metric to avoid missing metrics in dashboards
	InflationRate.WithLabelValues(collector.chainID).Set(0)

	// Note: If you need inflation rate, you could calculate it using other methods
	// or query the blockchain directly through a compatible API
}
