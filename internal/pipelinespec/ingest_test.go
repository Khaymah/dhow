package pipelinespec

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const ingestSource = "version: 1\n" +
	"on:\n  artifact:\n    names: [web, api]\n    refs: [main, release/**]\n" +
	"jobs:\n  ship:\n    image: " + img + "\n    ingested: [web]\n    steps:\n      - run: ls\n"

func TestIngestedWithOnArtifactCompilesAndShowsInThePlan(t *testing.T) {
	spec, issues := Check([]byte(ingestSource))
	require.NotNil(t, spec)
	assert.False(t, HasErrors(issues), "%v", issues)
	require.NotNil(t, spec.On.Artifact)
	assert.Equal(t, []string{"web", "api"}, spec.On.Artifact.Names)
	assert.Equal(t, []string{"main", "release/**"}, spec.On.Artifact.Refs)
	plan, err := Compile(spec)
	require.NoError(t, err)
	require.Len(t, plan.Jobs, 1)
	assert.Equal(t, []string{"web"}, plan.Jobs[0].Ingested)
}

func TestIngestedChangesTheJobDigestOnlyWhenPresent(t *testing.T) {
	with := compileSource(t, []byte(ingestSource))
	without := compileSource(t, []byte(
		"version: 1\non:\n  artifact:\n    names: [web, api]\njobs:\n  ship:\n    image: "+img+"\n    steps:\n      - run: ls\n"))
	assert.NotEqual(t, with.Jobs[0].Digest, without.Jobs[0].Digest)
	plain := compileSource(t, []byte(job("    steps:\n      - run: make\n")))
	again := compileSource(t, []byte(job("    steps:\n      - run: make\n")))
	assert.Equal(t, plain.Jobs[0].Digest, again.Jobs[0].Digest)
	assert.Empty(t, plain.Jobs[0].Ingested)
}

func TestIngestedWithoutOnArtifactIsAnError(t *testing.T) {
	src := job("    ingested: [web]\n    steps:\n      - run: make\n")
	_, issues := Check([]byte(src))
	assert.Contains(t, issueCodes(issues, SeverityError), CodeIngestedNotDeclared)
}

func TestOnArtifactAloneIsAValidTrigger(t *testing.T) {
	src := "version: 1\non:\n  artifact:\n    names: [web]\njobs:\n  a:\n    image: " + img + "\n    steps:\n      - run: make\n"
	spec, issues := Check([]byte(src))
	require.NotNil(t, spec)
	assert.False(t, HasErrors(issues), "%v", issues)
}

func TestOnArtifactRefsUseTheSamePatternRules(t *testing.T) {
	src := "version: 1\non:\n  artifact:\n    names: [web]\n    refs: ['[x]']\njobs:\n  a:\n    image: " + img + "\n    steps:\n      - run: make\n"
	_, issues := Check([]byte(src))
	assert.Contains(t, issueCodes(issues, SeverityError), CodePatternInvalid)
}
