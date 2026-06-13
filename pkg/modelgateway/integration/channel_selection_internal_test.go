package integration

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/channelcapability"
	"github.com/QuantumNous/new-api/pkg/modelgateway/core"
	modelgatewaycredential "github.com/QuantumNous/new-api/pkg/modelgateway/credential"
	"github.com/stretchr/testify/require"
)

func TestSelectedPlanSchedulingRejectReasonReportsAuthError(t *testing.T) {
	plan := &core.DispatchPlan{
		Channel: &model.Channel{
			Id:     301,
			Type:   constant.ChannelTypeCodex,
			Status: common.ChannelStatusEnabled,
			ChannelInfo: model.ChannelInfo{
				IsMultiKey: true,
				MultiKeyCapabilities: map[int]model.ChannelAccountCapability{
					0: {
						CapabilityClassification: channelcapability.ClassificationAuthError,
					},
				},
			},
		},
		CredentialRef: core.CredentialRef{CredentialIndex: 0},
	}
	resolved := modelgatewaycredential.ResolvedCredential{CredentialIndex: 0}

	reason := selectedPlanSchedulingRejectReason(plan, resolved)

	require.Equal(t, channelcapability.ClassificationAuthError, reason)
}
