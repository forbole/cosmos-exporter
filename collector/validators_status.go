package collector

import (
	"context"
	"log"
	"math"
	"sort"
	"strconv"

	querytypes "github.com/cosmos/cosmos-sdk/types/query"
	stakingtypes "github.com/cosmos/cosmos-sdk/x/staking/types"

	cosmostypes "github.com/cosmos/cosmos-sdk/types"
)

func (collector *CosmosSDKCollector) CollectValidatorsStat() {
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
	bondedTokens := cosmostypes.ZeroInt()
	notBondedTokens := cosmostypes.ZeroInt()

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
