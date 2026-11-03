package audit

import (
	"context"
	"sort"
	"strings"

	config "github.com/Khaymah/dhow/apps/server/internal/Config"
	eventbus "github.com/Khaymah/dhow/apps/server/internal/Core/EventBus"
)

const (
	maxDataEntries  = 32
	maxDataValueLen = 256
)

var allowedDataKeys = map[string]struct{}{
	"Arch": {}, "Disabled": {}, "Email": {}, "EmailDigest": {}, "EmailStatus": {}, "ExpiresAt": {}, "ExpiresInDays": {},
	"DefaultBranch": {}, "Fields": {}, "Fingerprint": {}, "FirstMethod": {}, "ForkPolicy": {}, "From": {}, "Group": {}, "Host": {}, "Image": {}, "KeyVersion": {}, "Labels": {}, "LogRetentionDays": {}, "MaxUses": {},
	"Method": {}, "Name": {}, "NewAccount": {}, "Number": {}, "ProjectId": {}, "Ref": {}, "Sha": {}, "Trigger": {}, "Operation": {}, "OrganizationSlug": {}, "Os": {}, "Owner": {}, "Passkeys": {}, "PipelinePath": {}, "Priority": {},
	"Provider": {}, "Purpose": {}, "Reason": {}, "RecoveryCodes": {}, "RegisteredWith": {}, "Remaining": {}, "Replaced": {},
	"Revoked": {}, "RevokedPats": {}, "RevokedSessions": {}, "Role": {}, "Scope": {}, "Scopes": {}, eventbus.SeverityKey: {},
	"Slug": {}, "SpecDigest": {}, "State": {}, "Steps": {}, "SubjectDigest": {}, "Switches": {}, "To": {}, "Totp": {}, "Transport": {},
	"UserId": {}, "Uses": {}, "Version": {}, "Window": {},
	"AllowPullRequests": {}, "RequiredApprovals": {}, "AllowSelfApproval": {}, "WaitMinutes": {}, "ApprovalTimeoutHours": {},
	"RequirePinnedImages": {}, "RequiredIsolation": {}, "AllowAdminBypass": {}, "SupersedePending": {},
	"AllowedRefsCount": {}, "AllowedRefsDigest": {}, "AllowedRefsFrom": {}, "AllowedRefsTo": {},
	"AllowedPluginsCount": {}, "AllowedPluginsDigest": {}, "AllowedPluginsFrom": {}, "AllowedPluginsTo": {},
	"ReviewersCount": {}, "ReviewersDigest": {}, "ReviewersAdded": {}, "ReviewersRemoved": {}, "ReasonDigest": {}, "Truncated": {},
	"KekVersions": {}, "DataKeys": {},
}

var credentialMarkers = []string{config.APITokenPrefix, config.RunnerAccessTokenPrefix, config.RunnerRegistrationTokenPrefix}

func AllowedDataKeys() []string {
	out := make([]string, 0, len(allowedDataKeys))
	for k := range allowedDataKeys {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func AllowedDataKey(k string) bool {
	_, ok := allowedDataKeys[k]
	return ok
}

func carriesCredential(v string) bool {
	for _, m := range credentialMarkers {
		if strings.Contains(v, m) {
			return true
		}
	}
	return false
}

func (s *Service) sanitizeData(ctx context.Context, action string, in map[string]string) map[string]string {
	out := make(map[string]string, min(len(in), maxDataEntries))
	keys := make([]string, 0, len(in))
	for k := range in {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		v := in[k]
		switch {
		case !AllowedDataKey(k):
			s.log.WarnContext(ctx, "audit data field dropped: not on the allowlist", "event", action, "field", k)
			continue
		case carriesCredential(v):
			s.log.WarnContext(ctx, "audit data field dropped: the value looks like a credential", "event", action, "field", k)
			continue
		case len(out) >= maxDataEntries:
			s.log.WarnContext(ctx, "audit data field dropped: too many fields", "event", action, "field", k)
			continue
		}
		if len(v) > maxDataValueLen {
			v = v[:maxDataValueLen]
		}
		out[k] = v
	}
	return out
}
