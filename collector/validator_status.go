package collector

import (
	"context"
	"log"
	"math"
	"strconv"

	stakingtypes "github.com/cosmos/cosmos-sdk/x/staking/types"
)

func (collector *CosmosSDKCollector) CollectValidatorStat() {
	stakingClient := stakingtypes.NewQueryClient(collector.grpcConn)
	validator, err := stakingClient.Validator(
		context.Background(),
		&stakingtypes.QueryValidatorRequest{ValidatorAddr: collector.valAddress},
	)
	if err != nil {
		log.Print(err)
		return
	}

	// Jail handle
	var jailed float64
	if validator.Validator.Jailed {
		jailed = 1
	} else {
		jailed = 0
	}
	ValidatorJailStatusGauge.WithLabelValues(collector.valAddress, collector.chainID).Set(jailed)

	// Commission rate handle
	if rate, err := strconv.ParseFloat(validator.Validator.Commission.CommissionRates.Rate.String(), 64); err != nil {
	} else {
		ValidatorCommissionRateGauge.WithLabelValues(collector.valAddress, collector.chainID).Set(rate)
	}

	// Voting power handle

	if value, err := strconv.ParseFloat(validator.Validator.DelegatorShares.String(), 64); err != nil {
	} else {
		baseDenom, found := collector.denomMetadata[collector.defaultBondDenom]
		if !found {
			log.Print("No denom infos")
			return
		}
		fromBaseToDisplay := value / math.Pow10(int(baseDenom.Exponent))
		ValidatorVotingPowerGauge.WithLabelValues(collector.valAddress, collector.chainID, baseDenom.Display).Set(fromBaseToDisplay)
	}

}
