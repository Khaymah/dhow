package environments

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"

	auth "github.com/Khaymah/dhow/apps/server/internal/Core/Auth"
	apperrors "github.com/Khaymah/dhow/apps/server/internal/Core/Errors"
	eventbus "github.com/Khaymah/dhow/apps/server/internal/Core/EventBus"
	sqlc "github.com/Khaymah/dhow/apps/server/internal/Database/Sqlc"
	dbtx "github.com/Khaymah/dhow/apps/server/internal/Database/Tx"
	models "github.com/Khaymah/dhow/apps/server/internal/Modules/Environments/Models"
)

const (
	constraintTargetName  = "UX_DeployTargets_Environment_Name"
	constraintHostAddress = "UX_DeployTargetHosts_Target_Address"
	constraintHostPath    = "UX_DeployTargetHosts_Address_Path"
	targetEntity          = "deploy_target"
)

func uniqueConstraint(err error) string {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == uniqueViolation {
		return pgErr.ConstraintName
	}
	return ""
}

func asTargetConflict(err error) error {
	switch uniqueConstraint(err) {
	case constraintTargetName:
		return apperrors.TargetExists()
	case constraintHostAddress, constraintHostPath:
		return apperrors.TargetDuplicate()
	default:
		return err
	}
}

func targetHostOf(row sqlc.DeployTargetHost) models.TargetHost {
	h := models.TargetHost{
		ID: row.Id, Position: int(row.Position), Host: row.Host, Port: int(row.Port), KeyPinned: row.HostKey.Valid,
		HostKeyFingerprint: row.HostKeyFingerprint.String, HostKeyConfirmedAt: row.HostKeyConfirmedAt,
		HostKeyConfirmedByUserID: row.HostKeyConfirmedBy, PendingHostKeyFingerprint: row.PendingFingerprint.String,
	}
	return h
}

func targetOf(row sqlc.DeployTarget, hosts []sqlc.DeployTargetHost) (models.Target, error) {
	var settings models.TargetSettings
	if err := json.Unmarshal(row.Settings, &settings); err != nil {
		return models.Target{}, fmt.Errorf("decode the settings of target %s: %w", row.Id, err)
	}
	t := models.Target{
		ID: row.Id, EnvironmentID: row.EnvironmentId, Name: row.Name, Kind: row.Kind, Username: row.Username,
		KeySecretName: row.KeySecretName.String, PublicKey: row.PublicKey.String, DeployPath: row.DeployPath,
		KeepReleases: int(row.KeepReleases), Settings: settings, AllowedImages: row.AllowedImages, Hosts: []models.TargetHost{},
		NotReady: []string{}, Version: row.Version, CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt,
	}
	if t.AllowedImages == nil {
		t.AllowedImages = []string{}
	}
	unpinned := false
	for _, h := range hosts {
		view := targetHostOf(h)
		unpinned = unpinned || !view.KeyPinned
		t.Hosts = append(t.Hosts, view)
	}
	if unpinned || len(hosts) == 0 {
		t.NotReady = append(t.NotReady, models.NotReadyHostKey)
	}
	if t.KeySecretName == "" {
		t.NotReady = append(t.NotReady, models.NotReadyKey)
	}
	t.Ready = len(t.NotReady) == 0
	return t, nil
}

