package app

import (
	"context"
	"errors"

	"github.com/google/uuid"

	secretstore "github.com/Khaymah/dhow/apps/server/internal/Core/SecretStore"
	environments "github.com/Khaymah/dhow/apps/server/internal/Modules/Environments"
	jobs "github.com/Khaymah/dhow/apps/server/internal/Modules/Jobs"
	pipelines "github.com/Khaymah/dhow/apps/server/internal/Modules/Pipelines"
	secretsmodels "github.com/Khaymah/dhow/apps/server/internal/Modules/Secrets/Models"
	"github.com/Khaymah/dhow/internal/jobspec"
)

func (p *pipelineSecrets) checkTargets(ctx context.Context, req jobs.SecretRequest, scope pipelines.SecretScope) ([]string, error) {
	if len(req.Targets) == 0 {
		return nil, nil
	}
	if scope.EnvironmentID == nil || p.envs == nil {
		return nil, jobs.ErrTargetChanged
	}
	keys := make([]string, 0, len(req.Targets))
	for _, t := range req.Targets {
		hosts := make([]environments.PinnedHost, 0, len(t.GetHosts()))
		for _, h := range t.GetHosts() {
			id, err := uuid.Parse(h.GetHostId())
			if err != nil {
				return nil, jobs.ErrTargetChanged
			}
			hosts = append(hosts, environments.PinnedHost{ID: id, Host: h.GetHost(), Port: int(h.GetPort()), KnownHostsLine: h.GetKnownHostsLine()})
		}
		id, err := uuid.Parse(t.GetTargetId())
		if err != nil {
			return nil, jobs.ErrTargetChanged
		}
		key, err := p.envs.CheckTarget(ctx, environments.TargetCheck{
			OrganizationID: req.OrganizationID, EnvironmentID: *scope.EnvironmentID, TargetID: id, Version: t.GetVersion(), Hosts: hosts,
		})
		switch {
		case errors.Is(err, environments.ErrTargetChanged):
			p.log.WarnContext(ctx, "the deploy target of a job changed after it was queued, so it gets no key", "job_id", req.JobID.String())
			return nil, jobs.ErrTargetChanged
		case errors.Is(err, environments.ErrTargetKeyMissing):
			p.log.WarnContext(ctx, "the key secret of the deploy target of a job is gone, so it gets no key", "job_id", req.JobID.String())
			return nil, jobs.ErrTargetKeyMissing
		case err != nil:
			return nil, err
		}
		keys = append(keys, key)
	}
	return keys, nil
}

func splitTargetKeys(names []string) ([]string, []string) {
	var targets []string
	rest := make([]string, 0, len(names))
	for _, n := range names {
		if _, ok := jobspec.ParseTargetKeySecret(n); ok {
			targets = append(targets, n)
			continue
		}
		rest = append(rest, n)
	}
	return rest, targets
}

func (p *pipelineSecrets) resolveTargetKeys(
	ctx context.Context, req jobs.SecretRequest, scope pipelines.SecretScope, reserved, keys []string, out map[string][]byte,
) ([]secretstore.Secret, error) {
	var delivered []secretstore.Secret
	for _, name := range reserved {
		n, _ := jobspec.ParseTargetKeySecret(name)
		if n >= len(keys) || keys[n] == "" {
			return delivered, jobs.ErrTargetChanged
		}
		list, err := p.store.Resolve(ctx, secretstore.Request{
			OrganizationID: req.OrganizationID, ProjectID: scope.ProjectID, EnvironmentID: scope.EnvironmentID, Names: []string{keys[n]},
			PullRequest: scope.PullRequest,
		})
		if errors.Is(err, secretstore.ErrMissingName) {
			return delivered, jobs.ErrTargetKeyMissing
		}
		if err != nil {
			return delivered, err
		}
		if len(list) != 1 || list[0].Scope != secretsmodels.ScopeEnvironment {
			secretstore.Wipe(list)
			return delivered, jobs.ErrTargetKeyMissing
		}
		out[name] = list[0].Value
		delivered = append(delivered, list[0])
	}
	return delivered, nil
}
