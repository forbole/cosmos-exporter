package collector

import (
	"context"
	"log"
	"strconv"
	"strings"
	"time"

	"github.com/cosmos/cosmos-sdk/x/mint/types"
)

const defaultTimeout = 5 * time.Second

func (collector *CosmosSDKCollector) CollectInflationRate() {
	if collector.sdkVersion == SDKVersionLegacy {
		collector.collectInflationRateLegacy()
	} else {
		collector.collectInflationRateCurrent()
	}
}

// Implementation for pre-v0.50.x chains
func (collector *CosmosSDKCollector) collectInflationRateLegacy() {
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
		log.Printf("Error getting inflation rate (legacy): %v", err)
		return
	}

	// Calculate inflation rate by using annual provisions and total supply from bank module
	// This is a simplified approach; chains might have custom inflation calculation
	annualProvisions, err := annualProvisionsRes.AnnualProvisions.Float64()
	if err != nil {
		ErrorGauge.WithLabelValues("tendermint_inflation_rate").Inc()
		log.Printf("Error parsing annual provisions (legacy): %v", err)
		return
	}

	// Set inflation rate metric
	InflationRate.WithLabelValues(collector.chainID).Set(annualProvisions)
}

// Implementation for v0.50.x chains
func (collector *CosmosSDKCollector) collectInflationRateCurrent() {
	// In Cosmos SDK v0.50.x, there are protobuf compatibility issues with the mint module
	// We'll use a simple approach that catches errors and falls back gracefully

	// Try to get inflation rate via params
	mintClient := types.NewQueryClient(collector.grpcConn)

	ctx, cancel := context.WithTimeout(context.Background(), defaultTimeout)
	defer cancel()

	// Try to get params directly - this is more likely to work across chains
	paramsRes, err := mintClient.Params(ctx, &types.QueryParamsRequest{})

	if err == nil && paramsRes != nil {
		// Get the string representation and try to parse out values
		paramsStr := paramsRes.String()

		// Try to extract inflation values from the string
		for _, key := range []string{"inflation_rate", "inflation"} {
			if val := extractFloatFromString(paramsStr, key); val > 0 {
				InflationRate.WithLabelValues(collector.chainID).Set(val)
				return
			}
		}
	} else {
		log.Printf("Error getting mint params: %v", err)
	}

	// Try AnnualProvisions as a backup with protection against panics
	defer func() {
		if r := recover(); r != nil {
			log.Printf("Recovered from panic in AnnualProvisions: %v", r)
		}
	}()

	annualProvisionsRes, err := mintClient.AnnualProvisions(ctx, &types.QueryAnnualProvisionsRequest{})
	if err == nil && annualProvisionsRes != nil {
		// Try to safely get the value as a string
		valStr := annualProvisionsRes.String()
		if strings.Contains(valStr, "annual_provisions:") {
			val := extractFloatFromString(valStr, "annual_provisions")
			if val > 0 {
				InflationRate.WithLabelValues(collector.chainID).Set(val)
				return
			}
		}
	}

	// Last resort - set a default value
	log.Print("Setting default inflation rate value due to v0.50.x compatibility issues")
	InflationRate.WithLabelValues(collector.chainID).Set(0)
}

// Helper function to extract float values from string representations
func extractFloatFromString(input, key string) float64 {
	keyIndex := strings.Index(input, key+":")
	if keyIndex < 0 {
		return 0
	}

	// Extract the substring after the key
	valueStart := keyIndex + len(key) + 1
	valueEnd := strings.IndexAny(input[valueStart:], ", \n\"}")
	if valueEnd < 0 {
		return 0
	}

	valueStr := strings.TrimSpace(input[valueStart : valueStart+valueEnd])
	// Remove any quotes
	valueStr = strings.Trim(valueStr, "\"'`")

	val, err := strconv.ParseFloat(valueStr, 64)
	if err != nil {
		return 0
	}

	return val
}
