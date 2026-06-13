package controller

import (
	"fmt"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/service"
)

func clearChannelAccountRuntimeBlocks(channelID int, credentialIndex int, clearUsageLimit bool) {
	if channelID <= 0 || credentialIndex < 0 {
		return
	}
	service.ClearChannelRuntimeFailureAvoidanceForAccountIndex(channelID, credentialIndex)
	service.ClearChannelConfigIsolationForAccountIndex(channelID, credentialIndex)
	service.ClearChannelRuntimeBalanceInsufficient(service.ChannelRuntimeIdentity{
		ChannelID:          channelID,
		CredentialIndex:    credentialIndex,
		CredentialIndexSet: true,
	})
	if !clearUsageLimit {
		return
	}
	if _, err := service.ClearCodexAccountUsageLimit(channelID, credentialIndex); err != nil {
		common.SysLog(fmt.Sprintf("failed to clear channel account usage limit: channel_id=%d credential_index=%d error=%v", channelID, credentialIndex, err))
	}
}

func clearChannelAccountsRuntimeBlocks(channelID int, credentialIndexes []int, clearUsageLimit bool) {
	seen := make(map[int]struct{}, len(credentialIndexes))
	for _, credentialIndex := range credentialIndexes {
		if _, ok := seen[credentialIndex]; ok {
			continue
		}
		seen[credentialIndex] = struct{}{}
		clearChannelAccountRuntimeBlocks(channelID, credentialIndex, clearUsageLimit)
	}
}
