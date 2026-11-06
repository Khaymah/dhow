package app

import (
	"slices"
	"strconv"
	"strings"

	output "github.com/Khaymah/dhow/apps/cli/internal/Output"
	"github.com/Khaymah/dhow/internal/ingestpush"
)

const (
	CommandArtifact = "artifact"
	SubcommandPush  = "push"

	EnvIngestToken = "DHOW_INGEST_TOKEN" //nolint:gosec // The name of the variable, not a credential. allow-secret
	EnvServer      = "DHOW_SERVER"
	EnvCAFile      = "DHOW_CA_FILE"

	optionServer     = "server"
	optionName       = "name"
	optionRef        = "ref"
	optionRefKind    = "ref-kind"
	optionSha        = "sha"
	optionProvenance = "provenance"
	optionNoTrigger  = "no-trigger"
	optionTokenStdin = "token-stdin"
	optionDryRun     = "dry-run"
	optionCAFile     = "ca-file"
	optionRetries    = "retries"
	optionMaxBytes   = "max-bytes"
	optionIncludeGit = "include-git"
	optionToken      = "token"
	optionHelp       = "help"
	optionShortHelp  = "h"
)

type pushOptions struct {
	server     string
	name       string
	ref        string
	refKind    string
	sha        string
	caFile     string
	format     string
	lang       string
	retries    int
	maxBytes   int64
	includeGit bool
	noTrigger  bool
	tokenStdin bool
	dryRun     bool
	help       bool
	provenance map[string]string
	paths      []string
}

var pushValueOptions = map[string]func(*pushOptions, string) *usageError{
	optionServer:   func(o *pushOptions, v string) *usageError { o.server = v; return nil },
	optionName:     func(o *pushOptions, v string) *usageError { o.name = v; return nil },
	optionRef:      func(o *pushOptions, v string) *usageError { o.ref = v; return nil },
	optionRefKind:  func(o *pushOptions, v string) *usageError { o.refKind = v; return nil },
	optionSha:      func(o *pushOptions, v string) *usageError { o.sha = v; return nil },
	optionCAFile:   func(o *pushOptions, v string) *usageError { o.caFile = v; return nil },
	optionFormat:   func(o *pushOptions, v string) *usageError { o.format = v; return nil },
	optionLang:     func(o *pushOptions, v string) *usageError { o.lang = v; return nil },
	optionRetries:  setRetries,
	optionMaxBytes: setMaxBytes,
	optionProvenance: func(o *pushOptions, v string) *usageError {
		k, val, ok := strings.Cut(v, "=")
		if !ok || k == "" {
			return &usageError{key: output.KeyPushProvenanceArg}
		}
		o.provenance[k] = val
		return nil
	},
}

var pushFlagOptions = map[string]func(*pushOptions){
	optionNoTrigger:  func(o *pushOptions) { o.noTrigger = true },
	optionTokenStdin: func(o *pushOptions) { o.tokenStdin = true },
	optionDryRun:     func(o *pushOptions) { o.dryRun = true },
	optionIncludeGit: func(o *pushOptions) { o.includeGit = true },
	optionHelp:       func(o *pushOptions) { o.help = true },
	optionShortHelp:  func(o *pushOptions) { o.help = true },
}

func setRetries(o *pushOptions, v string) *usageError {
	n, err := strconv.Atoi(v)
	if err != nil || n < 0 || n > ingestpush.MaxRetries {
		return &usageError{key: output.KeyPushRetries}
	}
	o.retries = n
	return nil
}

func setMaxBytes(o *pushOptions, v string) *usageError {
	n, err := strconv.ParseInt(v, 10, 64)
	if err != nil || n <= 0 {
		return &usageError{key: output.KeyPushMaxBytes}
	}
	o.maxBytes = n
	return nil
}

func optionLabel(arg string) string {
	name, _, _ := strings.Cut(arg, "=")
	return name
}

func parsePush(args []string) (pushOptions, *usageError) {
	opts := pushOptions{format: FormatText, retries: ingestpush.DefaultRetries, provenance: map[string]string{}}
	var bad *usageError
	note := func(e *usageError) {
		if bad == nil && e != nil {
			bad = e
		}
	}
	onlyPaths := false
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if onlyPaths || arg == "-" || !strings.HasPrefix(arg, "-") {
			opts.paths = append(opts.paths, arg)
			continue
		}
		if arg == "--" {
			onlyPaths = true
			continue
		}
		name, value, hasValue := strings.Cut(strings.TrimPrefix(strings.TrimPrefix(arg, "-"), "-"), "=")
		if flag, ok := pushFlagOptions[name]; ok && !hasValue {
			flag(&opts)
			continue
		}
		set, takesValue := pushValueOptions[name]
		switch {
		case name == optionToken:
			note(&usageError{key: output.KeyPushTokenFlag})
		case !takesValue:
			note(&usageError{key: output.KeyPushUnknownOption, arg: optionLabel(arg)})
		case !hasValue && i+1 >= len(args):
			note(&usageError{key: output.KeyPushOptionValue, arg: "--" + name})
		default:
			if !hasValue {
				i++
				value = args[i]
			}
			note(set(&opts, value))
		}
	}
	note(pushShape(opts))
	return opts, bad
}

func pushShape(opts pushOptions) *usageError {
	switch {
	case opts.lang != "" && !output.ValidLang(opts.lang):
		return &usageError{key: output.KeyPushLangInvalid}
	case opts.format != FormatText && opts.format != FormatJSON:
		return &usageError{key: output.KeyPushFormatInvalid}
	case opts.help:
		return nil
	case slices.ContainsFunc(opts.paths, func(p string) bool { return strings.HasPrefix(p, ingestpush.TokenPrefix) }):
		return &usageError{key: output.KeyPushPathIsToken}
	case len(opts.paths) != 1:
		return &usageError{key: output.KeyPushNeedPath}
	}
	return nil
}
