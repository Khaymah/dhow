package environments

import (
	"context"
	"fmt"
	"slices"
	"time"

	"github.com/google/uuid"

	auth "github.com/Khaymah/dhow/apps/server/internal/Core/Auth"
	apperrors "github.com/Khaymah/dhow/apps/server/internal/Core/Errors"
	eventbus "github.com/Khaymah/dhow/apps/server/internal/Core/EventBus"
	sqlc "github.com/Khaymah/dhow/apps/server/internal/Database/Sqlc"
	dbtx "github.com/Khaymah/dhow/apps/server/internal/Database/Tx"
	models "github.com/Khaymah/dhow/apps/server/internal/Modules/Environments/Models"
)

func addressOf(host string, port int) string { return fmt.Sprintf("%s:%d", host, port) }

func (s *Service) renumber(ctx context.Context, q *sqlc.Queries, org, target uuid.UUID, ordered []uuid.UUID) error {
	if err := q.ShiftDeployTargetHostPositions(ctx, sqlc.ShiftDeployTargetHostPositionsParams{TargetID: target, OrganizationID: org}); err != nil {
		return err
	}
	return q.SetDeployTargetHostPositions(ctx, sqlc.SetDeployTargetHostPositionsParams{TargetID: target, OrganizationID: org, HostIds: ordered})
}

func hostIDs(hosts []models.TargetHost) []uuid.UUID {
	out := make([]uuid.UUID, 0, len(hosts))
	for _, h := range hosts {
		out = append(out, h.ID)
	}
	return out
}

func (s *Service) bumpAfterHostChange(ctx context.Context, q *sqlc.Queries, locked lockedTarget) (models.Target, error) {
	hosts, err := q.ListDeployTargetHosts(ctx, sqlc.ListDeployTargetHostsParams{OrganizationID: locked.row.OrganizationId, TargetIds: []uuid.UUID{locked.row.Id}})
	if err != nil {
		return models.Target{}, err
	}
	next := locked.target
	if r := next.Settings.Rolling; r != nil && r.BatchSize > len(hosts) {
		next.Settings.Rolling = &models.Rolling{BatchSize: max(len(hosts), 1), BatchPauseSeconds: r.BatchPauseSeconds}
		return s.saveTarget(ctx, q, locked, next, nil)
	}
	row, err := q.TouchDeployTarget(ctx, sqlc.TouchDeployTargetParams{ID: locked.row.Id, OrganizationID: locked.row.OrganizationId, Now: s.now()})
	if err != nil {
		return models.Target{}, err
	}
	return s.oneTarget(ctx, q, row)
}

func (s *Service) AddHost(ctx context.Context, org, project uuid.UUID, envName, name string, version int64, in models.HostInput, factor auth.Factor) (models.Target, error) {
	p, err := principalOf(ctx)
	if err != nil {
		return models.Target{}, err
	}
	host, ae := normalizeHost(in.Host, fieldHost)
	if ae != nil {
		return models.Target{}, ae
	}
	port := models.DefaultSSHPort
	if in.Port != nil {
		port = *in.Port
	}
	if !validPort(port) {
		return models.Target{}, apperrors.ValidationFailed(invalid(fieldPort))
	}
	if err := s.confirm(ctx, p, factor); err != nil {
		return models.Target{}, err
	}
	var out models.Target
	err = dbtx.Run(ctx, s.db, func(q *sqlc.Queries) error {
		locked, err := s.lockTarget(ctx, q, org, project, envName, name)
		if err != nil {
			return err
		}
		if err := locked.checkVersion(version); err != nil {
			return err
		}
		if len(locked.target.Hosts) >= models.MaxTargetHosts {
			return apperrors.ValidationFailed(invalid(fieldHosts))
		}
		row, err := q.InsertDeployTargetHost(ctx, sqlc.InsertDeployTargetHostParams{
			OrganizationID: org, TargetID: locked.row.Id, Position: int32(len(locked.target.Hosts) + 1), Host: host, //nolint:gosec // at most 10.
			Port: int32(port), DeployPath: locked.row.DeployPath, Now: s.now(), //nolint:gosec // validated range.
		})
		if err != nil {
			return asTargetConflict(err)
		}
		if out, err = s.bumpAfterHostChange(ctx, q, locked); err != nil {
			return err
		}
		return s.auditTarget(ctx, q, p, eventbus.TargetHostAdded, locked.env, out, map[string]string{
			"HostId": row.Id.String(), "Address": addressOf(host, port), "Position": fmt.Sprint(row.Position), "Version": fmt.Sprint(out.Version),
		})
	})
	return out, err
}

func findHost(t models.Target, id uuid.UUID) (models.TargetHost, bool) {
	i := slices.IndexFunc(t.Hosts, func(h models.TargetHost) bool { return h.ID == id })
	if i < 0 {
		return models.TargetHost{}, false
	}
	return t.Hosts[i], true
}

