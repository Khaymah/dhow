package policy

import (
	"net/http"
	"strings"
)

type Scope uint8

const (
	ScopePublic Scope = iota + 1
	ScopeAuthenticated
	ScopeOrganization
	ScopeInstance
	ScopeRunner
	ScopeProject
	ScopeWebhook
)

const RunnerBlobPrefix = "/blobs/"

type RunnerAccess uint8

const (
	RunnerRegistration RunnerAccess = iota + 1
	RunnerChallenge
	RunnerSigned
	RunnerSession
)

type Rule struct {
	Method       string
	Path         string
	Scope        Scope
	Action       Action
	OrgParam     string
	ProjectParam string
	Runner       RunnerAccess
	Tokens       bool
}

func Public(method, path string) Rule {
	return Rule{Method: method, Path: path, Scope: ScopePublic}
}

func Authenticated(method, path string, action Action) Rule {
	return Rule{Method: method, Path: path, Scope: ScopeAuthenticated, Action: action}
}

func Organization(method, path string, action Action, orgParam string) Rule {
	return Rule{Method: method, Path: path, Scope: ScopeOrganization, Action: action, OrgParam: orgParam}
}

func Instance(method, path string, action Action) Rule {
	return Rule{Method: method, Path: path, Scope: ScopeInstance, Action: action}
}

func Runner(procedure string, access RunnerAccess) Rule {
	return Rule{Method: http.MethodPost, Path: procedure, Scope: ScopeRunner, Runner: access}
}

func RunnerHTTP(method, path string, access RunnerAccess) Rule {
	return Rule{Method: method, Path: path, Scope: ScopeRunner, Runner: access}
}

func (r Rule) AllowTokens() Rule {
	r.Tokens = true
	return r
}

func (r Rule) Key() string { return r.Method + " " + r.Path }

func (r Rule) Valid() bool {
	if r.Tokens && ((r.Scope != ScopeOrganization && r.Scope != ScopeProject) || !IsTokenAction(r.Action)) {
		return false
	}
	if r.Scope != ScopeProject && r.ProjectParam != "" {
		return false
	}
	switch r.Scope {
	case ScopePublic:
		return r.Action == "" && r.Runner == 0
	case ScopeAuthenticated:
		return r.Action == SessionManageSelf && r.Runner == 0
	case ScopeOrganization:
		return r.Action != "" && r.Action != SessionManageSelf && !IsInstanceAction(r.Action) && r.OrgParam != "" && r.Runner == 0
	case ScopeProject:
		return r.validProject()
	case ScopeWebhook:
		return r.validWebhook()
	case ScopeInstance:
		return IsInstanceAction(r.Action) && r.OrgParam == "" && r.Runner == 0
	case ScopeRunner:
		return r.Action == "" && r.OrgParam == "" && r.Runner >= RunnerRegistration && r.Runner <= RunnerSession &&
			strings.HasPrefix(r.Path, "/") && (r.Method == http.MethodPost ||
			((r.Method == http.MethodGet || r.Method == http.MethodPut) && r.Runner == RunnerSession && strings.HasPrefix(r.Path, RunnerBlobPrefix)))
	default:
		return false
	}
}

func (r Rule) Unsafe() bool {
	switch r.Method {
	case http.MethodGet, http.MethodHead, http.MethodOptions:
		return false
	default:
		return true
	}
}

type Table struct {
	rules map[string]Rule
}

func NewTable(rules ...Rule) *Table {
	t := &Table{rules: make(map[string]Rule, len(rules))}
	for _, r := range rules {
		t.rules[r.Key()] = r
	}
	return t
}

func (t *Table) Lookup(method, path string) (Rule, bool) {
	r, ok := t.rules[method+" "+path]
	if !ok || !r.Valid() {
		return Rule{}, false
	}
	return r, true
}
