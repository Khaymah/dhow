package pipelines

import (
	"context"
	"sort"
	"sync"

	"github.com/google/uuid"

	models "github.com/Khaymah/dhow/apps/server/internal/Modules/Pipelines/Models"
	"github.com/Khaymah/dhow/internal/jobspec"
	"github.com/Khaymah/dhow/internal/pipelinespec"
)

const pinParallel = 4

type pinResult struct {
	pinned string
	err    error
}

type pinScope struct {
	org, project uuid.UUID
	fork         bool
}

func (s *Service) resolveOne(ctx context.Context, ref string, scope pinScope) (string, error) {
	withLogin, ok := s.images.(LoginImageResolver)
	host, known := RegistryHost(ref)
	if !ok || !known || s.logins == nil || scope.fork {
		return s.images.Resolve(ctx, ref)
	}
	login, found, err := s.logins.RegistryLogin(ctx, scope.org, scope.project, host)
	if err != nil {
		return "", err
	}
	if !found {
		return s.images.Resolve(ctx, ref)
	}
	defer login.Password.Wipe()
	return withLogin.ResolveWith(ctx, ref, &login)
}

func (s *Service) resolveAll(ctx context.Context, refs []string, scope pinScope) []pinResult {
	out := make([]pinResult, len(refs))
	if s.images == nil {
		for i := range out {
			out[i].err = ErrRegistry
		}
		return out
	}
	ctx, cancel := context.WithTimeout(ctx, s.pinBudget)
	defer cancel()
	slots := make(chan struct{}, pinParallel)
	var wg sync.WaitGroup
	for i, ref := range refs {
		wg.Go(func() {
			select {
			case slots <- struct{}{}:
			case <-ctx.Done():
				out[i].err = ctx.Err()
				return
			}
			defer func() { <-slots }()
			out[i].pinned, out[i].err = s.resolveOne(ctx, ref, scope)
		})
	}
	wg.Wait()
	return out
}

func (s *Service) pin(ctx context.Context, spec *pipelinespec.Spec, p *prepared, scope pinScope) bool {
	byRef := map[string][]string{}
	for _, image := range taggedImages(spec) {
		if _, ok := p.pins[image]; ok {
			continue
		}
		ref := jobspec.QualifyImage(image)
		byRef[ref] = append(byRef[ref], image)
	}
	refs := make([]string, 0, len(byRef))
	for ref := range byRef {
		refs = append(refs, ref)
	}
	sort.Strings(refs)
	failed := false
	for i, res := range s.resolveAll(ctx, refs, scope) {
		for _, image := range byRef[refs[i]] {
			if res.err != nil {
				failed = true
				p.issues = append(p.issues, pinIssue(spec, image))
				continue
			}
			p.pins[image] = res.pinned
		}
	}
	return failed
}

func pinIssue(spec *pipelinespec.Spec, image string) models.Issue {
	ids := make([]string, 0, len(spec.Jobs))
	for id := range spec.Jobs {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		if spec.Jobs[id].Image == image {
			return models.Issue{
				Code: models.ReasonImagePinFailed, Severity: string(pipelinespec.SeverityError), Path: "jobs." + id + ".image",
				Message: "the registry did not give a digest for this image tag; pin the image by digest or check the registry",
			}
		}
	}
	return models.Issue{Code: models.ReasonImagePinFailed, Severity: string(pipelinespec.SeverityError)}
}
