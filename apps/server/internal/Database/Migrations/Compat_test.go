package migrations

import (
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

var (
	reCaseName      = regexp.MustCompile(`^(accept|reject)_(m[1-9])_[a-z0-9_]+\.sql$`)
	reMigrationName = regexp.MustCompile(`^(\d+)_[a-z0-9_]+\.sql$`)
)

var grandfathered = []string{
	"00001_instance.sql", "00002_identity.sql", "00003_sessions.sql", "00004_setup_tokens.sql",
	"00005_audit_events.sql", "00006_mfa.sql", "00007_organizations.sql", "00030_runners.sql", "00031_jobs.sql",
	"00032_identities_tokens_preferences.sql", "00033_job_logs.sql", "00034_identity_hardening.sql",
	"00035_second_factor_policy_sessions.sql", "00036_jobs_overview_indexes.sql", "00037_instance_settings.sql",
	"00038_oidc_providers.sql", "00039_oidc_flow_issuer.sql", "00040_membership_last_seen.sql", "00041_projects.sql",
	"00042_projects_validate.sql", "00043_sources.sql", "00044_pipelines.sql", "00045_runner_features.sql",
	"00046_artifacts.sql", "00047_blob_owners.sql", "00048_repository_unlink.sql",
}

func tagSet(lines string) map[string]bool {
	out := map[string]bool{}
	for _, l := range strings.Fields(lines) {
		out[l] = true
	}
	return out
}

func TestCompatCases(t *testing.T) {
	dir := filepath.Join("testdata", "compat")
	rawTags, err := os.ReadFile(filepath.Join(dir, "tags.txt"))
	if err != nil {
		t.Fatalf("read tags: %v", err)
	}
	tags := tagSet(string(rawTags))
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read cases: %v", err)
	}
	covered := map[string]bool{}
	for _, e := range entries {
		if e.Name() == "tags.txt" {
			continue
		}
		m := reCaseName.FindStringSubmatch(e.Name())
		if m == nil {
			t.Errorf("%s: case files are named accept_mN_<what>.sql or reject_mN_<what>.sql", e.Name())
			continue
		}
		covered[m[1]+"_"+m[2]] = true
		want := ""
		if m[1] == "reject" {
			want = strings.ToUpper(m[2])
		}
		body, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			t.Fatalf("read %s: %v", e.Name(), err)
		}
		if got := compatVerdict(string(body), tags); got != want {
			t.Errorf("%s: verdict %q, want %q", e.Name(), got, want)
		}
	}
	for n := 1; n <= 9; n++ {
		for _, kind := range []string{"accept", "reject"} {
			if key := kind + "_m" + strconv.Itoa(n); !covered[key] {
				t.Errorf("no %s case for M%d in %s", kind, n, dir)
			}
		}
	}
}

func repoTags(t *testing.T) map[string]bool {
	t.Helper()
	out, err := exec.CommandContext(t.Context(), "git", "tag", "--list", "v*").Output()
	if err != nil {
		t.Logf("git tag: %v (treating the repo as having no release tags)", err)
		return map[string]bool{}
	}
	return tagSet(string(out))
}

func TestMigrationsFollowTheRule(t *testing.T) {
	names, err := fs.Glob(FS, "*.sql")
	if err != nil {
		t.Fatalf("list migrations: %v", err)
	}
	tags := repoTags(t)
	var old []string
	for _, name := range names {
		m := reMigrationName.FindStringSubmatch(name)
		if m == nil {
			t.Errorf("%s: migration files are named NNNNN_snake_case.sql", name)
			continue
		}
		version, err := strconv.Atoi(m[1])
		if err != nil {
			t.Fatalf("%s: version: %v", name, err)
		}
		if version <= grandfatheredThrough {
			old = append(old, name)
			continue
		}
		body, err := fs.ReadFile(FS, name)
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		if rule := compatVerdict(string(body), tags); rule != "" {
			t.Errorf("%s breaks %s of the expand/contract rule (Plan/Upgrades-and-Help.md §5, ADR-0077)", name, rule)
		}
	}
	if strings.Join(old, ",") != strings.Join(grandfathered, ",") {
		t.Errorf("migrations up to %05d are grandfathered and frozen; got %v, want %v. Number a new migration after the last one",
			grandfatheredThrough, old, grandfathered)
	}
}
