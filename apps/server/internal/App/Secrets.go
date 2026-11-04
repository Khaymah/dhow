package app

import (
	"context"
	"errors"
	"log/slog"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"

	eventbus "github.com/Khaymah/dhow/apps/server/internal/Core/EventBus"
	secretstore "github.com/Khaymah/dhow/apps/server/internal/Core/SecretStore"
	jobs "github.com/Khaymah/dhow/apps/server/internal/Modules/Jobs"
	pipelines "github.com/Khaymah/dhow/apps/server/internal/Modules/Pipelines"
)

const (
	deliveredTarget   = "job"
	deliveredValueMax = 256
)

type pipelineSecrets struct {
	runs  *pipelines.Service
	store secretstore.Store
	bus   *eventbus.Bus
	log   *slog.Logger
}

var _ jobs.SecretResolver = (*pipelineSecrets)(nil)

func (p *pipelineSecrets) Accepts(_ context.Context, origin jobs.Origin, _ []string) error {
	if origin.Kind != jobs.OriginPipeline || origin.ID == uuid.Nil {
		return jobs.ErrSecretsUnavailable
	}
	return nil
}

func (p *pipelineSecrets) Resolve(ctx context.Context, req jobs.SecretRequest) (map[string][]byte, error) {
	scope, err := p.runs.SecretScopeOfJob(ctx, req.OrganizationID, req.JobID)
	if errors.Is(err, pipelines.ErrNoSecretScope) {
		return nil, jobs.ErrSecretsUnavailable
	}
	if err != nil {
		return nil, err
	}
	if scope.IsFork {
		p.log.WarnContext(ctx, "a job of a fork asked for secrets at start and got none", "job_id", req.JobID.String())
		return nil, jobs.ErrSecretsUnavailable
	}
	list, err := p.store.Resolve(ctx, secretstore.Request{
		OrganizationID: req.OrganizationID, ProjectID: scope.ProjectID, Names: req.Names, PullRequest: scope.PullRequest,
	})
	if errors.Is(err, secretstore.ErrMissingName) || errors.Is(err, secretstore.ErrFork) ||
		errors.Is(err, secretstore.ErrTooMany) || errors.Is(err, secretstore.ErrTooLarge) {
		return nil, jobs.ErrSecretsUnavailable
	}
	if err != nil {
		return nil, err
	}
	out := make(map[string][]byte, len(list))
	for _, s := range list {
		out[s.Name] = s.Value
	}
	if err := p.audit(ctx, req, scope, list); err != nil {
		secretstore.Wipe(list)
		return nil, err
	}
	return out, nil
}

func deliveredList(list []secretstore.Secret) (string, bool) {
	parts := make([]string, 0, len(list))
	for _, s := range list {
		parts = append(parts, s.Name+":"+s.Scope+":v"+strconv.FormatInt(int64(s.Version), 10))
	}
	v := strings.Join(parts, ",")
	if len(v) <= deliveredValueMax {
		return v, false
	}
	end := deliveredValueMax
	for end > 0 && !utf8.RuneStart(v[end]) {
		end--
	}
	return v[:end], true
}

func (p *pipelineSecrets) audit(ctx context.Context, req jobs.SecretRequest, scope pipelines.SecretScope, list []secretstore.Secret) error {
	value, cut := deliveredList(list)
	data := map[string]string{
		"Secrets": value, "ProjectId": scope.ProjectID.String(), "RunId": scope.RunID.String(), "AttemptId": req.AttemptID.String(),
	}
	if cut {
		data["Truncated"] = "true"
	}
	return p.bus.Publish(ctx, eventbus.Event{Name: eventbus.JobSecretsDelivered, Payload: eventbus.Audited{
		OrganizationIDs: []uuid.UUID{req.OrganizationID}, TargetType: deliveredTarget, TargetID: req.JobID.String(), Data: data,
	}})
}
