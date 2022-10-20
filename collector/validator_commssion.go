package collector

import (
	"context"
	"log"
	"math"
	"strconv"

	distributiontypes "github.com/cosmos/cosmos-sdk/x/distribution/types"
)

func (collector *CosmosSDKCollector) CollectValidatorCommissionGauge() {
	distributionClient := distributiontypes.NewQueryClient(collector.grpcConn)
	distributionRes, err := distributionClient.ValidatorCommission(
		context.Background(),
		&distributiontypes.QueryValidatorCommissionRequest{ValidatorAddress: collector.valAddress},
	)
	if err != nil {
		ErrorGauge.WithLabelValues("tendermint_validator_commission_total").Inc()
		log.Print(err)
		return
	}

	for _, commission := range distributionRes.Commission.Commission {
		if value, err := strconv.ParseFloat(commission.Amount.String(), 64); err != nil {
			ErrorGauge.WithLabelValues("tendermint_validator_commission_total").Inc()
		} else {
			baseDenom, found := collector.denomMetadata[commission.Denom]
			if !found {
				continue
			}
			commissionFromBaseToDisplay := value / math.Pow10(int(baseDenom.Exponent))

			ValidatorCommissionGauge.WithLabelValues(collector.valAddress, collector.chainID, baseDenom.Display).Set(commissionFromBaseToDisplay)
		}
	}

}
