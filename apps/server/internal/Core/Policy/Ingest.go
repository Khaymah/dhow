package policy

import (
	"net/http"
	"strings"
)

const IngestPrefix = "/ingest/v1/"

func Ingest(method, path string) Rule {
	return Rule{Method: method, Path: path, Scope: ScopeIngest}
}

func (r Rule) validIngest() bool {
	return (r.Method == http.MethodPut || r.Method == http.MethodGet) && strings.HasPrefix(r.Path, IngestPrefix) &&
		len(r.Path) > len(IngestPrefix) && r.Action == "" && r.OrgParam == "" && r.ProjectParam == "" && r.Runner == 0 && !r.Tokens
}
