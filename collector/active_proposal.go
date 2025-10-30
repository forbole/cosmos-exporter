package collector

import (
	"context"
	"log"
	"math/rand"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/cosmos/cosmos-sdk/types/query"
	v1 "github.com/cosmos/cosmos-sdk/x/gov/types/v1"
	v1beta1 "github.com/cosmos/cosmos-sdk/x/gov/types/v1beta1"
	"github.com/prometheus/client_golang/prometheus"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func (collector *CosmosSDKCollector) CollectActiveProposal() {
	// If chainID is unknown but we have a cached lastGoodChainID, use it
	if collector.chainID == "unknown-chain" && collector.lastGoodChainID != "" {
		collector.chainID = collector.lastGoodChainID
	}
	// If still unknown, skip to avoid polluting metrics with flapping label values
	if collector.chainID == "unknown-chain" {
		log.Printf("Skipping active proposal collection due to unknown chain ID")
		return
	}
	// retry configuration (could be made configurable later)
	const (
		maxRetries      = 3
		baseDelay       = 300 * time.Millisecond
		maxDelay        = 2 * time.Second
		voteTimeout     = 3 * time.Second
		bulkPageTimeout = 5 * time.Second
	)

	retry := func(opName string, fn func() error) error {
		var attempt int
		for {
			attempt++
			err := fn()
			if err == nil {
				return nil
			}
			st, ok := status.FromError(err)
			if ok {
				// Do not retry on NotFound or InvalidArgument etc.
				switch st.Code() {
				case codes.NotFound, codes.InvalidArgument, codes.PermissionDenied, codes.Unimplemented:
					return err
				}
			}
			if attempt >= maxRetries {
				return err
			}
			// exponential backoff with jitter
			delay := baseDelay * time.Duration(1<<uint(attempt-1))
			if delay > maxDelay {
				delay = maxDelay
			}
			// jitter 0-100ms
			j := time.Duration(rand.Intn(100)) * time.Millisecond
			log.Printf("retry %s attempt=%d err=%v (sleep %s)", opName, attempt, err, delay+j)
			time.Sleep(delay + j)
		}
	}

	// Try v1 first, fall back to v1beta1 if the service is not implemented
	govClient := v1.NewQueryClient(collector.grpcConn)
	govRes, err := govClient.Proposals(
		context.Background(),
		&v1.QueryProposalsRequest{
			ProposalStatus: v1.StatusVotingPeriod,
		},
	)

	// Check if error is Unimplemented and fall back to v1beta1
	st, isGrpcErr := status.FromError(err)
	useBeta1 := false
	if isGrpcErr && st.Code() == codes.Unimplemented {
		log.Printf("gov v1 not implemented, falling back to v1beta1")
		useBeta1 = true
		err = nil // Clear error to proceed with beta1
	}

	if err != nil {
		collector.recordGRPCError(err)
		ErrorGauge.WithLabelValues("tendermint_active_proposals_total").Inc()
		log.Print(err)
		return
	} else {
		collector.recordGRPCSuccess()
	}

	// If we need to use beta1, query with beta1 client
	var proposals []*v1.Proposal
	if useBeta1 {
		govBetaClient := v1beta1.NewQueryClient(collector.grpcConn)
		betaRes, betaErr := govBetaClient.Proposals(
			context.Background(),
			&v1beta1.QueryProposalsRequest{
				ProposalStatus: v1beta1.StatusVotingPeriod,
			},
		)
		if betaErr != nil {
			// Check if it's a transient error that should be retried
			betaSt, isBetaGrpcErr := status.FromError(betaErr)
			if isBetaGrpcErr {
				// For Unknown errors (like HTTP 520), don't treat as unrecoverable
				// It could be a temporary endpoint issue
				if betaSt.Code() != codes.Unknown {
					collector.recordGRPCError(betaErr)
					ErrorGauge.WithLabelValues("tendermint_active_proposals_total").Inc()
					log.Printf("v1beta1 query failed with non-retryable error: %v", betaErr)
					return
				}
			}
			// For Unknown or other recoverable errors, log and continue
			collector.recordGRPCError(betaErr)
			ErrorGauge.WithLabelValues("tendermint_active_proposals_total").Inc()
			log.Printf("v1beta1 query failed (possible endpoint issue): %v", betaErr)
			return
		}
		// Convert beta1 proposals to v1 proposals for unified processing
		proposals = convertBeta1ProposalsToV1(betaRes.Proposals)
	} else {
		proposals = govRes.Proposals
	}

	// We no longer blindly delete vote gauges to preserve continuity; only delete/overwrite active proposals after logic.

	ActiveProposalGauge.DeletePartialMatch(prometheus.Labels{"chain_id": collector.chainID})

	// Pre-emit last known statuses so Prometheus keeps timeseries even if this scrape encounters issues
	collector.stateMu.Lock()
	for key, val := range collector.lastVoteStatus {
		// key format: proposalID|address
		sep := strings.IndexByte(key, '|')
		if sep <= 0 {
			continue
		}
		pid := key[:sep]
		addr := key[sep+1:]
		VotedActiveProposalGauge.WithLabelValues(collector.chainID, addr, pid).Set(val)
	}
	collector.stateMu.Unlock()

	// Count proposals base on TypeUrl
	countProposalType := make(map[string]float64)
	for _, proposal := range proposals {
		msgTypeUrl := "unknown"
		if proposal.Messages != nil && len(proposal.Messages) > 0 {
			msgTypeUrl = proposal.Messages[0].TypeUrl
		}
		countProposalType[msgTypeUrl] += 1

		// Track whether this proposal ends up with zero confirmed votes for our watched addresses
		proposalHadAnyVote := false

		// Fetch all votes for the proposal in batches (pagination) and build a lookup map
		votesMap := make(map[string]struct{})
		var nextKey []byte
		fallbackExecuted := false
		for {
			var err error
			if useBeta1 {
				// Use beta1 client for votes
				var betaRes *v1beta1.QueryVotesResponse
				err = retry("VotesBeta1", func() error {
					ctx, cancel := context.WithTimeout(context.Background(), bulkPageTimeout)
					defer cancel()
					var innerErr error
					govBetaClient := v1beta1.NewQueryClient(collector.grpcConn)
					betaRes, innerErr = govBetaClient.Votes(
						ctx,
						&v1beta1.QueryVotesRequest{
							ProposalId: proposal.Id,
							Pagination: &query.PageRequest{Key: nextKey, Limit: 500},
						},
					)
					if innerErr == nil && betaRes != nil {
						for _, vote := range betaRes.Votes {
							votesMap[vote.Voter] = struct{}{}
						}
						if betaRes.Pagination != nil && len(betaRes.Pagination.NextKey) > 0 {
							nextKey = betaRes.Pagination.NextKey
						} else {
							nextKey = nil
						}
					}
					return innerErr
				})
				if nextKey == nil {
					break
				}
			} else {
				// Use v1 client for votes
				var res *v1.QueryVotesResponse
				err = retry("Votes", func() error {
					ctx, cancel := context.WithTimeout(context.Background(), bulkPageTimeout)
					defer cancel()
					var innerErr error
					res, innerErr = govClient.Votes(
						ctx,
						&v1.QueryVotesRequest{
							ProposalId: proposal.Id,
							Pagination: &query.PageRequest{Key: nextKey, Limit: 500},
						},
					)
					if innerErr == nil && res != nil {
						for _, vote := range res.Votes {
							votesMap[vote.Voter] = struct{}{}
						}
						if res.Pagination != nil && len(res.Pagination.NextKey) > 0 {
							nextKey = res.Pagination.NextKey
						} else {
							nextKey = nil
						}
					}
					return innerErr
				})
				if nextKey == nil {
					break
				}
			}
			if err != nil {
				collector.recordGRPCError(err)
				// Fall back to single queries if bulk fetch fails once
				log.Printf("bulk votes query failed for proposal %d: %v -- falling back to per-address queries", proposal.Id, err)
				ErrorGauge.WithLabelValues("tendermint_active_proposals_vote_status").Inc()
				var wg sync.WaitGroup
				for _, address := range collector.accAddresses {
					wg.Add(1)
					go func(address string) {
						defer wg.Done()
						var singleErr error
						retry("VoteFallback", func() error {
							ctxVote, cancelVote := context.WithTimeout(context.Background(), voteTimeout)
							defer cancelVote()
							if useBeta1 {
								govBetaClient := v1beta1.NewQueryClient(collector.grpcConn)
								_, singleErr = govBetaClient.Vote(ctxVote, &v1beta1.QueryVoteRequest{ProposalId: proposal.Id, Voter: address})
							} else {
								_, singleErr = govClient.Vote(ctxVote, &v1.QueryVoteRequest{ProposalId: proposal.Id, Voter: address})
							}
							return singleErr
						})
						if singleErr != nil {
							st, ok := status.FromError(singleErr)
							if ok && st.Code() == codes.NotFound {
								VotedActiveProposalGauge.WithLabelValues(collector.chainID, address, strconv.FormatUint(proposal.Id, 10)).Set(0)
								return
							}
							// Log non-NotFound errors but don't increment error counter
							// as this is a fallback scenario and already logged as an error
							log.Printf("error querying vote (fallback) proposal_id=%d address=%s: %v", proposal.Id, address, singleErr)
							// Still set 0 to indicate vote not found
							VotedActiveProposalGauge.WithLabelValues(collector.chainID, address, strconv.FormatUint(proposal.Id, 10)).Set(0)
							return
						}
						VotedActiveProposalGauge.WithLabelValues(collector.chainID, address, strconv.FormatUint(proposal.Id, 10)).Set(1)
					}(address)
				}
				wg.Wait()
				fallbackExecuted = true
				break
			}

			collector.recordGRPCSuccess()
		}

		if !fallbackExecuted {
			// Two-phase verification: first pass collects addresses with NotFound, second pass re-checks before setting 0
			var firstNotFound []string
			var firstNFMu sync.Mutex
			var verifyWg sync.WaitGroup
			for _, address := range collector.accAddresses {
				if _, ok := votesMap[address]; ok {
					VotedActiveProposalGauge.WithLabelValues(collector.chainID, address, strconv.FormatUint(proposal.Id, 10)).Set(1)
					collector.stateMu.Lock()
					collector.lastVoteStatus[strconv.FormatUint(proposal.Id, 10)+"|"+address] = 1
					collector.voteMissCounts[strconv.FormatUint(proposal.Id, 10)+"|"+address] = 0
					collector.stateMu.Unlock()
					proposalHadAnyVote = true
					continue
				}
				verifyWg.Add(1)
				go func(address string) {
					defer verifyWg.Done()
					var singleErr error
					retry("VoteVerify1", func() error {
						ctxVote, cancelVote := context.WithTimeout(context.Background(), voteTimeout)
						defer cancelVote()
						if useBeta1 {
							govBetaClient := v1beta1.NewQueryClient(collector.grpcConn)
							_, singleErr = govBetaClient.Vote(ctxVote, &v1beta1.QueryVoteRequest{ProposalId: proposal.Id, Voter: address})
						} else {
							_, singleErr = govClient.Vote(ctxVote, &v1.QueryVoteRequest{ProposalId: proposal.Id, Voter: address})
						}
						return singleErr
					})
					if singleErr != nil {
						st, ok := status.FromError(singleErr)
						if ok && st.Code() == codes.NotFound {
							// collect for second confirmation
							firstNFMu.Lock()
							firstNotFound = append(firstNotFound, address)
							firstNFMu.Unlock()
							return
						}
						// Log error but don't double-count
						log.Printf("verify1 vote query error proposal_id=%d address=%s: %v", proposal.Id, address, singleErr)
						return
					}
					VotedActiveProposalGauge.WithLabelValues(collector.chainID, address, strconv.FormatUint(proposal.Id, 10)).Set(1)
					collector.stateMu.Lock()
					collector.lastVoteStatus[strconv.FormatUint(proposal.Id, 10)+"|"+address] = 1
					collector.voteMissCounts[strconv.FormatUint(proposal.Id, 10)+"|"+address] = 0
					collector.stateMu.Unlock()
				}(address)
			}
			verifyWg.Wait()

			if len(firstNotFound) > 0 {
				// small delay before second confirmation to mitigate race with just-submitted votes
				time.Sleep(400 * time.Millisecond)
				var secondWg sync.WaitGroup
				for _, address := range firstNotFound {
					addr := address
					secondWg.Add(1)
					go func() {
						defer secondWg.Done()
						var secondErr error
						retry("VoteVerify2", func() error {
							ctxVote, cancelVote := context.WithTimeout(context.Background(), voteTimeout)
							defer cancelVote()
							if useBeta1 {
								govBetaClient := v1beta1.NewQueryClient(collector.grpcConn)
								_, secondErr = govBetaClient.Vote(ctxVote, &v1beta1.QueryVoteRequest{ProposalId: proposal.Id, Voter: addr})
							} else {
								_, secondErr = govClient.Vote(ctxVote, &v1.QueryVoteRequest{ProposalId: proposal.Id, Voter: addr})
							}
							return secondErr
						})
						if secondErr != nil {
							st, ok := status.FromError(secondErr)
							if ok && st.Code() == codes.NotFound {
								// multi-scrape confirmation: increment miss count; only set 0 if >=2 consecutive scrapes missing
								collector.stateMu.Lock()
								key := strconv.FormatUint(proposal.Id, 10) + "|" + addr
								collector.voteMissCounts[key]++
								misses := collector.voteMissCounts[key]
								if misses >= 2 {
									VotedActiveProposalGauge.WithLabelValues(collector.chainID, addr, strconv.FormatUint(proposal.Id, 10)).Set(0)
									collector.lastVoteStatus[key] = 0
									log.Printf("confirmed zero vote proposal_id=%d address=%s after %d consecutive misses", proposal.Id, addr, misses)
								} else {
									// retain previous gauge (already emitted earlier) until confirmed second miss
									log.Printf("deferring zero until second consecutive miss proposal_id=%d address=%s", proposal.Id, addr)
								}
								collector.stateMu.Unlock()
								return
							}
							// still uncertain; log and skip setting 0
							log.Printf("verify2 uncertain proposal_id=%d address=%s err=%v", proposal.Id, addr, secondErr)
							return
						}
						// vote found on second attempt -> set 1
						VotedActiveProposalGauge.WithLabelValues(collector.chainID, addr, strconv.FormatUint(proposal.Id, 10)).Set(1)
						collector.stateMu.Lock()
						key := strconv.FormatUint(proposal.Id, 10) + "|" + addr
						collector.lastVoteStatus[key] = 1
						collector.voteMissCounts[key] = 0
						collector.stateMu.Unlock()
					}()
				}
				secondWg.Wait()
			}
			// After per-address processing, handle proposal-level all-miss heuristic
			collector.stateMu.Lock()
			if !proposalHadAnyVote {
				collector.proposalAllMiss[proposal.Id]++
				pm := collector.proposalAllMiss[proposal.Id]
				if pm == 1 {
					log.Printf("proposal %d had no detected votes this scrape; waiting for second consecutive confirmation before zeroing all", proposal.Id)
				} else if pm >= 2 {
					// Second consecutive full-miss: ensure any addresses not explicitly set get zeroed now
					for _, addr := range collector.accAddresses {
						key := strconv.FormatUint(proposal.Id, 10) + "|" + addr
						if _, had := collector.lastVoteStatus[key]; !had || collector.lastVoteStatus[key] != 1 {
							VotedActiveProposalGauge.WithLabelValues(collector.chainID, addr, strconv.FormatUint(proposal.Id, 10)).Set(0)
							collector.lastVoteStatus[key] = 0
						}
					}
				}
			} else {
				collector.proposalAllMiss[proposal.Id] = 0
			}
			collector.stateMu.Unlock()
		}
	}

	for key, total := range countProposalType {
		ActiveProposalGauge.WithLabelValues(collector.chainID, key).Set(float64(total))
	}

	// Prune stale proposal state (proposals no longer in voting period)
	activeSet := make(map[uint64]struct{})
	for _, p := range proposals {
		activeSet[p.Id] = struct{}{}
	}
	collector.stateMu.Lock()
	for k := range collector.lastVoteStatus {
		sep := strings.IndexByte(k, '|')
		if sep <= 0 {
			continue
		}
		pidStr := k[:sep]
		pid, err := strconv.ParseUint(pidStr, 10, 64)
		if err != nil {
			continue
		}
		if _, ok := activeSet[pid]; !ok {
			delete(collector.lastVoteStatus, k)
			delete(collector.voteMissCounts, k)
		}
	}
	for pid := range collector.proposalAllMiss {
		if _, ok := activeSet[pid]; !ok {
			delete(collector.proposalAllMiss, pid)
		}
	}
	collector.stateMu.Unlock()
}

// convertBeta1ProposalsToV1 converts v1beta1 proposals to v1 proposals for unified processing
func convertBeta1ProposalsToV1(betaProposals []v1beta1.Proposal) []*v1.Proposal {
	v1Proposals := make([]*v1.Proposal, 0, len(betaProposals))
	for _, bp := range betaProposals {
		v1p := &v1.Proposal{
			Id: bp.ProposalId,
		}
		// v1beta1 uses Content instead of Messages
		// Convert single Content to Messages array format
		if bp.Content != nil {
			// bp.Content is already compatible with Messages field
			v1p.Messages = append(v1p.Messages, bp.Content)
		}
		v1Proposals = append(v1Proposals, v1p)
	}
	return v1Proposals
}
