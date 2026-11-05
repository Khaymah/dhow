package environments

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	deployments "github.com/Khaymah/dhow/apps/server/internal/Core/Deployments"
	sqlc "github.com/Khaymah/dhow/apps/server/internal/Database/Sqlc"
	dbtx "github.com/Khaymah/dhow/apps/server/internal/Database/Tx"
	models "github.com/Khaymah/dhow/apps/server/internal/Modules/Environments/Models"
	"github.com/Khaymah/dhow/internal/jobspec"
)

var (
	ErrTargetChanged    = errors.New("the deploy target or the host key of one of its hosts changed after the job was queued")
	ErrTargetKeyMissing = errors.New("the secret that holds the key of the deploy target no longer exists on the environment")
)

type Targets struct{ s *Service }

var _ deployments.Targets = (*Targets)(nil)

func (s *Service) Targets() *Targets { return &Targets{s: s} }

func deliverySettings(raw []byte) (deployments.TargetSettings, error) {
	var st models.TargetSettings
	if err := json.Unmarshal(raw, &st); err != nil {
		return deployments.TargetSettings{}, fmt.Errorf("decode the settings of a deploy target: %w", err)
	}
	out := deployments.TargetSettings{BatchSize: models.DefaultBatchSize}
	if st.Rolling != nil {
		out.BatchSize, out.BatchPauseSeconds = max(st.Rolling.BatchSize, models.DefaultBatchSize), st.Rolling.BatchPauseSeconds
	}
	if st.SSH != nil {
		out.Hooks = deployments.Hooks{Reload: st.SSH.ReloadCommand, PreSwitch: st.SSH.PreSwitchCommand, PostSwitch: st.SSH.PostSwitchCommand}
		out.Shared = st.SSH.Shared
	}
	if st.Compose != nil {
		out.ComposeProject, out.ComposeFile, out.AllowUnsafe = st.Compose.ProjectName, st.Compose.ComposeFile, st.Compose.AllowUnsafeCompose
	}
	return out, nil
}

func deliveryOf(row sqlc.DeployTarget, hosts []sqlc.DeployTargetHost, keyVersion int32) (deployments.DeployTarget, error) {
	settings, err := deliverySettings(row.Settings)
	if err != nil {
		return deployments.DeployTarget{}, err
	}
	t := deployments.DeployTarget{
		ID: row.Id, Name: row.Name, Kind: row.Kind, Username: row.Username, DeployPath: row.DeployPath,
		KeySecretName: row.KeySecretName.String, KeyFound: keyVersion > 0, KeyVersion: keyVersion, KeepReleases: int(row.KeepReleases), Version: row.Version,
		AllowedImages: row.AllowedImages, Settings: settings, Hosts: make([]deployments.TargetHost, 0, len(hosts)),
	}
	for _, h := range hosts {
		t.Hosts = append(t.Hosts, deployments.TargetHost{ID: h.Id, Position: int(h.Position), Host: h.Host, Port: int(h.Port), HostKey: h.HostKey.String})
	}
	return t, nil
}

func (s *Service) deliveryParts(ctx context.Context, q *sqlc.Queries, row sqlc.DeployTarget) (deployments.DeployTarget, error) {
	hosts, err := q.ListDeployTargetHosts(ctx, sqlc.ListDeployTargetHostsParams{OrganizationID: row.OrganizationId, TargetIds: []uuid.UUID{row.Id}})
	if err != nil {
		return deployments.DeployTarget{}, err
	}
	version := int32(0)
	if row.KeySecretName.Valid {
		version, err = q.EnvironmentSecretVersion(ctx, sqlc.EnvironmentSecretVersionParams{
			OrganizationID: row.OrganizationId, EnvironmentID: &row.EnvironmentId, Name: row.KeySecretName.String,
		})
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return deployments.DeployTarget{}, err
		}
	}
	return deliveryOf(row, hosts, version)
}

func (t *Targets) Find(ctx context.Context, q *sqlc.Queries, org, environment uuid.UUID, name string) (deployments.DeployTarget, bool, error) {
	if !ValidName(name) {
		return deployments.DeployTarget{}, false, nil
	}
	row, err := q.GetDeployTarget(ctx, sqlc.GetDeployTargetParams{EnvironmentID: environment, OrganizationID: org, Name: name})
	if errors.Is(err, pgx.ErrNoRows) {
		return deployments.DeployTarget{}, false, nil
	}
	if err != nil {
		return deployments.DeployTarget{}, false, err
	}
	out, err := t.s.deliveryParts(ctx, q, row)
	return out, err == nil, err
}

func (t *Targets) KeyNames(ctx context.Context, q *sqlc.Queries, org, environment uuid.UUID) ([]string, error) {
	return q.ListDeployTargetKeyNames(ctx, sqlc.ListDeployTargetKeyNamesParams{OrganizationID: org, EnvironmentID: environment})
}

func (s *Service) TargetKeyNames(ctx context.Context, org, environment uuid.UUID) ([]string, error) {
	return s.Targets().KeyNames(ctx, s.queries(), org, environment)
}

type TargetPin struct {
	Key     string
	Version int32
}

type PinnedHost struct {
	ID             uuid.UUID
	Host           string
	Port           int
	KnownHostsLine string
}

type TargetCheck struct {
	OrganizationID uuid.UUID
	EnvironmentID  uuid.UUID
	TargetID       uuid.UUID
	Version        int64
	Hosts          []PinnedHost
}

func (c TargetCheck) matches(t deployments.DeployTarget) bool {
	if t.Version != c.Version || len(t.Hosts) != len(c.Hosts) || t.KeySecretName == "" {
		return false
	}
	for i, h := range t.Hosts {
		want := c.Hosts[i]
		if h.ID != want.ID || h.Host != want.Host || h.Port != want.Port || h.HostKey == "" ||
			jobspec.KnownHostsLine(h.Host, h.Port, h.HostKey) != want.KnownHostsLine {
			return false
		}
	}
	return true
}

func (s *Service) CheckTarget(ctx context.Context, c TargetCheck) (TargetPin, error) {
	var pin TargetPin
	err := dbtx.Run(ctx, s.db, func(q *sqlc.Queries) error {
		row, err := q.GetDeployTargetByID(ctx, sqlc.GetDeployTargetByIDParams{ID: c.TargetID, OrganizationID: c.OrganizationID, EnvironmentID: c.EnvironmentID})
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrTargetChanged
		}
		if err != nil {
			return err
		}
		current, err := s.deliveryParts(ctx, q, row)
		if err != nil {
			return err
		}
		if !c.matches(current) {
			return ErrTargetChanged
		}
		if !current.KeyFound {
			return ErrTargetKeyMissing
		}
		pin = TargetPin{Key: current.KeySecretName, Version: current.KeyVersion}
		return nil
	})
	return pin, err
}
