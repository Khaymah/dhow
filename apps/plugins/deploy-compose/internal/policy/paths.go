package policy

import (
	"path"
	"strings"
)

const (
	sharedDir   = "shared"
	releasesDir = "releases"
	stateDir    = ".dhow"
	defaultFile = "compose.yaml"
)

var hardCodes = map[string]bool{
	"INTERPOLATION": true, "BIND_RELEASE": true, "BIND_COMPOSE": true, "BIND_SHARED": true, "BIND_STATE": true, "BIND_SYMLINK": true,
}

func (c *checker) structCode(p string, strict bool) string {
	switch {
	case p == "":
		return "BIND_OUTSIDE"
	case strings.Contains(p, "$"):
		return "INTERPOLATION"
	case strings.Contains(strings.ToLower(p), "docker.sock"):
		return "DOCKER_SOCKET"
	case strings.HasPrefix(p, "~"):
		return "BIND_OUTSIDE"
	case strings.HasPrefix(p, "/"):
		return c.absoluteCode(path.Clean(p), strict)
	}
	return c.relativeCode(path.Clean(p), strict)
}

func (c *checker) absoluteCode(p string, strict bool) string {
	deploy := path.Clean(c.opt.DeployPath)
	if p == "/" {
		return "BIND_ROOT"
	}
	if deploy == "/" || deploy == "." || !strings.HasPrefix(p, deploy+"/") {
		return "BIND_OUTSIDE"
	}
	first, tail, _ := strings.Cut(strings.TrimPrefix(p, deploy+"/"), "/")
	switch {
	case first == sharedDir:
		if strict && (tail == "" || tail == ".env" || strings.HasPrefix(tail, ".env/")) {
			return "BIND_SHARED"
		}
		return ""
	case first == releasesDir:
		id, sub, _ := strings.Cut(tail, "/")
		if id == "" || sub == "" {
			return "BIND_RELEASE"
		}
		return c.relativeCode(sub, strict)
	case first == stateDir:
		return "BIND_STATE"
	case first == "current" || strings.HasPrefix(first, "current."):
		return "BIND_RELEASE"
	}
	return "BIND_OUTSIDE"
}

func (c *checker) relativeCode(p string, strict bool) string {
	if p == ".." || strings.HasPrefix(p, "../") {
		return "BIND_OUTSIDE"
	}
	if strict {
		if p == "." {
			return "BIND_RELEASE"
		}
		if c.reachesComposeFile(p) {
			return "BIND_COMPOSE"
		}
	}
	for _, link := range c.opt.Links {
		if p == link || strings.HasPrefix(p, link+"/") {
			return "BIND_SYMLINK"
		}
	}
	return ""
}

func (c *checker) reachesComposeFile(p string) bool {
	file := c.opt.File
	if file == "" {
		file = defaultFile
	}
	file = path.Clean(file)
	for _, f := range []string{file, OverrideFile} {
		if p == f || strings.HasPrefix(f, p+"/") {
			return true
		}
	}
	return false
}
