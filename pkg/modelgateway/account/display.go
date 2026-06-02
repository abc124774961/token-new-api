package account

import (
	"fmt"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/pkg/modelgateway/core"
)

func CredentialUID(identity core.AccountIdentity, credential core.CredentialRef, resource core.ResourceRef, channelID int) string {
	return CredentialUIDFromParts(CredentialUIDParts{
		CredentialSubjectFingerprint: firstNonEmpty(identity.CredentialSubjectFingerprint, credential.CredentialSubjectFingerprint),
		CredentialFingerprint:        firstNonEmpty(identity.CredentialFingerprint, credential.CredentialFingerprint),
		AccountUniqueKey:             identity.AccountUniqueKey,
		AccountIdentityKey:           identity.AccountIdentityKey,
		AccountID:                    firstNonEmpty(identity.AccountID, credential.AccountID),
		CredentialResourceID:         credential.ResourceID,
		ResourceID:                   resource.ResourceID,
		ChannelID:                    channelID,
	})
}

func CredentialLabel(identity core.AccountIdentity, credential core.CredentialRef, resource core.ResourceRef, channelID int) string {
	uid := CredentialUID(identity, credential, resource, channelID)
	if uid == "" {
		return ""
	}
	brand := strings.TrimSpace(identity.Brand)
	if brand == "" {
		brand = strings.TrimSpace(resource.Brand)
	}
	if brand == "" {
		return uid
	}
	return strings.ToLower(brand) + "-" + uid
}

type CredentialUIDParts struct {
	CredentialSubjectFingerprint string
	CredentialFingerprint        string
	AccountUniqueKey             string
	AccountIdentityKey           string
	AccountID                    string
	CredentialAccountID          string
	CredentialResourceID         string
	ResourceID                   string
	ChannelID                    int
}

func CredentialUIDFromParts(parts CredentialUIDParts) string {
	for _, fingerprint := range []string{
		parts.CredentialSubjectFingerprint,
		parts.CredentialFingerprint,
	} {
		if short := ShortFingerprint(fingerprint); short != "" {
			return "acct-" + short
		}
	}
	for _, source := range []string{
		parts.AccountUniqueKey,
		parts.AccountIdentityKey,
		parts.AccountID,
		parts.CredentialAccountID,
		parts.CredentialResourceID,
		parts.ResourceID,
	} {
		source = strings.TrimSpace(source)
		if source != "" {
			return "acct-" + ShortFingerprint(common.GenerateHMAC(source))
		}
	}
	if parts.ChannelID > 0 {
		return fmt.Sprintf("acct-ch%d", parts.ChannelID)
	}
	return ""
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			return value
		}
	}
	return ""
}
