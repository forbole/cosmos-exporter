package collector

import (
	"context"
	"log"
	"strconv"
	"sync"

	"github.com/cosmos/cosmos-sdk/x/gov/types/v1"
	"github.com/prometheus/client_golang/prometheus"
)

func (collector *CosmosSDKCollector) CollectActiveProposal() {
	govClient := v1.NewQueryClient(collector.grpcConn)
	govRes, err := govClient.Proposals(
		context.Background(),
		&v1.QueryProposalsRequest{
			ProposalStatus: v1.StatusVotingPeriod,
		},
	)

	if err != nil {
		ErrorGauge.WithLabelValues("tendermint_active_proposals_total").Inc()
		log.Print(err)
		return
	}

	VotedActiveProposalGauge.DeletePartialMatch(
		prometheus.Labels{
			"chain_id": collector.chainID,
		},
	)

	ActiveProposalGauge.DeletePartialMatch(
		prometheus.Labels{
			"chain_id": collector.chainID,
		},
	)

	// Count proposals base on TypeUrl
	countProposalType := make(map[string]float64)
	for _, proposal := range govRes.Proposals {
		msgTypeUrl := "unknown"
		if proposal.Messages != nil && len(proposal.Messages) > 0 {
			msgTypeUrl = proposal.Messages[0].TypeUrl
		}
		countProposalType[msgTypeUrl] += 1
		
		// Vote status
		var wg sync.WaitGroup
		for _, address := range collector.accAddresses {
			wg.Add(1)
			go func(address string) {
				defer wg.Done()
				vote, err := govClient.Vote(
					context.Background(),
					&v1.QueryVoteRequest{
						ProposalId: proposal.Id,
						Voter:      address,
					},
				)

				// When the voter_address hasn't voted, the query returns "not found for proposal" error
				if err != nil {
					VotedActiveProposalGauge.WithLabelValues(collector.chainID, address, strconv.FormatUint(proposal.Id, 10)).Set(float64(0))
					return
				}

				VotedActiveProposalGauge.WithLabelValues(collector.chainID, address, strconv.FormatUint(proposal.Id, 10)).Set(float64(1))
			}(address)
		}
		wg.Wait()
	}

	for key, total := range countProposalType {
		ActiveProposalGauge.WithLabelValues(collector.chainID, key).Set(float64(total))
	}
}
