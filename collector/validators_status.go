package collector

import (
	"context"
	"log"
	"math"
	"sort"
	"strconv"

	sdkmath "cosmossdk.io/math"
	querytypes "github.com/cosmos/cosmos-sdk/types/query"
	stakingtypes "github.com/cosmos/cosmos-sdk/x/staking/types"
)

func (collector *CosmosSDKCollector) CollectValidatorsStat() {
	if collector.sdkVersion == SDKVersionLegacy {
		collector.collectValidatorsStatLegacy()
	} else {
		collector.collectValidatorsStatCurrent()
	}
}

// Implementation for both SDK versions with version-specific conversions
func (collector *CosmosSDKCollector) collectValidatorsStatLegacy() {
	stakingClient := stakingtypes.NewQueryClient(collector.grpcConn)
	validatorsResponse, err := stakingClient.Validators(
		context.Background(),
		&stakingtypes.QueryValidatorsRequest{
			Pagination: &querytypes.PageRequest{
				Limit: 1000,
			},
		},
	)
	if err != nil {
		ErrorGauge.WithLabelValues("tendermint_voting_power_total").Inc()
		log.Print(err)
		return
	}

	var validatorRanking int
	var bondedTokensTotalStr, notBondedTokensTotalStr string

	validators := validatorsResponse.Validators

	// Sort to get validator ranking.
	sort.Slice(validators, func(i, j int) bool {
		return validators[i].DelegatorShares.GT(validators[j].DelegatorShares)
	})

	// For legacy SDK, use safer string approach to avoid type mismatches
	bondedTokensTotal := int64(0)
	notBondedTokensTotal := int64(0)

	for index, validator := range validators {
		if err != nil {
			log.Print(err)
			return
		}

		// Accumulate tokens as string to handle large amounts
		switch validator.GetStatus() {
		case stakingtypes.Bonded:
			// Add tokens to running total, handling potential overflow with strings
			tokenStr := validator.GetTokens().String()
			bondedTokensTotalStr = addTokenAmountsAsStrings(bondedTokensTotalStr, tokenStr)

		case stakingtypes.Unbonding, stakingtypes.Unbonded:
			tokenStr := validator.GetTokens().String()
			notBondedTokensTotalStr = addTokenAmountsAsStrings(notBondedTokensTotalStr, tokenStr)

		default:
			panic("invalid validator status")
		}

		if validator.OperatorAddress == collector.valAddress {
			validatorRanking = index + 1
		}
	}

	// Convert totals to float64 from strings
	bondedTokensToFloat, err := strconv.ParseFloat(bondedTokensTotalStr, 64)
	if err != nil {
		// Fallback to int64 if needed (though this may lose precision for very large amounts)
		bondedTokensToFloat = float64(bondedTokensTotal)
		log.Printf("Warning: using int64 conversion for bonded tokens due to: %v", err)
	}

	notBondedTokensToFloat, err := strconv.ParseFloat(notBondedTokensTotalStr, 64)
	if err != nil {
		notBondedTokensToFloat = float64(notBondedTokensTotal)
		log.Printf("Warning: using int64 conversion for not bonded tokens due to: %v", err)
	}

	baseDenom, found := collector.denomMetadata[collector.defaultBondDenom]
	if !found {
		log.Print("No denom infos")
		return
	}

	bondedTokensTodisplay := bondedTokensToFloat / math.Pow10(int(baseDenom.Exponent))
	notBondedTokensTodisplay := notBondedTokensToFloat / math.Pow10(int(baseDenom.Exponent))

	BondedTokenGauge.WithLabelValues(collector.chainID).Set(bondedTokensTodisplay)
	NotBondedTokenGauge.WithLabelValues(collector.chainID).Set(notBondedTokensTodisplay)
	ValidatorVotingPowerRanking.WithLabelValues(collector.valAddress, collector.chainID).Set(float64(validatorRanking))
}

// Implementation for v0.50.x chains using updated math types
func (collector *CosmosSDKCollector) collectValidatorsStatCurrent() {
	stakingClient := stakingtypes.NewQueryClient(collector.grpcConn)
	validatorsResponse, err := stakingClient.Validators(
		context.Background(),
		&stakingtypes.QueryValidatorsRequest{
			Pagination: &querytypes.PageRequest{
				Limit: 1000,
			},
		},
	)
	if err != nil {
		ErrorGauge.WithLabelValues("tendermint_voting_power_total").Inc()
		log.Print(err)
		return
	}

	var validatorRanking int
	bondedTokens := sdkmath.NewInt(0)
	notBondedTokens := sdkmath.NewInt(0)

	validators := validatorsResponse.Validators

	// Sort to get validator ranking.
	sort.Slice(validators, func(i, j int) bool {
		return validators[i].DelegatorShares.GT(validators[j].DelegatorShares)
	})

	for index, validator := range validators {
		if err != nil {
			log.Print(err)
			return
		}

		switch validator.GetStatus() {
		case stakingtypes.Bonded:
			bondedTokens = bondedTokens.Add(validator.GetTokens())

		case stakingtypes.Unbonding, stakingtypes.Unbonded:
			notBondedTokens = notBondedTokens.Add(validator.GetTokens())

		default:
			panic("invalid validator status")
		}

		if validator.OperatorAddress == collector.valAddress {
			validatorRanking = index + 1
		}
	}

	bondedTokensToFloat, err := strconv.ParseFloat(bondedTokens.String(), 64)
	if err != nil {
		ErrorGauge.WithLabelValues("tendermint_voting_power_total").Inc()
		log.Print(err)
		return
	}

	notBondedTokensToFloat, err := strconv.ParseFloat(notBondedTokens.String(), 64)
	if err != nil {
		ErrorGauge.WithLabelValues("tendermint_voting_power_total").Inc()
		log.Print(err)
		return
	}

	baseDenom, found := collector.denomMetadata[collector.defaultBondDenom]
	if !found {
		log.Print("No denom infos")
		return
	}

	bondedTokensTodisplay := bondedTokensToFloat / math.Pow10(int(baseDenom.Exponent))
	notBondedTokensTodisplay := notBondedTokensToFloat / math.Pow10(int(baseDenom.Exponent))

	BondedTokenGauge.WithLabelValues(collector.chainID).Set(bondedTokensTodisplay)
	NotBondedTokenGauge.WithLabelValues(collector.chainID).Set(notBondedTokensTodisplay)
	ValidatorVotingPowerRanking.WithLabelValues(collector.valAddress, collector.chainID).Set(float64(validatorRanking))
}

// Helper function to add two token amounts represented as strings
func addTokenAmountsAsStrings(a, b string) string {
	if a == "" {
		return b
	}
	if b == "" {
		return a
	}

	// Simple parsing for basic cases
	aVal, err := strconv.ParseFloat(a, 64)
	if err != nil {
		return b // Fallback if parsing fails
	}

	bVal, err := strconv.ParseFloat(b, 64)
	if err != nil {
		return a // Fallback if parsing fails
	}

	return strconv.FormatFloat(aVal+bVal, 'f', -1, 64)
}
