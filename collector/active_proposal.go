package collector

import (
	"context"
	"log"
	"sync"

	govtypes "github.com/cosmos/cosmos-sdk/x/gov/types"
)

func (collector *CosmosSDKCollector) CollectActiveProposal() {
	govClient := govtypes.NewQueryClient(collector.grpcConn)
	govRes, err := govClient.Proposals(
		context.Background(),
		&govtypes.QueryProposalsRequest{
			ProposalStatus: govtypes.StatusVotingPeriod,
		},
	)

	if err != nil {
		ErrorGauge.WithLabelValues("tendermint_active_proposals_total").Inc()
		log.Print(err)
		return
	}

	// Count proposals base on TypeUrl
	countProposalType := make(map[string]float64)
	for _, proposal := range govRes.Proposals {
		countProposalType[proposal.Content.TypeUrl] += 1
	}

	for key, total := range countProposalType {
		ActiveProposalGauge.WithLabelValues(collector.chainID, key).Set(float64(total))
	}

	// Voted active proposal
	var wg sync.WaitGroup

	for _, address := range collector.accAddresses {
		wg.Add(1)
		go func(address string) {
			defer wg.Done()
			votedGovRes, err := govClient.Proposals(
				context.Background(),
				&govtypes.QueryProposalsRequest{
					ProposalStatus: govtypes.StatusVotingPeriod,
					Voter:          address,
				},
			)

			if err != nil {
				ErrorGauge.WithLabelValues("tendermint_active_proposals_total").Inc()
				log.Print(err)
				return
			}

			VotedActiveProposalGauge.WithLabelValues(collector.chainID, address).Set(float64(float64(len(votedGovRes.Proposals))))
		}(address)
	}
	wg.Wait()
}
