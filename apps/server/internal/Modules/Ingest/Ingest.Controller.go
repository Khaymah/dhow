package ingest

import (
	"context"

	auth "github.com/Khaymah/dhow/apps/server/internal/Core/Auth"
	apperrors "github.com/Khaymah/dhow/apps/server/internal/Core/Errors"
	response "github.com/Khaymah/dhow/apps/server/internal/Core/Response"
	gen "github.com/Khaymah/dhow/apps/server/internal/Modules/Ingest/Gen"
	models "github.com/Khaymah/dhow/apps/server/internal/Modules/Ingest/Models"
)

const (
	messageTokensListed    = "ingest.tokens.listed"
	messageTokenCreated    = "ingest.token.created" //nolint:gosec // An i18n key, not a credential.
	messageTokenRevoked    = "ingest.token.revoked" //nolint:gosec // An i18n key, not a credential.
	messageArtifactsListed = "ingest.artifacts.listed"
)

type Controller struct {
	service *Service
}

var _ gen.StrictServerInterface = (*Controller)(nil)

func NewController(service *Service) *Controller { return &Controller{service: service} }

func ptrOf[T any](v T) *T { return &v }

func nonNil(in []string) []string {
	if in == nil {
		return []string{}
	}
	return in
}

func factorOf(f gen.CurrentFactor) auth.Factor {
	out := auth.Factor{Method: string(f.Method)}
	if f.Password != nil {
		out.Password = *f.Password
	}
	if f.Code != nil {
		out.Code = *f.Code
	}
	if f.ChallengeId != nil {
		out.ChallengeID = *f.ChallengeId
	}
	if f.Credential != nil {
		out.Credential = *f.Credential
	}
	return out
}

func tokenView(t models.Token) gen.IngestToken {
	patterns := make([]gen.IngestRefPattern, 0, len(t.RefPatterns))
	for _, p := range t.RefPatterns {
		patterns = append(patterns, gen.IngestRefPattern{Kind: gen.IngestRefKind(p.Kind), Pattern: p.Pattern})
	}
	return gen.IngestToken{
		Id: t.ID, ProjectId: t.ProjectID, Name: t.Name, Prefix: t.Prefix, RefPatterns: patterns, Status: gen.IngestTokenStatus(t.Status),
		CreatedAt: t.CreatedAt, ExpiresAt: t.ExpiresAt, LastUsedAt: t.LastUsedAt, RevokedAt: t.RevokedAt, CreatedByUserId: ptrOf(t.CreatedByUserID),
		Reaches: ptrOf(nonNil(t.Reaches)),
	}
}

func artifactView(a models.Artifact) gen.IngestedArtifact {
	out := gen.IngestedArtifact{
		Id: a.ID, Name: a.Name, Digest: a.Digest, SizeBytes: a.SizeBytes, Ref: a.Ref, RefKind: gen.IngestRefKind(a.RefKind), Sha: a.Sha,
		Provenance: a.Provenance, ProvenanceVerified: a.ProvenanceVerified, TokenName: a.TokenName, CreatedAt: a.CreatedAt,
		ExpiresAt: a.ExpiresAt, PinnedUntil: a.PinnedUntil,
	}
	if a.Run != nil {
		out.RunId, out.RunNumber, out.RunState = ptrOf(a.Run.ID), ptrOf(a.Run.Number), ptrOf(a.Run.State)
	}
	return out
}

func (c *Controller) ListIngestTokens(ctx context.Context, req gen.ListIngestTokensRequestObject) (gen.ListIngestTokensResponseObject, error) {
	page, limit := response.PageParams(req.Params.Page, req.Params.Limit)
	list, err := c.service.ListTokens(ctx, req.Id, req.ProjectId, page, limit)
	if err != nil {
		return nil, err
	}
	items := make([]gen.IngestToken, 0, len(list.Items))
	for _, t := range list.Items {
		items = append(items, tokenView(t))
	}
	return gen.ListIngestTokens200JSONResponse{
		Success: true, Message: ptrOf(messageTokensListed), Data: &items, Meta: &gen.PageMeta{Page: page, Limit: limit, Total: int(list.Total)},
	}, nil
}

func (c *Controller) CreateIngestToken(ctx context.Context, req gen.CreateIngestTokenRequestObject) (gen.CreateIngestTokenResponseObject, error) {
	if req.Body == nil {
		return nil, apperrors.BadRequest()
	}
	b := req.Body
	patterns := make([]models.RefPattern, 0, len(b.RefPatterns))
	for _, p := range b.RefPatterns {
		patterns = append(patterns, models.RefPattern{Kind: string(p.Kind), Pattern: p.Pattern})
	}
	in := models.CreateTokenInput{Name: b.Name, RefPatterns: patterns}
	if b.ExpiresInDays != nil {
		in.ExpiresInDays = *b.ExpiresInDays
	}
	if b.AcknowledgeProtectedReach != nil {
		in.AcknowledgeProtectedReach = *b.AcknowledgeProtectedReach
	}
	created, err := c.service.CreateToken(ctx, req.Id, req.ProjectId, in, factorOf(b.CurrentFactor))
	if err != nil {
		return nil, err
	}
	data := gen.CreatedIngestToken{Token: tokenView(created.Token), Value: created.Value}
	return gen.CreateIngestToken201JSONResponse{Success: true, Message: ptrOf(messageTokenCreated), Data: &data}, nil
}

func (c *Controller) RevokeIngestToken(ctx context.Context, req gen.RevokeIngestTokenRequestObject) (gen.RevokeIngestTokenResponseObject, error) {
	if req.Body == nil {
		return nil, apperrors.BadRequest()
	}
	t, err := c.service.RevokeToken(ctx, req.Id, req.ProjectId, req.IngestTokenId, factorOf(req.Body.CurrentFactor))
	if err != nil {
		return nil, err
	}
	data := tokenView(t)
	return gen.RevokeIngestToken200JSONResponse{Success: true, Message: ptrOf(messageTokenRevoked), Data: &data}, nil
}

func (c *Controller) ListIngestedArtifacts(ctx context.Context, req gen.ListIngestedArtifactsRequestObject) (gen.ListIngestedArtifactsResponseObject, error) {
	page, limit := response.PageParams(req.Params.Page, req.Params.Limit)
	list, err := c.service.ListArtifacts(ctx, req.Id, req.ProjectId, page, limit)
	if err != nil {
		return nil, err
	}
	items := make([]gen.IngestedArtifact, 0, len(list.Items))
	for _, a := range list.Items {
		items = append(items, artifactView(a))
	}
	return gen.ListIngestedArtifacts200JSONResponse{
		Success: true, Message: ptrOf(messageArtifactsListed), Data: &items,
		Meta: &gen.PageMeta{Page: page, Limit: limit, Total: int(list.Total)},
	}, nil
}