func (s *Service) targetsOf(ctx context.Context, q *sqlc.Queries, org uuid.UUID, rows ...sqlc.DeployTarget) ([]models.Target, error) {
	ids := make([]uuid.UUID, 0, len(rows))
	for _, r := range rows {
		ids = append(ids, r.Id)
	}
	hosts, err := q.ListDeployTargetHosts(ctx, sqlc.ListDeployTargetHostsParams{OrganizationID: org, TargetIds: ids})
	if err != nil {
		return nil, err
	}
	byTarget := map[uuid.UUID][]sqlc.DeployTargetHost{}
	for _, h := range hosts {
		byTarget[h.TargetId] = append(byTarget[h.TargetId], h)
	}
	out := make([]models.Target, 0, len(rows))
	for _, r := range rows {
		t, err := targetOf(r, byTarget[r.Id])
		if err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, nil
}

func (s *Service) oneTarget(ctx context.Context, q *sqlc.Queries, row sqlc.DeployTarget) (models.Target, error) {
	list, err := s.targetsOf(ctx, q, row.OrganizationId, row)
	if err != nil {
		return models.Target{}, err
	}
	return list[0], nil
}

func (s *Service) environmentRow(ctx context.Context, q *sqlc.Queries, org, project uuid.UUID, name string) (sqlc.Environment, error) {
	if !ValidName(name) {
		return sqlc.Environment{}, apperrors.NotFound()
	}
	row, err := q.GetEnvironment(ctx, sqlc.GetEnvironmentParams{ProjectID: project, OrganizationID: org, Name: name})
	if err != nil {
		return sqlc.Environment{}, notFoundOr(err)
	}
	return row, nil
}

func (s *Service) ListTargets(ctx context.Context, org, project uuid.UUID, envName string, page, limit int) (models.TargetPage, error) {
	q := s.queries()
	env, err := s.environmentRow(ctx, q, org, project, envName)
	if err != nil {
		return models.TargetPage{}, err
	}
	rows, err := q.ListDeployTargets(ctx, sqlc.ListDeployTargetsParams{EnvironmentID: env.Id, OrganizationID: org})
	if err != nil {
		return models.TargetPage{}, err
	}
	total := int64(len(rows))
	from := min((page-1)*limit, len(rows))
	rows = rows[from:min(from+limit, len(rows))]
	if len(rows) == 0 {
		return models.TargetPage{Items: []models.Target{}, Total: total}, nil
	}
	items, err := s.targetsOf(ctx, q, org, rows...)
	if err != nil {
		return models.TargetPage{}, err
	}
	return models.TargetPage{Items: items, Total: total}, nil
}

func (s *Service) GetTarget(ctx context.Context, org, project uuid.UUID, envName, name string) (models.Target, error) {
	if !ValidName(name) {
		return models.Target{}, apperrors.NotFound()
	}
	q := s.queries()
	env, err := s.environmentRow(ctx, q, org, project, envName)
	if err != nil {
		return models.Target{}, err
	}
	row, err := q.GetDeployTarget(ctx, sqlc.GetDeployTargetParams{EnvironmentID: env.Id, OrganizationID: org, Name: name})
	if err != nil {
		return models.Target{}, notFoundOr(err)
	}
	return s.oneTarget(ctx, q, row)
}

type lockedTarget struct {
	env    models.Environment
	row    sqlc.DeployTarget
	target models.Target
}

func (s *Service) lockTarget(ctx context.Context, q *sqlc.Queries, org, project uuid.UUID, envName, name string) (lockedTarget, error) {
	if !ValidName(name) {
		return lockedTarget{}, apperrors.NotFound()
	}
	env, _, err := s.lock(ctx, q, org, project, envName)
	if err != nil {
		return lockedTarget{}, err
	}
	if err := s.checkProject(ctx, q, org, project); err != nil {
		return lockedTarget{}, err
	}
	row, err := q.LockDeployTarget(ctx, sqlc.LockDeployTargetParams{EnvironmentID: env.ID, OrganizationID: org, Name: name})
	if err != nil {
		return lockedTarget{}, notFoundOr(err)
	}
	t, err := s.oneTarget(ctx, q, row)
	return lockedTarget{env: env, row: row, target: t}, err
}

func (l lockedTarget) checkVersion(version int64) error {
	if l.target.Version != version {
		return apperrors.TargetVersionConflict()
	}
	return nil
}

func (s *Service) auditTarget(ctx context.Context, q *sqlc.Queries, p auth.Principal, event string, env models.Environment, t models.Target, data map[string]string) error {
	actor := p.UserID
	data["Name"], data["Environment"] = t.Name, env.Name
	data["ProjectId"], data["EnvironmentId"] = env.ProjectID.String(), env.ID.String()
	return s.bus.Publish(ctx, eventbus.Event{Name: event, Payload: eventbus.Audited{
		OrganizationIDs: []uuid.UUID{env.OrganizationID}, ActorUserID: &actor, TargetType: targetEntity,
		TargetID: t.ID.String(), IP: auth.RequestMetaFrom(ctx).IPString(), Data: data, Tx: q,
	}})
}

func (s *Service) requireSecret(ctx context.Context, q *sqlc.Queries, env models.Environment, name string) error {
	if name == "" {
		return nil
	}
	found, err := q.EnvironmentSecretExists(ctx, sqlc.EnvironmentSecretExistsParams{OrganizationID: env.OrganizationID, EnvironmentID: &env.ID, Name: name})
	if err != nil {
		return err
	}
	if !found {
		return apperrors.TargetKeyNotFound(invalid(fieldKeyName))
	}
	return nil
}

func settingsJSON(st models.TargetSettings) ([]byte, error) {
	raw, err := json.Marshal(st)
	if err != nil {
		return nil, err
	}
	if len(raw) > models.MaxSettingsBytes {
		return nil, apperrors.ValidationFailed(invalid(fieldSettings))
	}
	return raw, nil
}

func textOf(v string) pgtype.Text { return pgtype.Text{String: v, Valid: v != ""} }

func (s *Service) validateHostInputs(hosts []models.HostInput) ([]string, []int, error) {
	if len(hosts) < 1 || len(hosts) > models.MaxTargetHosts {
		return nil, nil, apperrors.ValidationFailed(invalid(fieldHosts))
	}
	names, ports := make([]string, 0, len(hosts)), make([]int, 0, len(hosts))
	for i, h := range hosts {
		host, ae := normalizeHost(h.Host, hostField(i, fieldHost))
		if ae != nil {
			return nil, nil, ae
		}
		port := models.DefaultSSHPort
		if h.Port != nil {
			port = *h.Port
		}
		if !validPort(port) {
			return nil, nil, apperrors.ValidationFailed(invalid(hostField(i, fieldPort)))
		}
		for j := range names {
			if names[j] == host && ports[j] == port {
				return nil, nil, apperrors.TargetDuplicate()
			}
		}
		names, ports = append(names, host), append(ports, port)
	}
	return names, ports, nil
}

func (s *Service) CreateTarget(ctx context.Context, org, project uuid.UUID, envName string, in models.CreateTargetInput, factor auth.Factor) (models.Target, error) {
	p, err := principalOf(ctx)
	if err != nil {
		return models.Target{}, err
	}
	fields := targetFields{
		kind: in.Kind, username: in.Username, deployPath: in.DeployPath, keepReleases: models.DefaultKeepReleases, hosts: len(in.Hosts),
	}
	if in.KeySecretName != nil {
		fields.keySecretName = *in.KeySecretName
	}
	if in.KeepReleases != nil {
		fields.keepReleases = *in.KeepReleases
	}
	if in.Settings != nil {
		fields.settings = *in.Settings
	}
	if in.AllowedImages != nil {
		fields.images = uniqueInOrder(*in.AllowedImages)
	}
	if !ValidName(in.Name) {
		return models.Target{}, apperrors.ValidationFailed(invalid(fieldName))
	}
	if ae := fields.validate(); ae != nil {
		return models.Target{}, ae
	}
	names, ports, err := s.validateHostInputs(in.Hosts)
	if err != nil {
		return models.Target{}, err
	}
	if err := s.confirm(ctx, p, factor); err != nil {
		return models.Target{}, err
	}
	settings, err := settingsJSON(normalizeSettings(in.Kind, in.Name, fields.settings))
	if err != nil {
		return models.Target{}, err
	}
	var out models.Target
	err = dbtx.Run(ctx, s.db, func(q *sqlc.Queries) error {
		env, _, err := s.lock(ctx, q, org, project, envName)
		if err != nil {
			return err
		}
		if err := s.requireSecret(ctx, q, env, fields.keySecretName); err != nil {
			return err
		}
		if err := s.checkProject(ctx, q, org, project); err != nil {
			return err
		}
		if err := s.checkKeyNameFree(ctx, q, env, in.Name); err != nil {
			return err
		}
		now := s.now()
		row, err := q.InsertDeployTarget(ctx, sqlc.InsertDeployTargetParams{
			OrganizationID: org, ProjectID: project, EnvironmentID: env.ID, Name: in.Name, Kind: in.Kind, Username: in.Username,
			KeySecretName: textOf(fields.keySecretName), DeployPath: in.DeployPath, KeepReleases: int32(fields.keepReleases), //nolint:gosec // validated to 2..50.
			Settings: settings, AllowedImages: nonNil(fields.images), CreatedByUserID: p.UserID, Now: now,
		})
		if err != nil {
			return asTargetConflict(err)
		}
		for i := range names {
			if _, err := q.InsertDeployTargetHost(ctx, sqlc.InsertDeployTargetHostParams{
				OrganizationID: org, TargetID: row.Id, Position: int32(i + 1), Host: names[i], Port: int32(ports[i]), //nolint:gosec // validated ranges.
				DeployPath: in.DeployPath, Now: now,
			}); err != nil {
				return asTargetConflict(err)
			}
		}
		if out, err = s.oneTarget(ctx, q, row); err != nil {
			return err
		}
		return s.auditTarget(ctx, q, p, eventbus.TargetCreated, env, out, map[string]string{
			"Kind": out.Kind, "Username": out.Username, "DeployPath": out.DeployPath, "Hosts": fmt.Sprint(len(out.Hosts)),
			"KeyName": out.KeySecretName, "Version": fmt.Sprint(out.Version),
		})
	})
	return out, err
}

func nonNil(v []string) []string {
	if v == nil {
		return []string{}
	}
	return v
}

func (s *Service) UpdateTarget(ctx context.Context, org, project uuid.UUID, envName, name string, in models.UpdateTargetInput, factor auth.Factor) (models.Target, error) {
	p, err := principalOf(ctx)
	if err != nil {
		return models.Target{}, err
	}
	if in.Username == nil && in.KeySecretName == nil && in.DeployPath == nil && in.KeepReleases == nil && in.Settings == nil && in.AllowedImages == nil {
		return models.Target{}, apperrors.BadRequest()
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
		out = locked.target
		next, changed, ae := applyTargetUpdate(locked.target, in)
		if ae != nil {
			return ae
		}
		if len(changed) == 0 {
			return nil
		}
		if slices.Contains(changed, fieldKeyName) {
			if err := s.requireSecret(ctx, q, locked.env, next.KeySecretName); err != nil {
				return err
			}
		}
		if out, err = s.saveTarget(ctx, q, locked, next, changed); err != nil {
			return err
		}
		return s.auditTarget(ctx, q, p, eventbus.TargetUpdated, locked.env, out, map[string]string{
			"Fields": strings.Join(changed, ","), "Version": fmt.Sprint(out.Version),
		})
	})
	return out, err
}

