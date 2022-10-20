package collector

import (
	"context"
	"log"
	"math"
	"strconv"
	"sync"

	querytypes "github.com/cosmos/cosmos-sdk/types/query"
	banktypes "github.com/cosmos/cosmos-sdk/x/bank/types"
)

func (collector *CosmosSDKCollector) CollectAvailableBalance() {
	var wg sync.WaitGroup
	for _, address := range collector.accAddresses {
		wg.Add(1)
		go func(address string) {
			defer wg.Done()
			bankClient := banktypes.NewQueryClient(collector.grpcConn)
			bankRes, err := bankClient.AllBalances(
				context.Background(),
				&banktypes.QueryAllBalancesRequest{
					Address: address,
					Pagination: &querytypes.PageRequest{
						Limit: 1000,
					},
				},
			)
			if err != nil {
				ErrorGauge.WithLabelValues("tendermint_available_balance").Inc()
				log.Print(err)
				return
			}

			for _, balance := range bankRes.Balances {
				baseDenom, found := collector.denomMetadata[balance.Denom]
				if !found {
					log.Print("No denom infos")
					continue
				}

				var balanceFromBaseToDisPlay float64
				if value, err := strconv.ParseFloat(balance.Amount.String(), 64); err != nil {
					balanceFromBaseToDisPlay = 0
				} else {
					balanceFromBaseToDisPlay = value / math.Pow10(int(baseDenom.Exponent))
				}
				AvailableBalanceGauge.WithLabelValues(collector.chainID, address, baseDenom.Display).Set(balanceFromBaseToDisPlay)
			}
		}(address)
	}
	wg.Wait()
}
