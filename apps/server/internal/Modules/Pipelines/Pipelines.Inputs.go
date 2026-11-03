package pipelines

import (
	"context"
	"errors"
	"slices"
	"strconv"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	apperrors "github.com/Khaymah/dhow/apps/server/internal/Core/Errors"
	sqlc "github.com/Khaymah/dhow/apps/server/internal/Database/Sqlc"
	gen "github.com/Khaymah/dhow/apps/server/internal/Modules/Pipelines/Gen"
	models "github.com/Khaymah/dhow/apps/server/internal/Modules/Pipelines/Models"
	"github.com/Khaymah/dhow/internal/pipelinespec"
)

const (
	messageManualInputs = "pipelines.manualInputs.loaded"
	inputString         = "string"
	branchPrefix        = "refs/heads/"
)

func (s *Service) ManualInputs(ctx context.Context, org, project uuid.UUID) (models.ManualInputs, error) {
	row, err := s.queries().GetManualDefinition(ctx, sqlc.GetManualDefinitionParams{ProjectID: project, OrganizationID: org})
	if errors.Is(err, pgx.ErrNoRows) {
		return models.ManualInputs{}, apperrors.RepositoryNotLinked()
	}
	if err != nil {
		return models.ManualInputs{}, err
	}
	out := models.ManualInputs{Ref: branchPrefix + row.DefaultBranch, Inputs: []models.ManualInput{}}
	if !row.DefinitionSource.Valid {
		return out, nil
	}
	out.Known, out.Sha = true, row.DefinitionSha.String
	spec, _ := pipelinespec.Parse([]byte(row.DefinitionSource.String))
	if spec == nil || spec.On == nil || spec.On.Manual == nil {
		return out, nil
	}
	out.Enabled = true
	for name, in := range spec.On.Manual.Inputs {
		if in == nil {
			continue
		}
		out.Inputs = append(out.Inputs, manualInputOf(name, in))
	}
	slices.SortFunc(out.Inputs, func(a, b models.ManualInput) int { return strings.Compare(a.Name, b.Name) })
	return out, nil
}

func manualInputOf(name string, in *pipelinespec.Input) models.ManualInput {
	kind := in.Type
	if kind == "" {
		kind = inputString
	}
	m := models.ManualInput{Name: name, Type: kind, Description: in.Description, Required: in.Required, Options: []string{}}
	if kind == inputChoice {
		m.Options = append(m.Options, in.Options...)
	}
	if in.Default != nil {
		v := in.Default.Text
		if in.Default.Kind == pipelinespec.KindBool {
			v = strconv.FormatBool(in.Default.Bool)
		}
		m.Default = &v
	}
	return m
}

func (c *Controller) GetManualInputs(ctx context.Context, req gen.GetManualInputsRequestObject) (gen.GetManualInputsResponseObject, error) {
	m, err := c.service.ManualInputs(ctx, req.Id, req.ProjectId)
	if err != nil {
		return nil, err
	}
	data := gen.ManualInputs{Known: m.Known, Enabled: m.Enabled, Ref: m.Ref, Sha: optional(m.Sha), Inputs: make([]gen.ManualInput, 0, len(m.Inputs))}
	for _, in := range m.Inputs {
		data.Inputs = append(data.Inputs, gen.ManualInput{
			Name: in.Name, Type: gen.ManualInputType(in.Type), Description: optional(in.Description), Required: in.Required,
			Default: in.Default, Options: in.Options,
		})
	}
	return gen.GetManualInputs200JSONResponse{Success: true, Message: ptrOf(messageManualInputs), Data: &data}, nil
}
