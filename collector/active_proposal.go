package collector

import (
	"context"
	"log"
	"strconv"
	"sync"

	govtypes "github.com/cosmos/cosmos-sdk/x/gov/types"
	"github.com/prometheus/client_golang/prometheus"
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
		countProposalType[proposal.Content.TypeUrl] += 1
		// Vote status
		var wg sync.WaitGroup
		for _, address := range collector.accAddresses {
			wg.Add(1)
			go func(address string) {
				defer wg.Done()
				vote, err := govClient.Vote(
					context.Background(),
					&govtypes.QueryVoteRequest{
						ProposalId: proposal.ProposalId,
						Voter:      address,
					},
				)
				vote.GetVote()

				// When the voter_address hasn't voted, the query returns "not found for proposal" error
				if err != nil {
					VotedActiveProposalGauge.WithLabelValues(collector.chainID, address, strconv.FormatUint(proposal.ProposalId, 10)).Set(float64(0))
					return
				}

				VotedActiveProposalGauge.WithLabelValues(collector.chainID, address, strconv.FormatUint(proposal.ProposalId, 10)).Set(float64(1))
			}(address)
		}
		wg.Wait()
	}

	for key, total := range countProposalType {
		ActiveProposalGauge.WithLabelValues(collector.chainID, key).Set(float64(total))
	}
}
