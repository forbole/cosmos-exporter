package collector

import (
	"github.com/prometheus/client_golang/prometheus"
)

var (
	ActiveProposalGauge = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "tendermint_active_proposals_total",
			Help: "Total active proposals on chain",
		},
		[]string{"chain_id", "type"},
	)

	VotedActiveProposalGauge = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "tendermint_voted_active_proposals_total",
			Help: "Total active proposals on chain that voter_address voted",
		},
		[]string{"chain_id", "voter_address"},
	)

	AvailableBalanceGauge = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "tendermint_available_balance",
			Help: "Available balance",
		},
		[]string{"chain_id", "address", "denom"},
	)

	DelegatorRewardGauge = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "tendermint_staking_reward_total",
			Help: "Rewards of the delegator address from validator",
		},
		[]string{"delegator_address", "validator_address", "chain_id", "denom"},
	)

	DelegatorStakeGauge = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "tendermint_staking_total",
			Help: "Stake amount of delegator address to validator",
		},
		[]string{"delegator_address", "validator_address", "chain_id", "denom"},
	)

	ValidatorCommissionGauge = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "tendermint_validator_commission_total",
			Help: "Commission of the validator",
		},
		[]string{"validator_address", "chain_id", "denom"},
	)

	ValidatorDelegationGauge = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "tendermint_validator_delegators_total",
			Help: "Number of delegators to the validator",
		},
		[]string{"validator_address", "chain_id"},
	)

	ValidatorJailStatusGauge = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "tendermint_validator_jailed",
			Help: "Return 1 if the validator is jailed",
		},
		[]string{"validator_address", "chain_id"},
	)

	ValidatorCommissionRateGauge = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "tendermint_validator_commission_rate",
			Help: "Commission rate of the validator",
		},
		[]string{"validator_address", "chain_id"},
	)

	ValidatorVotingPowerGauge = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "tendermint_validator_voting_power_total",
			Help: "Voting power of the validator",
		},
		[]string{"validator_address", "chain_id", "denom"},
	)

	VotingPowerGauge = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "tendermint_voting_power_total",
			Help: "Total voting power of validators",
		},
		[]string{"chain_id", "denom"},
	)

	ValidatorVotingPowerRanking = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "tendermint_validator_voting_power_ranking",
			Help: "Ranking of the validator based on voting power",
		},
		[]string{"validator_address", "chain_id"},
	)

	BondedTokenGauge = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "tendermint_bonded_token",
		},
		[]string{"chain_id"},
	)

	NotBondedTokenGauge = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "tendermint_not_bonded_token",
			Help: "Total token staked in unbonding/unbonded validator",
		},
		[]string{"chain_id"},
	)

	CirculatingSupply = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "tendermint_circulating_supply",
			Help: "total circulating supply of staking token",
		},
		[]string{"chain_id"},
	)

	InflationRate = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "tendermint_inflation_rate",
			Help: "Current minting inflation value",
		},
		[]string{"chain_id"},
	)

	CommunityTax = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "tendermint_community_tax_rate",
		},
		[]string{"chain_id"},
	)

	UnbondingTime = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "tendermint_unbonding_time",
			Help: "Unbonding time in second",
		},
		[]string{"chain_id"},
	)

	// represents number of errors while collecting chain stats
	// collector label is used to determine which collector to debug
	ErrorGauge = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "cosmos_exporter_error_count",
			Help: "Total errors while collecting chain stats",
		},
		[]string{"collector"},
	)
)

func init() {
	prometheus.MustRegister(
		ActiveProposalGauge,
		VotedActiveProposalGauge,
		AvailableBalanceGauge,
		DelegatorRewardGauge,
		DelegatorStakeGauge,
		ValidatorCommissionGauge,
		ValidatorDelegationGauge,
		ValidatorJailStatusGauge,
		ValidatorCommissionRateGauge,
		ValidatorVotingPowerGauge,
		VotingPowerGauge,
		ValidatorVotingPowerRanking,
		BondedTokenGauge,
		NotBondedTokenGauge,
		CirculatingSupply,
		InflationRate,
		CommunityTax,
		UnbondingTime,
		ErrorGauge,
	)
}
