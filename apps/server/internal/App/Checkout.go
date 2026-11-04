package app

import (
	"context"
	"errors"
	"log/slog"
	"slices"

	"github.com/google/uuid"

	eventbus "github.com/Khaymah/dhow/apps/server/internal/Core/EventBus"
	keys "github.com/Khaymah/dhow/apps/server/internal/Core/Keys"
	ratelimit "github.com/Khaymah/dhow/apps/server/internal/Core/RateLimit"
	artifacts "github.com/Khaymah/dhow/apps/server/internal/Modules/Artifacts"
	jobs "github.com/Khaymah/dhow/apps/server/internal/Modules/Jobs"
	pipelines "github.com/Khaymah/dhow/apps/server/internal/Modules/Pipelines"
	secretsmodule "github.com/Khaymah/dhow/apps/server/internal/Modules/Secrets"
	sources "github.com/Khaymah/dhow/apps/server/internal/Modules/Sources"
	"github.com/Khaymah/dhow/internal/buildinfo"
	"github.com/Khaymah/dhow/internal/jobspec"
)

var errCheckoutChanged = errors.New("the repository address or host key changed after the job was queued")

type checkoutSecrets struct {
	next      jobs.SecretResolver
	pipelines *pipelines.Service
	sources   *sources.Service
	log       *slog.Logger
}

var _ jobs.SecretResolver = (*checkoutSecrets)(nil)

func newCheckoutSecrets(in Infra) *checkoutSecrets {
	var next jobs.SecretResolver = jobs.NoSecrets{}
	if in.Secrets != nil {
		next = in.Secrets
	}
	return &checkoutSecrets{next: next, log: in.Log}
}

func checkoutImage(in Infra) string {
	if in.CheckoutImage != "" {
		return in.CheckoutImage
	}
	return buildinfo.CheckoutImage()
}

func withoutCheckout(names []string) []string {
	return slices.DeleteFunc(slices.Clone(names), func(n string) bool { return n == jobspec.CheckoutSecret })
}

func (c *checkoutSecrets) Accepts(ctx context.Context, org uuid.UUID, names []string) error {
	rest := withoutCheckout(names)
	if len(rest) == 0 {
		return nil
	}
	return c.next.Accepts(ctx, org, rest)
}

func (c *checkoutSecrets) Resolve(ctx context.Context, req jobs.SecretRequest) (map[string][]byte, error) {
	rest := withoutCheckout(req.Names)
	out := map[string][]byte{}
	if len(rest) > 0 {
		inner := req
		inner.Names = rest
		values, err := c.next.Resolve(ctx, inner)
		if err != nil {
			return nil, err
		}
		out = values
	}
	if len(rest) == len(req.Names) {
		return out, nil
	}
	secret, err := c.checkoutCredential(ctx, req)
	if err != nil {
		for _, v := range out {
			clear(v)
		}
		c.log.WarnContext(ctx, "the checkout credential of a job could not be made", "job_id", req.JobID.String(), "error", err)
		return nil, jobs.ErrSecretsUnavailable
	}
	out[jobspec.CheckoutSecret] = secret
	return out, nil
}

func (c *checkoutSecrets) checkoutCredential(ctx context.Context, req jobs.SecretRequest) ([]byte, error) {
	if c.pipelines == nil || c.sources == nil {
		return nil, jobs.ErrSecretsUnavailable
	}
	src, err := c.pipelines.CheckoutSourceOfJob(ctx, req.OrganizationID, req.JobID)
	if err != nil {
		return nil, err
	}
	cred, err := c.sources.CheckoutCredential(ctx, req.OrganizationID, src.ProjectID, src.RepositoryID)
	if err != nil {
		return nil, err
	}
	if len(cred.Secret) == 0 {
		return nil, jobs.ErrSecretsUnavailable
	}
	co := req.Checkout
	if co.GetRepositoryUrl() == "" || cred.URL != co.GetRepositoryUrl() || cred.KnownHost != co.GetKnownHost() {
		clear(cred.Secret)
		return nil, errCheckoutChanged
	}
	return cred.Secret, nil
}

type runServices struct {
	pipelines *pipelines.Service
	sources   *sources.Service
	artifacts *artifacts.Service
	secrets   *secretsmodule.Service
}

func runsFor(
	in Infra, values *keys.Values, bus *eventbus.Bus, limiter ratelimit.Limiter, jobsService *jobs.Service, secrets *checkoutSecrets,
) (runServices, error) {
	pipelinesService := pipelinesFor(in, bus, jobsService.Queue(), limiter)
	jobsService.UseRunEvents(pipelinesService)
	sourcesService, err := sourcesFor(in, values, bus, limiter, pipelinesService)
	if err != nil {
		return runServices{}, err
	}
	artifactsService, err := artifactsFor(in)
	if err != nil {
		return runServices{}, err
	}
	secretsService, err := secretsFor(in, values, bus, pipelinesService)
	if err != nil {
		return runServices{}, err
	}
	pipelinesService.UseStatuses(sourcesService.StatusRecorder())
	pipelinesService.UseArtifacts(artifactsService)
	secrets.pipelines, secrets.sources = pipelinesService, sourcesService
	return runServices{pipelines: pipelinesService, sources: sourcesService, artifacts: artifactsService, secrets: secretsService}, nil
}