func applyTargetUpdate(current models.Target, in models.UpdateTargetInput) (models.Target, []string, *apperrors.AppError) {
	next := current
	var changed []string
	set := func(field string, differs bool) {
		if differs {
			changed = append(changed, field)
		}
	}
	if in.Username != nil {
		set(fieldUsername, *in.Username != current.Username)
		next.Username = *in.Username
	}
	if in.KeySecretName != nil {
		set(fieldKeyName, *in.KeySecretName != current.KeySecretName)
		next.KeySecretName = *in.KeySecretName
	}
	if in.DeployPath != nil {
		set(fieldDeployPath, *in.DeployPath != current.DeployPath)
		next.DeployPath = *in.DeployPath
	}
	if in.KeepReleases != nil {
		set(fieldKeepReleases, *in.KeepReleases != current.KeepReleases)
		next.KeepReleases = *in.KeepReleases
	}
	if in.Settings != nil {
		next.Settings = normalizeSettings(current.Kind, current.Name, *in.Settings)
		set(fieldSettings, canonical(next.Settings) != canonical(current.Settings))
	}
	if in.AllowedImages != nil {
		next.AllowedImages = uniqueInOrder(*in.AllowedImages)
		set(fieldAllowedImages, !slices.Equal(next.AllowedImages, current.AllowedImages))
	}
	fields := targetFields{
		kind: next.Kind, username: next.Username, keySecretName: next.KeySecretName, deployPath: next.DeployPath,
		keepReleases: next.KeepReleases, settings: next.Settings, images: next.AllowedImages, hosts: len(next.Hosts),
	}
	return next, changed, fields.validate()
}

