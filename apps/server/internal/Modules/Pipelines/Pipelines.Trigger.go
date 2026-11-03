package pipelines

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	config "github.com/Khaymah/dhow/apps/server/internal/Config"
	commitstatus "github.com/Khaymah/dhow/apps/server/internal/Core/CommitStatus"
	apperrors "github.com/Khaymah/dhow/apps/server/internal/Core/Errors"
	eventbus "github.com/Khaymah/dhow/apps/server/internal/Core/EventBus"
	ratelimit "github.com/Khaymah/dhow/apps/server/internal/Core/RateLimit"
	triggers "github.com/Khaymah/dhow/apps/server/internal/Core/Triggers"
	sqlc "github.com/Khaymah/dhow/apps/server/internal/Database/Sqlc"
	models "github.com/Khaymah/dhow/apps/server/internal/Modules/Pipelines/Models"
	"github.com/Khaymah/dhow/internal/gitref"
	"github.com/Khaymah/dhow/internal/pipelinespec"
)

const (
	messageInvalidValue = "errors.validationFailed"
	maxActorBytes       = 255
	limitTriggerUser    = "run-trigger-user"
	limitTriggerProject = "run-trigger-project"
)

var (
	ErrInvalidTrigger = errors.New("invalid trigger")

	inputName = regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_]{0,63}$`)
)

func invalid(field string) apperrors.IErrorDetail {
	return apperrors.IErrorDetail{Field: field, Message: messageInvalidValue}
}

type newRun struct {
	project   sqlc.GetRunProjectRow
	trigger   string
	ref       string
	baseRef   string
	sha       string
	baseSha   string
	isFork    bool
	prNumber  int
	actor     string
	inputs    []byte
	delivery  *uuid.UUID
	userID    *uuid.UUID
	repoID    *uuid.UUID
	provider  string
	awaitsYou bool
}

func (s *Service) insertRun(ctx context.Context, q *sqlc.Queries, n newRun) (sqlc.PipelineRun, error) {
	number, err := q.NextRunNumber(ctx, n.project.Id)
	if err != nil {
		return sqlc.PipelineRun{}, err
	}
	state := models.RunPending
	if n.awaitsYou {
		state = models.RunAwaitingApproval
	}
	definitionSha := n.sha
	if n.isFork {
		definitionSha = n.baseSha
	}
	if len(n.actor) > maxActorBytes {
		n.actor = n.actor[:maxActorBytes]
	}
	inputs := n.inputs
	if len(inputs) == 0 {
		inputs = []byte("{}")
	}
	run, err := q.InsertRun(ctx, sqlc.InsertRunParams{
		OrganizationID: n.project.OrganizationId, ProjectID: n.project.Id, RepositoryID: n.repoID, Provider: text(n.provider),
		Number: number, Trigger: n.trigger, Ref: n.ref, BaseRef: text(n.baseRef), Sha: text(n.sha), BaseSha: text(n.baseSha),
		IsFork: n.isFork, PrNumber: int4(n.prNumber), Actor: text(strings.ToValidUTF8(n.actor, "")), Inputs: inputs,
		DefinitionPath: n.project.PipelinePath, DefinitionSha: text(definitionSha), State: state, DeliveryID: n.delivery,
		TriggeredByUserID: n.userID,
	})
	if err != nil {
		return run, err
	}
	return run, s.recordStatus(ctx, q, run, commitstatus.StatePending)
}

func checkTrigger(t triggers.Trigger) error {
	switch t.Event {
	case triggers.EventPush, triggers.EventTag, triggers.EventPullRequest:
	default:
		return fmt.Errorf("%w: event %q does not come from a webhook", ErrInvalidTrigger, t.Event)
	}
	if t.RepositoryID == uuid.Nil || t.ProjectID == uuid.Nil || t.OrganizationID == uuid.Nil {
		return fmt.Errorf("%w: no repository", ErrInvalidTrigger)
	}
	ref, err := gitref.Parse(t.Ref)
	if err != nil || !gitref.ValidSha(t.Sha) || gitref.IsZeroSha(t.Sha) || !refFits(t.Event, ref.Kind) {
		return fmt.Errorf("%w: ref or sha", ErrInvalidTrigger)
	}
	if t.Event == triggers.EventPullRequest && (t.PrNumber <= 0 || !gitref.ValidSha(t.BaseSha)) {
		return fmt.Errorf("%w: pull request without a number or a base", ErrInvalidTrigger)
	}
	if t.Event != triggers.EventPullRequest && t.Origin != triggers.PullOriginUnknown {
		return fmt.Errorf("%w: only a pull request has an origin", ErrInvalidTrigger)
	}
	return nil
}

func refFits(event string, kind gitref.Kind) bool {
	switch event {
	case triggers.EventPush:
		return kind == gitref.KindBranch
	case triggers.EventTag:
		return kind == gitref.KindTag
	default:
		return kind == gitref.KindPullRequest || kind == gitref.KindMergeRequest
	}
}

func fromFork(t triggers.Trigger) bool {
	return t.Event == triggers.EventPullRequest && t.Origin != triggers.PullOriginSameRepository
}

func (s *Service) Deliver(ctx context.Context, q *sqlc.Queries, t triggers.Trigger) (triggers.Accepted, error) {
	if err := checkTrigger(t); err != nil {
		return triggers.Accepted{}, err
	}
	project, err := q.GetRunProject(ctx, sqlc.GetRunProjectParams{ProjectID: t.ProjectID, OrganizationID: t.OrganizationID})
	if errors.Is(err, pgx.ErrNoRows) {
		return triggers.Accepted{}, nil
	}
	if err != nil {
		return triggers.Accepted{}, err
	}
	if project.State != projectActive || project.RepositoryId == nil || *project.RepositoryId != t.RepositoryID {
		return triggers.Accepted{}, nil
	}
	if err := q.LockRunTrigger(ctx, sqlc.LockRunTriggerParams{RepositoryID: t.RepositoryID, Ref: t.Ref}); err != nil {
		return triggers.Accepted{}, err
	}
	fork := fromFork(t)
	existing, err := q.FindCommitRun(ctx, sqlc.FindCommitRunParams{
		RepositoryID: &t.RepositoryID, Ref: t.Ref, Sha: text(t.Sha), Trigger: t.Event, IsFork: fork,
	})
	switch {
	case err == nil:
		return triggers.Accepted{RunID: existing}, nil
	case !errors.Is(err, pgx.ErrNoRows):
		return triggers.Accepted{}, err
	}
	if err := chargeWebhook(ctx, q, project.Id); err != nil {
		return triggers.Accepted{}, err
	}
	delivery := userRef(t.DeliveryID)
	repo := t.RepositoryID
	run, err := s.insertRun(ctx, q, newRun{
		project: project, trigger: t.Event, ref: t.Ref, baseRef: t.BaseRef, sha: t.Sha, baseSha: t.BaseSha, isFork: fork,
		prNumber: t.PrNumber, actor: t.Actor, delivery: delivery, repoID: &repo, provider: t.Provider,
		awaitsYou: fork && project.ForkPolicy != models.ForkPolicyAutomatic,
	})
	if err != nil {
		return triggers.Accepted{}, err
	}
	return triggers.Accepted{RunID: run.Id, Created: true}, nil
}

func chargeWebhook(ctx context.Context, q *sqlc.Queries, project uuid.UUID) error {
	window := config.PipelineWebhookRunsWindow
	recent, err := q.CountRecentWebhookRuns(ctx, sqlc.CountRecentWebhookRunsParams{ProjectID: project, WindowSeconds: window.Seconds()})
	if err != nil {
		return err
	}
	if recent.Runs < config.PipelineWebhookRunsPerProject {
		return nil
	}
	return apperrors.RateLimited(max(time.Duration(recent.RetryAfterSeconds)*time.Second, time.Second))
}

func (s *Service) chargeTrigger(ctx context.Context, project, user uuid.UUID) error {
	if s.limiter == nil {
		return nil
	}
	for _, c := range []struct {
		limit   ratelimit.Limit
		subject string
	}{
		{ratelimit.Limit{Name: limitTriggerUser, Max: config.PipelineTriggerPerUser, Window: config.PipelineTriggerWindow}, user.String()},
		{ratelimit.Limit{Name: limitTriggerProject, Max: config.PipelineTriggerPerProject, Window: config.PipelineTriggerWindow}, project.String()},
	} {
		d, err := s.limiter.Take(ctx, c.limit, c.subject)
		if err != nil {
			return err
		}
		if err := d.Err(); err != nil {
			return err
		}
	}
	return nil
}

func trustedSha(ctx context.Context, q *sqlc.Queries, repo uuid.UUID, ref, sha string) (string, error) {
	if sha != "" {
		known, err := q.TrustedShaOnRef(ctx, sqlc.TrustedShaOnRefParams{RepositoryID: &repo, Ref: ref, Sha: text(sha)})
		if err != nil {
			return "", err
		}
		if !known {
			return "", apperrors.ShaNotOnRef()
		}
		return sha, nil
	}
	head, err := q.LatestTrustedSha(ctx, sqlc.LatestTrustedShaParams{RepositoryID: &repo, Ref: ref})
	if errors.Is(err, pgx.ErrNoRows) {
		return "", apperrors.RefHeadUnknown()
	}
	return head, err
}

func normalizeRef(ref string) (string, bool) {
	if !strings.HasPrefix(ref, "refs/") {
		ref = gitref.PrefixBranch + ref
	}
	parsed, err := gitref.Parse(ref)
	if err != nil || (parsed.Kind != gitref.KindBranch && parsed.Kind != gitref.KindTag) {
		return "", false
	}
	return parsed.Full, true
}

func checkManualInputs(in map[string]any) ([]byte, bool) {
	if len(in) > pipelinespec.MaxManualInputs {
		return nil, false
	}
	for name, v := range in {
		if !inputName.MatchString(name) {
			return nil, false
		}
		switch val := v.(type) {
		case bool:
		case string:
			if len(val) > pipelinespec.MaxContextVarBytes {
				return nil, false
			}
		default:
			return nil, false
		}
	}
	raw, err := json.Marshal(in)
	if err != nil {
		return nil, false
	}
	return raw, true
}

func (s *Service) Trigger(ctx context.Context, org, projectID uuid.UUID, in models.TriggerInput) (models.Run, error) {
	p, err := principalOf(ctx)
	if err != nil {
		return models.Run{}, err
	}
	ref, ok := normalizeRef(in.Ref)
	var details []apperrors.IErrorDetail
	if !ok {
		details = append(details, invalid("Ref"))
	}
	if in.Sha != "" && !gitref.ValidSha(in.Sha) {
		details = append(details, invalid("Sha"))
	}
	inputs, ok := checkManualInputs(in.Inputs)
	if !ok {
		details = append(details, invalid("Inputs"))
	}
	if len(details) > 0 {
		return models.Run{}, apperrors.ValidationFailed(details...)
	}
	if err := s.chargeTrigger(ctx, projectID, p.UserID); err != nil {
		return models.Run{}, err
	}
	event := triggers.EventManual
	if p.ViaToken() {
		event = triggers.EventAPI
	}
	var run sqlc.PipelineRun
	err = s.inTx(ctx, func(q *sqlc.Queries) error {
		project, err := q.GetRunProject(ctx, sqlc.GetRunProjectParams{ProjectID: projectID, OrganizationID: org})
		switch {
		case errors.Is(err, pgx.ErrNoRows):
			return apperrors.NotFound()
		case err != nil:
			return err
		case project.State != projectActive:
			return apperrors.ProjectArchived()
		case project.RepositoryId == nil:
			return apperrors.RepositoryNotLinked()
		}
		sha, err := trustedSha(ctx, q, *project.RepositoryId, ref, in.Sha)
		if err != nil {
			return err
		}
		run, err = s.insertRun(ctx, q, newRun{
			project: project, trigger: event, ref: ref, sha: sha, actor: p.Name, inputs: inputs, userID: &p.UserID,
			repoID: project.RepositoryId, provider: project.Provider.String,
		})
		return err
	})
	if err != nil {
		return models.Run{}, err
	}
	s.auditByUser(ctx, eventbus.RunCreated, run, p)
	return runOf(run), nil
}
