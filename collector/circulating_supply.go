package collector

import (
	"context"
	"log"
	"math"
	"strconv"

	banktypes "github.com/cosmos/cosmos-sdk/x/bank/types"
)

func (collector *CosmosSDKCollector) CollectCirculatingSupply() {
	if collector.sdkVersion == SDKVersionLegacy {
		collector.collectCirculatingSupplyLegacy()
	} else {
		collector.collectCirculatingSupplyCurrent()
	}
}

// Implementation for pre-v0.50.x chains
func (collector *CosmosSDKCollector) collectCirculatingSupplyLegacy() {
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

// Implementation for v0.50.x chains
func (collector *CosmosSDKCollector) collectCirculatingSupplyCurrent() {
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

	// Use string conversion to handle large token amounts safely
	supplyValue, err := strconv.ParseFloat(bankRes.Amount.Amount.String(), 64)
	if err != nil {
		ErrorGauge.WithLabelValues("tendermint_circulating_supply").Inc()
		log.Print(err)
		return
	}

	SupplyFromBaseToDisplay := supplyValue / math.Pow10(int(baseDenom.Exponent))

	CirculatingSupply.WithLabelValues(collector.chainID).Set(SupplyFromBaseToDisplay)
}
