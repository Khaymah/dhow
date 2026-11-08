package boards

import (
	"encoding/hex"
	"slices"

	"github.com/google/uuid"

	sqlc "github.com/Khaymah/dhow/apps/server/internal/Database/Sqlc"
	models "github.com/Khaymah/dhow/apps/server/internal/Modules/Boards/Models"
)

const (
	stateSucceeded      = "succeeded"
	stateRolledBack     = "rolled_back"
	stateRollbackFailed = "rollback_failed"
	stateUnknown        = "unknown"
	kindRollback        = "rollback"
	digestBytes         = 32
)

func assemble(snap snapshot) models.Board {
	byEnv := map[uuid.UUID][]models.Target{}
	for _, t := range snap.targets {
		byEnv[t.EnvironmentId] = append(byEnv[t.EnvironmentId], targetOf(t, snap))
	}
	out := models.Board{Environments: make([]models.Environment, 0, len(snap.envs)), Truncated: snap.truncated}
	for _, e := range snap.envs {
		targets := byEnv[e.Id]
		if targets == nil {
			targets = []models.Target{}
		}
		out.Environments = append(out.Environments, models.Environment{
			ID: e.Id, Name: e.Name, Paused: e.Paused, PendingApprovals: snap.approvals[e.Id], Targets: targets,
		})
	}
	return out
}

func targetOf(t sqlc.ListBoardTargetsRow, snap snapshot) models.Target {
	slots := snap.releases[t.Id]
	out := models.Target{ID: t.Id, Name: t.Name, Kind: t.Kind, Hosts: int(t.Hosts)}
	if cur, ok := slots[slotCurrent]; ok {
		out.CurrentReleaseID = cur.ReleaseId
		if cur.Kind == kindRollback && cur.RestoredReleaseId.Valid {
			out.CurrentReleaseID = cur.RestoredReleaseId.String
		}
		if len(cur.ArtifactDigest) == digestBytes {
			out.CurrentDigest = hex.EncodeToString(cur.ArtifactDigest)
		}
	}
	var last *sqlc.ListBoardReleasesRow
	if l, ok := slots[slotLast]; ok {
		last = &l
		out.Last = releaseOf(l)
		out.AutoRolledBack = l.State == stateRolledBack
		out.RollbackFailed = l.State == stateRollbackFailed
	}
	if run, ok := slots[slotRunning]; ok {
		out.Running = progressOf(run, snap.progress[run.Id])
	}
	out.Partial = isPartial(last, snap.checks[t.Id], int(t.Hosts))
	return out
}

func releaseOf(r sqlc.ListBoardReleasesRow) *models.Release {
	return &models.Release{
		ID: r.Id, ReleaseID: r.ReleaseId, Kind: r.Kind, State: r.State, RollbackReason: r.RollbackReason.String,
		StartedAt: r.StartedAt, FinishedAt: r.FinishedAt,
	}
}

func progressOf(r sqlc.ListBoardReleasesRow, h sqlc.ListBoardRunningHostsRow) *models.Progress {
	size := max(int(r.BatchSize), 1)
	hosts := int(h.Hosts)
	return &models.Progress{
		ReleaseID: r.ReleaseId, Hosts: hosts, HostsDone: int(h.Done), BatchSize: size, Batch: int(h.Batch),
		Batches: max((hosts+size-1)/size, 1),
	}
}

func isPartial(last *sqlc.ListBoardReleasesRow, checks []sqlc.ListBoardHostChecksRow, hosts int) bool {
	distinct := map[string]bool{}
	fresh := 0
	for _, c := range checks {
		if !c.CurrentReleaseId.Valid || c.CurrentReleaseId.String == "" {
			continue
		}
		if last != nil && last.FinishedAt != nil && !c.CheckedAt.After(*last.FinishedAt) {
			continue
		}
		distinct[c.CurrentReleaseId.String] = true
		fresh++
	}
	if len(distinct) > 1 {
		return true
	}
	if last != nil && slices.Contains([]string{stateRollbackFailed, stateUnknown}, last.State) {
		return len(distinct) != 1 || fresh != hosts
	}
	return false
}