func (s *Service) saveTarget(ctx context.Context, q *sqlc.Queries, locked lockedTarget, next models.Target, changed []string) (models.Target, error) {
	settings, err := settingsJSON(next.Settings)
	if err != nil {
		return models.Target{}, err
	}
	now := s.now()
	row, err := q.UpdateDeployTarget(ctx, sqlc.UpdateDeployTargetParams{
		ID: locked.row.Id, OrganizationID: locked.row.OrganizationId, Username: next.Username, KeySecretName: textOf(next.KeySecretName),
		DeployPath: next.DeployPath, KeepReleases: int32(next.KeepReleases), Settings: settings, //nolint:gosec // validated to 2..50.
		AllowedImages: nonNil(next.AllowedImages), Now: now,
	})
	if err != nil {
		return models.Target{}, asTargetConflict(err)
	}
	if slices.Contains(changed, fieldDeployPath) {
		if err := q.SyncDeployTargetHostPaths(ctx, sqlc.SyncDeployTargetHostPathsParams{
			TargetID: row.Id, OrganizationID: row.OrganizationId, DeployPath: row.DeployPath, Now: now,
		}); err != nil {
			return models.Target{}, asTargetConflict(err)
		}
	}
	return s.oneTarget(ctx, q, row)
}

func (s *Service) RemoveTarget(ctx context.Context, org, project uuid.UUID, envName, name string, factor auth.Factor) (models.Target, error) {
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
		now := s.now()
		if _, err := q.RemoveDeployTarget(ctx, sqlc.RemoveDeployTargetParams{ID: locked.row.Id, OrganizationID: org, Now: now}); err != nil {
			return err
		}
		if err := q.RemoveDeployTargetHosts(ctx, sqlc.RemoveDeployTargetHostsParams{TargetID: locked.row.Id, OrganizationID: org, Now: now}); err != nil {
			return err
		}
		out = locked.target
		return s.auditTarget(ctx, q, p, eventbus.TargetRemoved, locked.env, out, map[string]string{
			"Hosts": fmt.Sprint(len(out.Hosts)), "Version": fmt.Sprint(out.Version),
		})
	})
	return out, err
}

func (s *Service) removeTargetsOf(ctx context.Context, q *sqlc.Queries, org, env uuid.UUID, now time.Time) error {
	if err := q.RemoveEnvironmentDeployTargetHosts(ctx, sqlc.RemoveEnvironmentDeployTargetHostsParams{
		EnvironmentID: env, OrganizationID: org, Now: now,
	}); err != nil {
		return err
	}
	return q.RemoveEnvironmentDeployTargets(ctx, sqlc.RemoveEnvironmentDeployTargetsParams{EnvironmentID: env, OrganizationID: org, Now: now})
}
