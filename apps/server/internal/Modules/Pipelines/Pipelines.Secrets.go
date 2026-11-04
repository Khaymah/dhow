package pipelines

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"strconv"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	secretstore "github.com/Khaymah/dhow/apps/server/internal/Core/SecretStore"
	sqlc "github.com/Khaymah/dhow/apps/server/internal/Database/Sqlc"
	models "github.com/Khaymah/dhow/apps/server/internal/Modules/Pipelines/Models"
	"github.com/Khaymah/dhow/internal/jobspec"
	runnerv1 "github.com/Khaymah/dhow/packages/proto/gen/go/dhow/runner/v1"
)

const (
	triggerPullRequest = "pull_request"
	failureDetailMax   = 1024
)

var ErrNoSecretScope = errors.New("the job is not the current job of an active pipeline run")

type SecretScope struct {
	ProjectID   uuid.UUID
	RunID       uuid.UUID
	RunJobID    uuid.UUID
	IsFork      bool
	PullRequest bool
}

func (s *Service) UseSecrets(store secretstore.Store) { s.secrets = store }

func (s *Service) UseVariables(store secretstore.VariableStore) { s.vars = store }

func (s *Service) variablesOf(ctx context.Context, q *sqlc.Queries, run sqlc.PipelineRun) (map[string]string, error) {
	if s.vars == nil || run.IsFork {
		return map[string]string{}, nil
	}
	if q == nil {
		q = s.queries()
	}
	return s.vars.Variables(ctx, q, run.OrganizationId, run.ProjectId, nil)
}

func (s *Service) SecretScopeOfJob(ctx context.Context, org, job uuid.UUID) (SecretScope, error) {
	row, err := s.queries().SecretScopeOfJob(ctx, sqlc.SecretScopeOfJobParams{JobID: job, OrganizationID: org})
	if errors.Is(err, pgx.ErrNoRows) {
		return SecretScope{}, ErrNoSecretScope
	}
	if err != nil {
		return SecretScope{}, err
	}
	return SecretScope{ProjectID: row.ProjectId, RunID: row.RunId, RunJobID: row.RunJobId, IsFork: row.IsFork, PullRequest: row.PullRequest}, nil
}

func userSecretNames(spec *runnerv1.JobSpec) []string {
	var out []string
	for _, ref := range spec.GetSecrets() {
		if n := ref.GetName(); n != jobspec.CheckoutSecret && !slices.Contains(out, n) {
			out = append(out, n)
		}
	}
	return out
}

func failureDetail(names []string) []byte {
	for len(names) > 0 {
		raw, err := json.Marshal(map[string][]string{"Names": names})
		if err == nil && len(raw) <= failureDetailMax {
			return raw
		}
		names = names[:len(names)-1]
	}
	return []byte("{}")
}

func (s *Service) checkSecrets(ctx context.Context, q *sqlc.Queries, run sqlc.PipelineRun, spec *runnerv1.JobSpec) (string, []byte, error) {
	names := userSecretNames(spec)
	if len(names) == 0 || s.secrets == nil {
		return "", nil, nil
	}
	res, err := s.secrets.Check(ctx, q, secretstore.Request{
		OrganizationID: run.OrganizationId, ProjectID: run.ProjectId, Names: names,
		PullRequest: run.Trigger == triggerPullRequest, Fork: run.IsFork,
	})
	switch {
	case errors.Is(err, secretstore.ErrFork):
		return models.ReasonForkSecrets, nil, nil
	case errors.Is(err, secretstore.ErrTooMany), errors.Is(err, secretstore.ErrTooLarge):
		return models.ReasonSecretsTooLarge, nil, nil
	case err != nil:
		return "", nil, err
	case len(res.Missing) > 0:
		return models.ReasonSecretNotFound, failureDetail(res.Missing), nil
	case len(res.NotForPullRequests) > 0:
		return models.ReasonSecretNotForPRs, failureDetail(res.NotForPullRequests), nil
	}
	return "", nil, nil
}

func failureNamesOf(detail []byte) []string {
	var d struct{ Names []string }
	if len(detail) == 0 || json.Unmarshal(detail, &d) != nil {
		return nil
	}
	return d.Names
}

func namesOrNil(names []string) *[]string {
	if len(names) == 0 {
		return nil
	}
	return &names
}

func imageHost(image string) string {
	ref := jobspec.QualifyImage(image)
	host, _, found := strings.Cut(ref, "/")
	if !found {
		return ""
	}
	if host == dockerHubIndex {
		return dockerHubRegistry
	}
	return host
}

func jobImages(spec *runnerv1.JobSpec) []string {
	var out []string
	if spec.GetImage() != "" {
		out = append(out, spec.GetImage())
	}
	for _, st := range spec.GetSteps() {
		if img := st.GetPlugin().GetImage(); img != "" {
			out = append(out, img)
		}
	}
	return out
}

func (s *Service) attachRegistryAuth(ctx context.Context, q *sqlc.Queries, run sqlc.PipelineRun, spec *runnerv1.JobSpec) error {
	if s.logins == nil || run.IsFork {
		return nil
	}
	var hosts []string
	for _, img := range jobImages(spec) {
		if h := imageHost(img); h != "" && !slices.Contains(hosts, h) {
			hosts = append(hosts, h)
		}
	}
	for _, h := range hosts {
		if len(spec.GetRegistryAuth()) >= jobspec.MaxRegistryAuth {
			return nil
		}
		ok, err := s.logins.HasRegistryLogin(ctx, q, run.OrganizationId, run.ProjectId, h)
		if err != nil {
			return err
		}
		if ok {
			spec.RegistryAuth = append(spec.RegistryAuth, &runnerv1.RegistryAuth{
				Host: h, SecretName: jobspec.RegistrySecretPrefix + strconv.Itoa(len(spec.GetRegistryAuth())),
			})
		}
	}
	return nil
}
