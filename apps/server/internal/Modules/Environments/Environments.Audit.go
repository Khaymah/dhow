package environments

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"

	auth "github.com/Khaymah/dhow/apps/server/internal/Core/Auth"
	eventbus "github.com/Khaymah/dhow/apps/server/internal/Core/EventBus"
	sqlc "github.com/Khaymah/dhow/apps/server/internal/Database/Sqlc"
	models "github.com/Khaymah/dhow/apps/server/internal/Modules/Environments/Models"
)

const (
	auditValueMax = 256
	changeArrow   = " -> "
	truncatedFlag = "true"
)

type auditData map[string]string

func canonical(v any) string {
	raw, err := json.Marshal(v)
	if err != nil {
		return ""
	}
	return string(raw)
}

func digestOf(v string) string {
	sum := sha256.Sum256([]byte(v))
	return hex.EncodeToString(sum[:])
}

func fits(v string) bool { return len(v) <= auditValueMax }

func cut(v string) string {
	if fits(v) {
		return v
	}
	end := auditValueMax
	for end > 0 && !utf8.RuneStart(v[end]) {
		end--
	}
	return v[:end]
}

func (d auditData) truncated() { d["Truncated"] = truncatedFlag }

func scalars(r models.Rules) map[string]string {
	return map[string]string{
		"AllowPullRequests": strconv.FormatBool(r.AllowPullRequests), "RequiredApprovals": strconv.Itoa(r.RequiredApprovals),
		"AllowSelfApproval": strconv.FormatBool(r.AllowSelfApproval), "WaitMinutes": strconv.Itoa(r.WaitMinutes),
		"ApprovalTimeoutHours": strconv.Itoa(r.ApprovalTimeoutHours), "RequirePinnedImages": strconv.FormatBool(r.RequirePinnedImages),
		"RequiredIsolation": r.RequiredIsolation, "AllowAdminBypass": strconv.FormatBool(r.AllowAdminBypass),
		"SupersedePending": strconv.FormatBool(r.SupersedePending),
	}
}

type listChange struct {
	count, digest, from, to string
	changed, truncated      bool
}

func compareList(before, after any, nBefore, nAfter int, initial bool) listChange {
	b, a := canonical(before), canonical(after)
	if initial {
		c := listChange{count: strconv.Itoa(nAfter), digest: digestOf(a), changed: true}
		c.to, c.truncated = valueIfFits(a)
		return c
	}
	if b == a {
		return listChange{}
	}
	c := listChange{
		count: strconv.Itoa(nBefore) + changeArrow + strconv.Itoa(nAfter), digest: digestOf(b) + changeArrow + digestOf(a), changed: true,
	}
	var cutFrom, cutTo bool
	c.from, cutFrom = valueIfFits(b)
	c.to, cutTo = valueIfFits(a)
	c.truncated = cutFrom || cutTo
	return c
}

func valueIfFits(v string) (string, bool) {
	if fits(v) {
		return v, false
	}
	return "", true
}

func reviewerKeys(r models.Reviewers) []string {
	out := make([]string, 0, len(r.UserIDs)+len(r.TeamIDs))
	for _, id := range r.UserIDs {
		out = append(out, "user:"+id.String())
	}
	for _, id := range r.TeamIDs {
		out = append(out, "team:"+id.String())
	}
	slices.Sort(out)
	return out
}

func missingFrom(keys, other []string) string {
	var out []string
	for _, k := range keys {
		if !slices.Contains(other, k) {
			out = append(out, k)
		}
	}
	return strings.Join(out, ",")
}

func (d auditData) reviewers(before, after models.Reviewers, initial bool) {
	b, a := reviewerKeys(before), reviewerKeys(after)
	if !initial && slices.Equal(b, a) {
		return
	}
	added, removed := missingFrom(a, b), missingFrom(b, a)
	if initial {
		d["ReviewersCount"], d["ReviewersDigest"] = strconv.Itoa(len(a)), digestOf(strings.Join(a, ","))
	} else {
		d["ReviewersCount"] = strconv.Itoa(len(b)) + changeArrow + strconv.Itoa(len(a))
		d["ReviewersDigest"] = digestOf(strings.Join(b, ",")) + changeArrow + digestOf(strings.Join(a, ","))
	}
	if added != "" {
		if v, cutOff := valueIfFits(added); cutOff {
			d.truncated()
		} else {
			d["ReviewersAdded"] = v
		}
	}
	if removed != "" {
		if v, cutOff := valueIfFits(removed); cutOff {
			d.truncated()
		} else {
			d["ReviewersRemoved"] = v
		}
	}
}

func (d auditData) rules(before, after models.Rules, initial bool) {
	old, next := scalars(before), scalars(after)
	for k, v := range next {
		switch {
		case initial:
			d[k] = v
		case old[k] != v:
			d[k] = old[k] + changeArrow + v
		}
	}
	if c := compareList(before.AllowedRefs, after.AllowedRefs, len(before.AllowedRefs), len(after.AllowedRefs), initial); c.changed {
		d["AllowedRefsCount"], d["AllowedRefsDigest"] = c.count, c.digest
		if c.from != "" {
			d["AllowedRefsFrom"] = c.from
		}
		if c.to != "" {
			d["AllowedRefsTo"] = c.to
		}
		if c.truncated {
			d.truncated()
		}
	}
	if c := compareList(before.AllowedPlugins, after.AllowedPlugins, len(before.AllowedPlugins), len(after.AllowedPlugins), initial); c.changed {
		d["AllowedPluginsCount"], d["AllowedPluginsDigest"] = c.count, c.digest
		if c.from != "" {
			d["AllowedPluginsFrom"] = c.from
		}
		if c.to != "" {
			d["AllowedPluginsTo"] = c.to
		}
		if c.truncated {
			d.truncated()
		}
	}
	d.reviewers(before.Reviewers, after.Reviewers, initial)
}

func (d auditData) reason(reason string) {
	if fits(reason) {
		d["Reason"] = reason
		return
	}
	d["Reason"], d["ReasonDigest"] = cut(reason), digestOf(reason)
	d.truncated()
}

func (s *Service) audit(ctx context.Context, q *sqlc.Queries, p auth.Principal, name string, e models.Environment, data auditData) error {
	actor := p.UserID
	data["Name"] = e.Name
	data["ProjectId"] = e.ProjectID.String()
	return s.bus.Publish(ctx, eventbus.Event{Name: name, Payload: eventbus.Audited{
		OrganizationIDs: []uuid.UUID{e.OrganizationID}, ActorUserID: &actor, TargetType: targetEnvironment,
		TargetID: e.ID.String(), IP: auth.RequestMetaFrom(ctx).IPString(), Data: data, Tx: q,
	}})
}
