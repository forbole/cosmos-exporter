package collector

import (
	"context"

	govtypes "github.com/cosmos/cosmos-sdk/x/gov/types"
	"github.com/prometheus/client_golang/prometheus"
	"google.golang.org/grpc"
)

type ActiveProposalGauge struct {
	ChainID                  string
	DelegatorAddress         string
	ActiveProposalsDesc      *prometheus.Desc
	VotedActiveProposalsDesc *prometheus.Desc
	GrpcConn                 *grpc.ClientConn
}

func NewActiveProposalGauge(grpcConn *grpc.ClientConn, delegatorAddress string, chainID string) *ActiveProposalGauge {
	return &ActiveProposalGauge{
		ChainID:          chainID,
		DelegatorAddress: delegatorAddress,
		ActiveProposalsDesc: prometheus.NewDesc(
			"active_proposals_total",
			"Total active proposals",
			[]string{"chain_id", "type"},
			nil,
		),
		GrpcConn: grpcConn,
		VotedActiveProposalsDesc: prometheus.NewDesc(
			"voted_active_proposals_total",
			"Total voted active proposals",
			[]string{"chain_id", "voter_address"},
			nil,
		),
	}
}

func (collector *ActiveProposalGauge) Describe(ch chan<- *prometheus.Desc) {
	ch <- collector.ActiveProposalsDesc
	ch <- collector.VotedActiveProposalsDesc
}

func (collector *ActiveProposalGauge) Collect(ch chan<- prometheus.Metric) {
	govClient := govtypes.NewQueryClient(collector.GrpcConn)
	govRes, err := govClient.Proposals(
		context.Background(),
		&govtypes.QueryProposalsRequest{
			ProposalStatus: govtypes.StatusVotingPeriod,
		},
	)

	if err != nil {
		ch <- prometheus.NewInvalidMetric(collector.ActiveProposalsDesc, err)
		return
	}

	// Count proposals base on TypeUrl
	countProposalType := make(map[string]float64)
	for _, proposal := range govRes.Proposals {
		countProposalType[proposal.Content.TypeUrl] += 1
	}

	for key, total := range countProposalType {
		ch <- prometheus.MustNewConstMetric(collector.ActiveProposalsDesc, prometheus.GaugeValue, total, collector.ChainID, key)
	}

	// Voted active proposal
	votedGovRes, err := govClient.Proposals(
		context.Background(),
		&govtypes.QueryProposalsRequest{
			ProposalStatus: govtypes.StatusVotingPeriod,
			Voter:          collector.DelegatorAddress,
		},
	)
	ch <- prometheus.MustNewConstMetric(collector.VotedActiveProposalsDesc, prometheus.GaugeValue, float64(len(votedGovRes.Proposals)), collector.ChainID, collector.DelegatorAddress)

}
