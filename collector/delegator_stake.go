package collector

import (
	"context"
	"log"
	"math"
	"strconv"
	"sync"

	stakingtypes "github.com/cosmos/cosmos-sdk/x/staking/types"
)

func (collector *CosmosSDKCollector) CollecDelegatorStake() {
	var wg sync.WaitGroup
	for _, address := range collector.accAddresses {
		wg.Add(1)
		go func(address string) {
			defer wg.Done()
			stakingClient := stakingtypes.NewQueryClient(collector.grpcConn)
			stakingRes, err := stakingClient.DelegatorDelegations(
				context.Background(),
				&stakingtypes.QueryDelegatorDelegationsRequest{DelegatorAddr: address},
			)
			if err != nil {
				ErrorGauge.WithLabelValues("tendermint_staking_total").Inc()
				log.Print(err)
				return
			}

			for _, delegation := range stakingRes.DelegationResponses {
				baseDenom, found := collector.denomMetadata[collector.defaultBondDenom]
				if !found {
					ErrorGauge.WithLabelValues("tendermint_staking_total").Inc()
					log.Print("No denom infos")
					return
				}

				var delegationFromBaseToDisplay float64
				if value, err := strconv.ParseFloat(delegation.Balance.Amount.String(), 64); err != nil {
					delegationFromBaseToDisplay = 0
				} else {
					delegationFromBaseToDisplay = value / math.Pow10(int(baseDenom.Exponent))
				}
				DelegatorStakeGauge.WithLabelValues(address, delegation.Delegation.ValidatorAddress, collector.chainID, baseDenom.Display).Set(delegationFromBaseToDisplay)
			}
		}(address)
	}
	wg.Wait()
}
