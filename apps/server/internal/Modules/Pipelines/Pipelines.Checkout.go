package pipelines

import (
	"bytes"
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	sqlc "github.com/Khaymah/dhow/apps/server/internal/Database/Sqlc"
	"github.com/Khaymah/dhow/internal/gitproof"
	"github.com/Khaymah/dhow/internal/gitsource"
	"github.com/Khaymah/dhow/internal/jobspec"
	"github.com/Khaymah/dhow/internal/pipelinespec"
	runnerv1 "github.com/Khaymah/dhow/packages/proto/gen/go/dhow/runner/v1"
)

const (
	discoveryTimeout = 300
	discoveryMemory  = 256 << 20
	discoveryDisk    = 512 << 20
	discoveryCPU     = 500
	discoveryPids    = 256
	foundTrue        = "true"
	foundFalse       = "false"
)

var errRepositoryUnavailable = errors.New("the repository of the run cannot be checked out")

func transportOf(u gitsource.URL) runnerv1.CheckoutTransport {
	if u.Transport == gitsource.TransportSSH {
		return runnerv1.CheckoutTransport_CHECKOUT_TRANSPORT_SSH
	}
	return runnerv1.CheckoutTransport_CHECKOUT_TRANSPORT_HTTPS
}

func (s *Service) checkoutFor(
	ctx context.Context, q *sqlc.Queries, run sqlc.PipelineRun, ref, sha string,
) (*runnerv1.Checkout, []*runnerv1.SecretRef, error) {
	if run.RepositoryId == nil || run.Provider.String != providerGeneric {
		return nil, nil, nil
	}
	repo, err := q.GetCheckoutRepository(ctx, sqlc.GetCheckoutRepositoryParams{ID: *run.RepositoryId, OrganizationID: run.OrganizationId})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil, errRepositoryUnavailable
	}
	if err != nil {
		return nil, nil, err
	}
	u, err := gitsource.Parse(repo.CloneUrl)
	if err != nil {
		return nil, nil, fmt.Errorf("%w: %w", errRepositoryUnavailable, err)
	}
	co := &runnerv1.Checkout{
		RepositoryUrl: u.String(), Ref: ref, Sha: sha, Transport: transportOf(u), Depth: jobspec.DefaultCheckoutDepth,
	}
	if u.Transport != gitsource.TransportSSH {
		return co, nil, nil
	}
	if !repo.HostKey.Valid || !repo.HasDeployKey {
		return nil, nil, errRepositoryUnavailable
	}
	co.KnownHost = u.KnownHostsPattern() + " " + repo.HostKey.String
	co.CredentialSecret = jobspec.CheckoutSecret
	secret := &runnerv1.SecretRef{Name: jobspec.CheckoutSecret, Target: &runnerv1.SecretRef_AsFile{AsFile: jobspec.CheckoutSecretFile}}
	return co, []*runnerv1.SecretRef{secret}, nil
}

func definitionTarget(run sqlc.PipelineRun) (string, string) {
	if run.IsFork {
		return run.BaseRef.String, run.BaseSha.String
	}
	return run.Ref, run.DefinitionSha.String
}

func (s *Service) discoverySpec(ctx context.Context, q *sqlc.Queries, run sqlc.PipelineRun) (*runnerv1.JobSpec, error) {
	if s.checkoutImage == "" {
		return nil, ErrCheckoutUnavailable
	}
	ref, sha := definitionTarget(run)
	co, secrets, err := s.checkoutFor(ctx, q, run, ref, sha)
	if err != nil {
		return nil, err
	}
	if co == nil {
		return nil, errRepositoryUnavailable
	}
	return &runnerv1.JobSpec{
		Image: s.checkoutImage, Checkout: co, Secrets: secrets, TimeoutSeconds: discoveryTimeout, Network: &runnerv1.Network{},
		Resources: &runnerv1.Resources{CpuMillis: discoveryCPU, MemoryBytes: discoveryMemory, DiskBytes: discoveryDisk, Pids: discoveryPids},
		Steps:     []*runnerv1.Step{{Name: discoveryName, Run: gitproof.Script, Env: map[string]string{gitproof.EnvPath: run.DefinitionPath}}},
	}, nil
}

func (s *Service) readDiscovered(ctx context.Context, q *sqlc.Queries, run sqlc.PipelineRun, job uuid.UUID) (gitproof.Proof, error) {
	chunks, err := q.DiscoveryLog(ctx, job)
	if err != nil {
		return gitproof.Proof{}, err
	}
	var log bytes.Buffer
	for _, c := range chunks {
		if log.Len()+len(c) > gitproof.MaxLogBytes {
			return gitproof.Proof{}, fmt.Errorf("%w: the discovery log is too large", gitproof.ErrUnverified)
		}
		log.Write(c)
	}
	objects, err := gitproof.ParseLog(log.Bytes())
	if err != nil {
		return gitproof.Proof{}, err
	}
	_, sha := definitionTarget(run)
	return gitproof.Prove(sha, run.DefinitionPath, objects, pipelinespec.MaxFileBytes)
}

type CheckoutSource struct {
	ProjectID    uuid.UUID
	RepositoryID uuid.UUID
}

func (s *Service) CheckoutSourceOfJob(ctx context.Context, org, job uuid.UUID) (CheckoutSource, error) {
	row, err := s.queries().CheckoutRepositoryOfJob(ctx, sqlc.CheckoutRepositoryOfJobParams{JobID: job, OrganizationID: org})
	if errors.Is(err, pgx.ErrNoRows) {
		return CheckoutSource{}, errRepositoryUnavailable
	}
	if err != nil {
		return CheckoutSource{}, err
	}
	return CheckoutSource{ProjectID: row.ProjectId, RepositoryID: row.RepositoryId}, nil
}
