package jobspec

import (
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/Khaymah/dhow/internal/cachekey"
	runnerv1 "github.com/Khaymah/dhow/packages/proto/gen/go/dhow/runner/v1"
)

const (
	FeatureArtifacts      = "artifacts"
	FeatureCache          = "cache"
	FeatureRegistryAuth   = "registry_auth"
	MaxArtifacts          = 16
	MaxInputs             = 256
	MaxTransferPaths      = 32
	MaxTransferPathLength = 512
	MaxDownloadToken      = 256
)

var (
	artifactName  = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,63}$`)
	digestPattern = regexp.MustCompile(`^[0-9a-f]{64}$`)
	tokenPattern  = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)
)

func ValidArtifactName(name string) bool { return artifactName.MatchString(name) }

func ValidDigest(d string) bool { return digestPattern.MatchString(d) }

func ValidWorkspacePath(p string) bool {
	if p == "" || len(p) > MaxTransferPathLength || !utf8.ValidString(p) || strings.IndexFunc(p, unicode.IsControl) >= 0 {
		return false
	}
	if strings.HasPrefix(p, "/") || strings.HasPrefix(p, "~") || strings.Contains(p, "\\") {
		return false
	}
	for seg := range strings.SplitSeq(p, "/") {
		if seg == ".." {
			return false
		}
	}
	return true
}

func (c *checker) paths(field string, paths []string) {
	if len(paths) == 0 {
		c.add(field, ReasonRequired)
		return
	}
	if len(paths) > MaxTransferPaths {
		c.add(field, ReasonTooMany)
		return
	}
	for i, p := range paths {
		if !ValidWorkspacePath(p) {
			c.add(at(field, i), ReasonInvalid)
		}
	}
}

func (c *checker) transfers(spec *runnerv1.JobSpec) {
	if len(spec.GetArtifacts()) > MaxArtifacts {
		c.add("Artifacts", ReasonTooMany)
	}
	names := map[string]bool{}
	for i, a := range spec.GetArtifacts() {
		field := at("Artifacts", i)
		switch {
		case !ValidArtifactName(a.GetName()):
			c.add(field+".Name", ReasonInvalid)
		case names[a.GetName()]:
			c.add(field+".Name", ReasonDuplicate)
		}
		names[a.GetName()] = true
		c.paths(field+".Paths", a.GetPaths())
		if a.GetMaxBytes() <= 0 {
			c.add(field+".MaxBytes", ReasonOutOfRange)
		}
		if a.GetRetentionSeconds() < 0 {
			c.add(field+".RetentionSeconds", ReasonOutOfRange)
		}
	}
	c.inputs(spec.GetInputs())
	c.cache(spec.GetCache())
}

func (c *checker) inputs(inputs []*runnerv1.ArtifactInput) {
	if len(inputs) > MaxInputs {
		c.add("Inputs", ReasonTooMany)
	}
	names := map[string]bool{}
	for i, in := range inputs {
		field := at("Inputs", i)
		switch {
		case !ValidArtifactName(in.GetName()):
			c.add(field+".Name", ReasonInvalid)
		case names[in.GetName()]:
			c.add(field+".Name", ReasonDuplicate)
		}
		names[in.GetName()] = true
		if !ValidDigest(in.GetDigest()) {
			c.add(field+".Digest", ReasonInvalid)
		}
		if in.GetSizeBytes() < 0 {
			c.add(field+".SizeBytes", ReasonOutOfRange)
		}
		if t := in.GetDownloadToken(); t == "" || len(t) > MaxDownloadToken || !tokenPattern.MatchString(t) {
			c.add(field+".DownloadToken", ReasonInvalid)
		}
	}
}

func (c *checker) cache(cache *runnerv1.Cache) {
	if cache == nil {
		return
	}
	hashes := cache.GetHashes()
	if len(hashes) > cachekey.MaxHashes {
		c.add("Cache.Hashes", ReasonTooMany)
		return
	}
	for i, h := range hashes {
		if !cachekey.ValidPatterns(h.GetPatterns()) {
			c.add(at("Cache.Hashes", i), ReasonInvalid)
		}
	}
	placeholder := make([]string, len(hashes))
	for i := range placeholder {
		placeholder[i] = strings.Repeat("0", 64)
	}
	if _, err := cachekey.Render(cache.GetKeyExpression(), placeholder); err != nil {
		c.add("Cache.KeyExpression", ReasonInvalid)
	}
	c.paths("Cache.Paths", cache.GetPaths())
	if cache.GetMaxBytes() <= 0 {
		c.add("Cache.MaxBytes", ReasonOutOfRange)
	}
}
