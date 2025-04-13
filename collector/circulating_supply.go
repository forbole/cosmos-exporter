package collector

import (
	"context"
	"log"
	"math"

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

	baseDenom, found := collector.denomMetadata[collector.defaultMintDenom]
	if !found {
		log.Print("No denom infos")
		return
	}
	SupplyFromBaseToDisplay := float64(bankRes.Amount.Amount.Int64()) / math.Pow10(int(baseDenom.Exponent))

	CirculatingSupply.WithLabelValues(collector.chainID).Set(SupplyFromBaseToDisplay)
}