func (s *Service) UpdateHost(ctx context.Context, org, project uuid.UUID, envName, name string, hostID uuid.UUID, in models.UpdateHostInput, factor auth.Factor) (models.Target, error) {
	p, err := principalOf(ctx)
	if err != nil {
		return models.Target{}, err
	}
	if in.Host == nil && in.Port == nil {
		return models.Target{}, apperrors.BadRequest()
	}
	if in.Port != nil && !validPort(*in.Port) {
		return models.Target{}, apperrors.ValidationFailed(invalid(fieldPort))
	}
	var host string
	if in.Host != nil {
		normalized, ae := normalizeHost(*in.Host, fieldHost)
		if ae != nil {
			return models.Target{}, ae
		}
		host = normalized
	}
	if err := s.confirm(ctx, p, factor); err != nil {
		return models.Target{}, err
	}
	var out models.Target
	err = dbtx.Run(ctx, s.db, func(q *sqlc.Queries) error {
		locked, err := s.lockTarget(ctx, q, org, project, envName, name)
		if err != nil {
			return err
		}
		if err := locked.checkVersion(in.Version); err != nil {
			return err
		}
		current, ok := findHost(locked.target, hostID)
		if !ok {
			return apperrors.NotFound()
		}
		nextHost, nextPort := current.Host, current.Port
		if in.Host != nil {
			nextHost = host
		}
		if in.Port != nil {
			nextPort = *in.Port
		}
		out = locked.target
		if nextHost == current.Host && nextPort == current.Port {
			return nil
		}
		if _, err := q.UpdateDeployTargetHostAddress(ctx, sqlc.UpdateDeployTargetHostAddressParams{
			ID: hostID, OrganizationID: org, Host: nextHost, Port: int32(nextPort), Now: s.now(), //nolint:gosec // validated range.
		}); err != nil {
			return asTargetConflict(err)
		}
		if out, err = s.bumpAfterHostChange(ctx, q, locked); err != nil {
			return err
		}
		data := map[string]string{
			"HostId": hostID.String(), "From": addressOf(current.Host, current.Port), "To": addressOf(nextHost, nextPort),
			"Version": fmt.Sprint(out.Version),
		}
		if current.KeyPinned {
			data["DroppedFingerprint"] = current.HostKeyFingerprint
		}
		return s.auditTarget(ctx, q, p, eventbus.TargetHostUpdated, locked.env, out, data)
	})
	return out, err
}

func (s *Service) RemoveHost(ctx context.Context, org, project uuid.UUID, envName, name string, hostID uuid.UUID, version int64, factor auth.Factor) (models.Target, error) {
	p, err := principalOf(ctx)
	if err != nil {
		return models.Target{}, err
	}
	if err := s.confirm(ctx, p, factor); err != nil {
		return models.Target{}, err
	}
	var out models.Target
	err = dbtx.Run(ctx, s.db, func(q *sqlc.Queries) error {
		locked, err := s.lockTarget(ctx, q, org, project, envName, name)
		if err != nil {
			return err
		}
		if err := locked.checkVersion(version); err != nil {
			return err
		}
		current, ok := findHost(locked.target, hostID)
		if !ok {
			return apperrors.NotFound()
		}
		if len(locked.target.Hosts) == 1 {
			return apperrors.ValidationFailed(invalid(fieldHosts))
		}
		if _, err := q.RemoveDeployTargetHost(ctx, sqlc.RemoveDeployTargetHostParams{ID: hostID, OrganizationID: org, Now: s.now()}); err != nil {
			return err
		}
		rest := slices.DeleteFunc(hostIDs(locked.target.Hosts), func(id uuid.UUID) bool { return id == hostID })
		if err := s.renumber(ctx, q, org, locked.row.Id, rest); err != nil {
			return err
		}
		locked.target.Hosts = slices.DeleteFunc(slices.Clone(locked.target.Hosts), func(h models.TargetHost) bool { return h.ID == hostID })
		if out, err = s.bumpAfterHostChange(ctx, q, locked); err != nil {
			return err
		}
		data := map[string]string{"HostId": hostID.String(), "Address": addressOf(current.Host, current.Port), "Version": fmt.Sprint(out.Version)}
		if current.KeyPinned {
			data["DroppedFingerprint"] = current.HostKeyFingerprint
		}
		return s.auditTarget(ctx, q, p, eventbus.TargetHostRemoved, locked.env, out, data)
	})
	return out, err
}

func (s *Service) ReorderHosts(ctx context.Context, org, project uuid.UUID, envName, name string, version int64, ids []uuid.UUID, factor auth.Factor) (models.Target, error) {
	p, err := principalOf(ctx)
	if err != nil {
		return models.Target{}, err
	}
	if err := s.confirm(ctx, p, factor); err != nil {
		return models.Target{}, err
	}
	var out models.Target
	err = dbtx.Run(ctx, s.db, func(q *sqlc.Queries) error {
		locked, err := s.lockTarget(ctx, q, org, project, envName, name)
		if err != nil {
			return err
		}
		if err := locked.checkVersion(version); err != nil {
			return err
		}
		current := hostIDs(locked.target.Hosts)
		if !sameSet(current, ids) {
			return apperrors.ValidationFailed(invalid(fieldHostIDs))
		}
		if slices.Equal(current, ids) {
			out = locked.target
			return nil
		}
		if err := s.renumber(ctx, q, org, locked.row.Id, ids); err != nil {
			return err
		}
		if out, err = s.touchedTarget(ctx, q, locked.row, s.now()); err != nil {
			return err
		}
		return s.auditTarget(ctx, q, p, eventbus.TargetUpdated, locked.env, out, map[string]string{"Fields": "HostOrder", "Version": fmt.Sprint(out.Version)})
	})
	return out, err
}

func (s *Service) touchedTarget(ctx context.Context, q *sqlc.Queries, row sqlc.DeployTarget, now time.Time) (models.Target, error) {
	touched, err := q.TouchDeployTarget(ctx, sqlc.TouchDeployTargetParams{ID: row.Id, OrganizationID: row.OrganizationId, Now: now})
	if err != nil {
		return models.Target{}, err
	}
	return s.oneTarget(ctx, q, touched)
}

func sameSet(a, b []uuid.UUID) bool {
	if len(a) != len(b) {
		return false
	}
	seen := map[uuid.UUID]bool{}
	for _, id := range b {
		if seen[id] {
			return false
		}
		seen[id] = true
	}
	for _, id := range a {
		if !seen[id] {
			return false
		}
	}
	return true
}
