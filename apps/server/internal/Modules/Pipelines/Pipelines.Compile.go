package pipelines

import (
	"encoding/json"
	"errors"
	"sort"
	"strings"

	models "github.com/Khaymah/dhow/apps/server/internal/Modules/Pipelines/Models"
	"github.com/Khaymah/dhow/internal/jobspec"
	"github.com/Khaymah/dhow/internal/pipelinespec"
)

var errInvalidPipeline = errors.New("the pipeline file has errors")

type compiled struct {
	spec  *pipelinespec.Spec
	plan  *pipelinespec.Plan
	byKey map[string]*pipelinespec.RunJob
}

type matrixEntry struct {
	Key   string
	Value string
}

func issuesOf(in []pipelinespec.Issue) []models.Issue {
	out := make([]models.Issue, 0, len(in))
	for _, i := range in {
		out = append(out, models.Issue{
			Code: i.Code, Severity: string(i.Severity), Path: i.Path, Line: i.Position.Line, Column: i.Position.Column, Message: i.Message,
		})
	}
	return out
}

func encodeIssues(in []models.Issue) []byte {
	if len(in) == 0 {
		return []byte("[]")
	}
	raw, err := json.Marshal(in)
	if err != nil {
		return []byte("[]")
	}
	return raw
}

func decodeIssues(raw []byte) []models.Issue {
	var out []models.Issue
	if len(raw) > 0 {
		_ = json.Unmarshal(raw, &out)
	}
	return out
}

func decodePins(raw []byte) map[string]string {
	out := map[string]string{}
	if len(raw) > 0 {
		_ = json.Unmarshal(raw, &out)
	}
	return out
}

func encodePins(pins map[string]string) []byte {
	raw, err := json.Marshal(pins)
	if err != nil || len(pins) == 0 {
		return []byte("{}")
	}
	return raw
}

func pinList(pins map[string]string) []models.ImagePin {
	out := make([]models.ImagePin, 0, len(pins))
	for image, pinned := range pins {
		out = append(out, models.ImagePin{Image: image, Pinned: pinned})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Image < out[j].Image })
	return out
}

func taggedImages(spec *pipelinespec.Spec) []string {
	seen := map[string]bool{}
	var out []string
	for _, job := range spec.Jobs {
		img := job.Image
		if img == "" || seen[img] || strings.Contains(img, "${{") {
			continue
		}
		if jobspec.ImageReason(jobspec.QualifyImage(img)) == jobspec.ReasonNotPinned {
			seen[img] = true
			out = append(out, img)
		}
	}
	sort.Strings(out)
	return out
}

func applyPins(spec *pipelinespec.Spec, pins map[string]string) {
	for _, job := range spec.Jobs {
		if pinned, ok := pins[job.Image]; ok {
			job.Image = pinned
		}
	}
}

func checkSource(src []byte) (*pipelinespec.Spec, []models.Issue, error) {
	spec, issues := pipelinespec.Check(src)
	if spec == nil || pipelinespec.HasErrors(issues) {
		return nil, issuesOf(issues), errInvalidPipeline
	}
	return spec, issuesOf(issues), nil
}

func compileSpec(spec *pipelinespec.Spec, pins map[string]string) (compiled, []models.Issue, error) {
	applyPins(spec, pins)
	plan, err := pipelinespec.Compile(spec)
	if err != nil {
		var perr *pipelinespec.Error
		if errors.As(err, &perr) {
			return compiled{}, issuesOf(perr.Issues), errInvalidPipeline
		}
		return compiled{}, nil, err
	}
	out := compiled{spec: spec, plan: plan, byKey: make(map[string]*pipelinespec.RunJob, len(plan.Jobs))}
	for _, rj := range plan.Jobs {
		out.byKey[rj.Key] = rj
	}
	return out, issuesOf(plan.Warnings), nil
}

func compileSource(src []byte, pins map[string]string) (compiled, error) {
	spec, _, err := checkSource(src)
	if err != nil {
		return compiled{}, err
	}
	c, _, err := compileSpec(spec, pins)
	return c, err
}

func matrixOf(rj *pipelinespec.RunJob) []byte {
	out := make([]matrixEntry, 0, len(rj.Matrix))
	for _, m := range rj.Matrix {
		out = append(out, matrixEntry{Key: m.Key, Value: m.Value.Text})
	}
	raw, err := json.Marshal(out)
	if err != nil {
		return []byte("[]")
	}
	return raw
}

func matrixMap(raw []byte) map[string]string {
	var entries []matrixEntry
	_ = json.Unmarshal(raw, &entries)
	out := make(map[string]string, len(entries))
	for _, e := range entries {
		out[e.Key] = e.Value
	}
	return out
}
