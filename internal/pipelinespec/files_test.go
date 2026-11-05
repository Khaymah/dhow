package pipelinespec

import (
	"context"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	runnerv1 "github.com/Khaymah/dhow/packages/proto/gen/go/dhow/runner/v1"
)

func fileRefs(spec *runnerv1.JobSpec) map[string]*runnerv1.SecretRef {
	out := map[string]*runnerv1.SecretRef{}
	for _, ref := range spec.GetSecrets() {
		if f := ref.GetAsFile(); f != "" {
			out[f] = ref
		}
	}
	return out
}

func TestSecretFilesCompileToFileReferences(t *testing.T) {
	src := job(`    files:
      kubeconfig: ${{ secrets.KUBECONFIG }}
    steps:
      - run: make
      - uses: ` + img + `
        with:
          token: ${{ secrets.PLUGIN_TOKEN }}
      - run: ./deploy.sh
        files:
          deploy_key: ${{ secrets.DEPLOY_KEY }}
`)
	plan := compileSource(t, []byte(src))
	a := jobByKey(t, plan, "a")
	refs := fileRefs(a.Spec)
	require.Len(t, refs, 2)
	assert.Equal(t, "KUBECONFIG", refs["kubeconfig"].GetName())
	assert.Equal(t, []int32{0, 2}, refs["kubeconfig"].GetStepIndexes(), "a job file reaches every run step but never the plugin")
	assert.Equal(t, "DEPLOY_KEY", refs["deploy_key"].GetName())
	assert.Equal(t, []int32{2}, refs["deploy_key"].GetStepIndexes())

	built, err := a.Build(context.Background(), Context{})
	require.NoError(t, err, "every file reference is declared on its steps")
	assert.Len(t, fileRefs(built), 2)
}

func TestSecretFileRules(t *testing.T) {
	cases := map[string]struct {
		src  string
		code string
	}{
		"not a secret":     {job("    files: {kubeconfig: plain text}\n    steps: [{run: make}]\n"), CodeSecretFileNotSecret},
		"part of a string": {job("    files: {kubeconfig: 'x ${{ secrets.K }}'}\n    steps: [{run: make}]\n"), CodeExpressionSyntax},
		"a hidden name":    {job("    files: {.ssh: '${{ secrets.K }}'}\n    steps: [{run: make}]\n"), CodeSecretFileInvalid},
		"a path":           {job("    files: {a/b: '${{ secrets.K }}'}\n    steps: [{run: make}]\n"), CodeSecretFileInvalid},
		"on a plugin":      {job("    steps:\n      - uses: " + img + "\n        files: {k: '${{ secrets.K }}'}\n"), CodeSecretFilesOnPlugin},
		"id_token":         {job("    id_token: {audiences: [sts.example.com]}\n    steps: [{run: make}]\n"), CodeIDTokenUnsupported},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			_, issues := Check([]byte(tc.src))
			assert.Contains(t, issueCodes(issues, SeverityError), tc.code, "%v", issues)
		})
	}

	var many strings.Builder
	many.WriteString("    files:\n")
	for i := range 21 {
		many.WriteString("      f" + strconv.Itoa(i) + ": ${{ secrets.S" + strconv.Itoa(i) + " }}\n")
	}
	many.WriteString("    steps: [{run: make}]\n")
	_, issues := Check([]byte(job(many.String())))
	assert.Contains(t, issueCodes(issues, SeverityError), CodeTooManySecretFiles)
}
