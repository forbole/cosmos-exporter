package collector

import (
	"context"
	"log"
	"strconv"
	"sync"
	"time"

	"github.com/cosmos/cosmos-sdk/types/query"
	v1 "github.com/cosmos/cosmos-sdk/x/gov/types/v1"
	"github.com/prometheus/client_golang/prometheus"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
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

		// Fetch all votes for the proposal in batches (pagination) and build a lookup map
		votesMap := make(map[string]struct{})
		var nextKey []byte
		fallbackExecuted := false
		for {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			res, err := govClient.Votes(
				ctx,
				&v1.QueryVotesRequest{
					ProposalId: proposal.Id,
					Pagination: &query.PageRequest{Key: nextKey, Limit: 500},
				},
			)
			cancel()
			if err != nil {
				// Fall back to single queries if bulk fetch fails once
				log.Printf("bulk votes query failed for proposal %d: %v -- falling back to per-address queries", proposal.Id, err)
				ErrorGauge.WithLabelValues("tendermint_active_proposals_vote_status").Inc()
				var wg sync.WaitGroup
				for _, address := range collector.accAddresses {
					wg.Add(1)
					go func(address string) {
						defer wg.Done()
						ctxVote, cancelVote := context.WithTimeout(context.Background(), 3*time.Second)
						defer cancelVote()
						_, singleErr := govClient.Vote(ctxVote, &v1.QueryVoteRequest{ProposalId: proposal.Id, Voter: address})
						if singleErr != nil {
							st, ok := status.FromError(singleErr)
							if ok && st.Code() == codes.NotFound {
								VotedActiveProposalGauge.WithLabelValues(collector.chainID, address, strconv.FormatUint(proposal.Id, 10)).Set(0)
								return
							}
							ErrorGauge.WithLabelValues("tendermint_active_proposals_vote_status").Inc()
							log.Printf("error querying vote (fallback) proposal_id=%d address=%s: %v", proposal.Id, address, singleErr)
							return
						}
						VotedActiveProposalGauge.WithLabelValues(collector.chainID, address, strconv.FormatUint(proposal.Id, 10)).Set(1)
					}(address)
				}
				wg.Wait()
				fallbackExecuted = true
				break
			}

			for _, vote := range res.Votes {
				votesMap[vote.Voter] = struct{}{}
			}
			if res.Pagination == nil || len(res.Pagination.NextKey) == 0 {
				break
			}
			nextKey = res.Pagination.NextKey
		}

		if !fallbackExecuted {
			// Now set gauges; if not in map, double-check with a direct single vote query to avoid transient omission
			var verifyWg sync.WaitGroup
			for _, address := range collector.accAddresses {
				if _, ok := votesMap[address]; ok {
					VotedActiveProposalGauge.WithLabelValues(collector.chainID, address, strconv.FormatUint(proposal.Id, 10)).Set(1)
					continue
				}
				verifyWg.Add(1)
				go func(address string) {
					defer verifyWg.Done()
					ctxVote, cancelVote := context.WithTimeout(context.Background(), 3*time.Second)
					defer cancelVote()
					_, singleErr := govClient.Vote(ctxVote, &v1.QueryVoteRequest{ProposalId: proposal.Id, Voter: address})
					if singleErr != nil {
						st, ok := status.FromError(singleErr)
						if ok && st.Code() == codes.NotFound {
							VotedActiveProposalGauge.WithLabelValues(collector.chainID, address, strconv.FormatUint(proposal.Id, 10)).Set(0)
							return
						}
						// transient error: do not set 0, leave metric absent (next cycle may fill). Count error.
						ErrorGauge.WithLabelValues("tendermint_active_proposals_vote_status").Inc()
						log.Printf("verify vote query error proposal_id=%d address=%s: %v", proposal.Id, address, singleErr)
						return
					}
					VotedActiveProposalGauge.WithLabelValues(collector.chainID, address, strconv.FormatUint(proposal.Id, 10)).Set(1)
				}(address)
			}
			verifyWg.Wait()
		}
	}

	for key, total := range countProposalType {
		ActiveProposalGauge.WithLabelValues(collector.chainID, key).Set(float64(total))
	}
}
