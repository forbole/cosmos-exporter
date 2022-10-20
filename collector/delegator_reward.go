package collector

import (
	"context"
	"log"
	"math"
	"strconv"
	"sync"

	distributiontypes "github.com/cosmos/cosmos-sdk/x/distribution/types"
)

func (collector *CosmosSDKCollector) CollectDeleatorReward() {
	var wg sync.WaitGroup
	for _, address := range collector.accAddresses {
		wg.Add(1)
		go func(address string) {
			defer wg.Done()
			distributionClient := distributiontypes.NewQueryClient(collector.grpcConn)
			distributionRes, err := distributionClient.DelegationTotalRewards(
				context.Background(),
				&distributiontypes.QueryDelegationTotalRewardsRequest{DelegatorAddress: address},
			)
			if err != nil {
				ErrorGauge.WithLabelValues("tendermint_staking_reward_total").Inc()
				log.Print(err)
				return
			}

			for _, reward := range distributionRes.Rewards {
				baseDenom, found := collector.denomMetadata[collector.defaultMintDenom]
				if !found {
					log.Print("No denom infos")
					return
				}

				if len(reward.Reward) == 0 {
					rewardfromBaseToDisplay := float64(0)
					DelegatorRewardGauge.WithLabelValues(address, reward.ValidatorAddress, collector.chainID, baseDenom.Display).Set(rewardfromBaseToDisplay)
				} else {
					for _, entry := range reward.Reward {
						var rewardfromBaseToDisplay float64
						if value, err := strconv.ParseFloat(entry.Amount.String(), 64); err != nil {
							rewardfromBaseToDisplay = 0
						} else {
							rewardfromBaseToDisplay = value / math.Pow10(int(baseDenom.Exponent))
						}
						DelegatorRewardGauge.WithLabelValues(address, reward.ValidatorAddress, collector.chainID, baseDenom.Display).Set(rewardfromBaseToDisplay)
					}
				}
			}
		}(address)
	}
	wg.Wait()
}
