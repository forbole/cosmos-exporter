package collector

import (
	"context"
	"log"
	"math"
	"strconv"

	banktypes "github.com/cosmos/cosmos-sdk/x/bank/types"
)

func (collector *CosmosSDKCollector) CollectCirculatingSupply() {
	bankClient := banktypes.NewQueryClient(collector.grpcConn)
	bankRes, err := bankClient.SupplyOf(
		context.Background(),
		&banktypes.QuerySupplyOfRequest{Denom: collector.defaultMintDenom},
	)
	if err != nil {
		ErrorGauge.WithLabelValues("tendermint_circulating_supply").Inc()
		log.Print(err)
		return
	}

	if value, err := strconv.ParseFloat(bankRes.Amount.String(), 64); err != nil {
		ErrorGauge.WithLabelValues("tendermint_circulating_supply").Inc()
	} else {
		baseDenom, found := collector.denomMetadata[collector.defaultMintDenom]
		if !found {
			log.Print("No denom infos")
			return
		}
		SupplyFromBaseToDisplay := value / math.Pow10(int(baseDenom.Exponent))

		CirculatingSupply.WithLabelValues(collector.chainID, baseDenom.Display).Set(SupplyFromBaseToDisplay)
	}
}
