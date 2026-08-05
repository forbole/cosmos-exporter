package collector

import (
	"testing"

	codecTypes "github.com/cosmos/cosmos-sdk/codec/types"
	v1 "github.com/cosmos/cosmos-sdk/x/gov/types/v1"
	"google.golang.org/protobuf/encoding/protowire"
)

func TestProposalResultLabelsSoftwareUpgrade(t *testing.T) {
	plan := protowire.AppendTag(nil, 1, protowire.BytesType)
	plan = protowire.AppendString(plan, "v27.6.0")
	// Plan.height is protobuf field 3 (field 2 is deprecated time)
	plan = protowire.AppendTag(plan, 3, protowire.VarintType)
	plan = protowire.AppendVarint(plan, 32361600)

	message := protowire.AppendTag(nil, 2, protowire.BytesType)
	message = protowire.AppendBytes(message, plan)

	proposal := &v1.Proposal{Messages: []*codecTypes.Any{{
		TypeUrl: "/cosmos.upgrade.v1beta1.MsgSoftwareUpgrade",
		Value:   message,
	}}}

	proposalType, upgradeName, upgradeHeight := proposalResultLabels(proposal)
	if proposalType != "/cosmos.upgrade.v1beta1.MsgSoftwareUpgrade" {
		t.Fatalf("proposal type = %q", proposalType)
	}
	if upgradeName != "v27.6.0" || upgradeHeight != "32361600" {
		t.Fatalf("upgrade labels = %q, %q", upgradeName, upgradeHeight)
	}
}

func TestProposalResultLabelsNonUpgrade(t *testing.T) {
	proposal := &v1.Proposal{Messages: []*codecTypes.Any{{TypeUrl: "/cosmos.distribution.v1beta1.MsgCommunityPoolSpend"}}}
	proposalType, upgradeName, upgradeHeight := proposalResultLabels(proposal)
	if proposalType != "/cosmos.distribution.v1beta1.MsgCommunityPoolSpend" || upgradeName != "" || upgradeHeight != "" {
		t.Fatalf("unexpected labels = %q, %q, %q", proposalType, upgradeName, upgradeHeight)
	}
}
