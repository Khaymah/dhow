package pipelinespec

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidPipelinesPass(t *testing.T) {
	cases := map[string]string{
		"minimal":            runJob(""),
		"bare if":            job("    if: git.branch == \"main\"\n    steps: [{run: make}]\n"),
		"null trigger":       "version: 1\non:\n  pull_request:\njobs:\n  a:\n    image: " + img + "\n    steps: [{run: make}]\n",
		"plugin only job":    pipeline("  a:\n    steps:\n      - uses: ghcr.io/x/y@sha256:" + digest + "\n"),
		"include only":       job("    strategy:\n      matrix:\n        include: [{os: a}, {os: b}]\n    steps: [{run: make}]\n"),
		"needs outputs":      pipeline("  a:\n    image: " + img + "\n    steps: [{run: x}]\n  b:\n    needs: [a]\n    if: ${{ needs.a.outputs.v == \"1\" }}\n    image: " + img + "\n    steps: [{run: x}]\n"),
		"step env reads env": job("    env: {A: x}\n    steps:\n      - run: make\n        env:\n          B: ${{ env.A + \"-b\" }}\n"),
		"input string":       "version: 1\non:\n  manual:\n    inputs:\n      target: {type: choice, options: [a, b], default: a}\njobs:\n  a:\n    image: " + img + "\n    if: ${{ inputs.target == \"a\" }}\n    steps: [{run: make}]\n",
		"numbers as env":     job("    env: {PORT: 8080, ON: true}\n    steps: [{run: make}]\n"),
		"tag image warns":    pipeline("  a:\n    image: alpine:3\n    steps: [{run: make}]\n"),
		"retention":          job("    steps: [{run: make}]\n    artifacts: [{name: x, paths: [out], retention: 7d}]\n"),
		"resources":          job("    resources: {cpu: 1500m, memory: 512Mi, disk: 10Gi, pids: 64}\n    steps: [{run: make}]\n"),
		"matrix artifact":    job("    strategy:\n      matrix: {os: [a, b]}\n    steps: [{run: make}]\n    artifacts: [{name: '${{ \"out-\" + matrix.os }}', paths: [out]}]\n"),
	}
	for name, src := range cases {
		t.Run(name, func(t *testing.T) {
			spec, issues := Check([]byte(src))
			require.NotNil(t, spec, "%v", issues)
			assert.Empty(t, issueCodes(issues, SeverityError), "%v", issues)
		})
	}
}

func TestWarnings(t *testing.T) {
	_, issues := Check([]byte(pipeline("  a:\n    image: alpine:3\n    environment: prod\n    env: {T: '${{ secrets.T }}'}\n    steps: [{run: make}]\n")))
	assert.Empty(t, issueCodes(issues, SeverityError), "%v", issues)
	assert.ElementsMatch(t, []string{CodeImageNotPinned}, issueCodes(issues, SeverityWarning))
}

func TestIssuesCarryPathAndPosition(t *testing.T) {
	_, issues := Check([]byte(runJob("      - run: echo ${{ git.message }}\n")))
	require.Len(t, issues, 1)
	assert.Equal(t, "jobs.a.steps[1].run", issues[0].Path)
	assert.Equal(t, Position{Line: 9, Column: 9}, issues[0].Position)
	assert.Contains(t, issues[0].String(), "9:9 error RUN_EXPRESSION_NOT_ALLOWED at jobs.a.steps[1].run")
}

func TestMessagesNeverEchoValues(t *testing.T) {
	secret := "ghp_" + strings.Repeat("S3cr3t", 6)
	sources := []string{
		job("    if: ${{ " + secret + " }}\n    steps: [{run: x}]\n"),
		job("    env: {A: 'x${{ " + secret + " }}'}\n    steps: [{run: x}]\n"),
		job("    image: " + secret + "!\n    steps: [{run: x}]\n"),
		job("    steps: [{run: 'echo ${{ " + secret + " }}'}]\n"),
		job("    " + secret + ": 1\n    steps: [{run: x}]\n"),
		"version: 1\non: [" + secret + "\n",
		job("    timeout: " + secret + "\n    steps: [{run: x}]\n"),
	}
	for i, src := range sources {
		_, issues := Check([]byte(src))
		require.NotEmpty(t, issues, "source %d", i)
		for _, is := range issues {
			text := is.Message + " " + is.Code
			assert.NotContains(t, text, secret, "source %d", i)
			e := &Error{Issues: []Issue{{Code: is.Code, Message: is.Message, Severity: is.Severity}}}
			assert.NotContains(t, e.Error(), secret)
		}
	}
}
