package pipelines

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	sqlc "github.com/Khaymah/dhow/apps/server/internal/Database/Sqlc"
	"github.com/Khaymah/dhow/internal/gitsource"
	"github.com/Khaymah/dhow/internal/jobspec"
	"github.com/Khaymah/dhow/internal/pipelinespec"
	runnerv1 "github.com/Khaymah/dhow/packages/proto/gen/go/dhow/runner/v1"
)

const (
	discoveryPathEnv  = "PIPELINE_PATH"
	discoveryPrefix   = "dhow-definition:"
	discoveryTimeout  = 300
	discoveryLogBytes = 512 << 10
	discoveryMemory   = 256 << 20
	discoveryDisk     = 512 << 20
	discoveryCPU      = 500
	discoveryPids     = 256
	foundTrue         = "true"
)

var (
	errRepositoryUnavailable = errors.New("the repository of the run cannot be checked out")
	errDiscoveryOutput       = errors.New("the discovery job did not hand over the pipeline file")
)

const discoveryScript = `set -eu
f="$PIPELINE_PATH"
real=$(readlink -f -- "$f" 2>/dev/null || true)
case "$real" in
"$PWD"/*) ;;
*) printf 'dhow_found=false\n' >> "$DHOW_OUTPUT"; exit 0 ;;
esac
if [ -L "$f" ] || [ ! -f "$real" ]; then printf 'dhow_found=false\n' >> "$DHOW_OUTPUT"; exit 0; fi
if [ "$(wc -c < "$real")" -gt 262144 ]; then echo 'dhow: the pipeline file is larger than 256 KiB' >&2; exit 3; fi
printf 'dhow_found=true\ndhow_sha256=%s\n' "$(sha256sum < "$real" | cut -d ' ' -f 1)" >> "$DHOW_OUTPUT"
base64 < "$real" | sed 's/^/dhow-definition:/'
`

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
		Steps:     []*runnerv1.Step{{Name: discoveryName, Run: discoveryScript, Env: map[string]string{discoveryPathEnv: run.DefinitionPath}}},
	}, nil
}

func (s *Service) readDiscovered(ctx context.Context, q *sqlc.Queries, job uuid.UUID, digest string) ([]byte, error) {
	want, err := hex.DecodeString(digest)
	if err != nil || len(want) != sha256.Size {
		return nil, errDiscoveryOutput
	}
	chunks, err := q.DiscoveryLog(ctx, job)
	if err != nil {
		return nil, err
	}
	var log bytes.Buffer
	for _, c := range chunks {
		if log.Len()+len(c) > discoveryLogBytes {
			return nil, errDiscoveryOutput
		}
		log.Write(c)
	}
	var encoded bytes.Buffer
	for line := range bytes.SplitSeq(log.Bytes(), []byte("\n")) {
		if rest, ok := bytes.CutPrefix(bytes.TrimRight(line, "\r"), []byte(discoveryPrefix)); ok {
			encoded.Write(rest)
		}
	}
	source, err := base64.StdEncoding.DecodeString(encoded.String())
	if err != nil || len(source) == 0 || len(source) > pipelinespec.MaxFileBytes {
		return nil, errDiscoveryOutput
	}
	sum := sha256.Sum256(source)
	if !bytes.Equal(sum[:], want) {
		return nil, errDiscoveryOutput
	}
	return source, nil
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
