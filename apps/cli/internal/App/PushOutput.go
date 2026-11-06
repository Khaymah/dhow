package app

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"slices"
	"strconv"

	output "github.com/Khaymah/dhow/apps/cli/internal/Output"
	"github.com/Khaymah/dhow/internal/archive"
	"github.com/Khaymah/dhow/internal/ingestpush"
)

const hiddenAuthorization = "Bearer (hidden)"

type dryRunReport struct {
	DryRun    bool              `json:"DryRun"`
	Digest    string            `json:"Digest"`
	SizeBytes int64             `json:"SizeBytes"`
	Packed    bool              `json:"Packed"`
	Entries   int               `json:"Entries"`
	Unpacked  int64             `json:"UnpackedBytes"`
	URL       string            `json:"Url,omitempty"`
	Headers   map[string]string `json:"Headers"`
}

type resultReport struct {
	Name      string `json:"Name"`
	Digest    string `json:"Digest"`
	SizeBytes int64  `json:"SizeBytes"`
	Created   bool   `json:"Created"`
	Triggered bool   `json:"Triggered"`
	Reason    string `json:"Reason,omitempty"`
	RunID     string `json:"RunId,omitempty"`
	RunNumber int64  `json:"RunNumber,omitempty"`
	RunURL    string `json:"RunUrl,omitempty"`
	Attempts  int    `json:"Attempts"`
}

func failureLines(c output.Catalog, e *ingestpush.Error) []string {
	if e.Code == ingestpush.CodeInput {
		return []string{c.T(output.KeyPushPrefix) + " " + inputMessage(c, e.Err)}
	}
	msg := c.T(output.CodeKey(e.Code))
	if msg == string(output.CodeKey(e.Code)) {
		msg = c.T(classKey(e.Class))
	}
	lines := []string{fmt.Sprintf("%s %s (%s)", c.T(output.KeyPushPrefix), msg, output.Printable(e.Code))}
	if e.RetryAfter > 0 {
		lines = append(lines, c.T(output.KeyPushRetryAfter, strconv.Itoa(int(e.RetryAfter.Seconds()))))
	}
	if e.Attempts > 1 {
		lines = append(lines, c.T(output.KeyPushAttempts, e.Attempts))
	}
	return lines
}

func classKey(class ingestpush.Class) output.Key {
	switch class {
	case ingestpush.ClassToken:
		return output.KeyPushClassToken
	case ingestpush.ClassRefused:
		return output.KeyPushClassRefused
	case ingestpush.ClassTemporary:
		return output.KeyPushClassTemporary
	default:
		return output.KeyPushClassUnexpected
	}
}

var inputKeys = []struct {
	err error
	key output.Key
}{
	{ingestpush.ErrServer, output.KeyPushErrServer},
	{ingestpush.ErrToken, output.KeyPushTokenShape},
	{ingestpush.ErrName, output.KeyPushErrName},
	{ingestpush.ErrRef, output.KeyPushErrRef},
	{ingestpush.ErrRefKind, output.KeyPushErrRefKind},
	{ingestpush.ErrSha, output.KeyPushErrSha},
	{ingestpush.ErrProvenance, output.KeyPushErrProvenance},
	{ingestpush.ErrCAFile, output.KeyPushErrCAFile},
	{ingestpush.ErrNotGzip, output.KeyPushErrNotGzip},
	{ingestpush.ErrEmpty, output.KeyPushErrEmpty},
	{ingestpush.ErrNotLoopback, output.KeyPushErrNotLoopback},
	{ingestpush.ErrRetries, output.KeyPushRetries},
	{archive.ErrUnsafe, output.KeyPushErrUnsafe},
	{archive.ErrUnsupported, output.KeyPushErrUnsupported},
	{archive.ErrMissing, output.KeyPushErrMissing},
	{archive.ErrTooLarge, output.KeyPushErrTooLarge},
	{archive.ErrPath, output.KeyPushErrPath},
}

func inputMessage(c output.Catalog, err error) string {
	for _, k := range inputKeys {
		if errors.Is(err, k.err) {
			return c.T(k.key)
		}
	}
	if errors.Is(err, ingestpush.ErrSource) {
		return c.T(output.KeyPushErrSource, output.Printable(err.Error()))
	}
	return c.T(output.KeyPushErrSource, output.Printable(fmt.Sprint(err)))
}

func renderDryRun(c output.Catalog, env Env, opts pushOptions, meta ingestpush.Meta, server string, pkg *ingestpush.Package) int {
	headers, err := meta.Headers(pkg.Size)
	if err != nil {
		return fail(c, env, err)
	}
	report := dryRunReport{DryRun: true, Digest: pkg.Digest, SizeBytes: pkg.Size, Packed: pkg.Packed, Entries: pkg.Entries, Unpacked: pkg.Unpacked, Headers: map[string]string{}}
	for k := range headers {
		report.Headers[k] = headers.Get(k)
	}
	report.Headers["Authorization"] = hiddenAuthorization
	if server != "" {
		target, err := ingestpush.Endpoint(server, pkg.Digest)
		if err != nil {
			return fail(c, env, err)
		}
		report.URL = target.String()
	}
	if opts.format == FormatJSON {
		return writeJSON(env.Out, report)
	}
	lines := []string{c.T(output.KeyPushDryRun), c.T(output.KeyPushDigest, pkg.Digest), c.T(output.KeyPushSize, pkg.Size), c.T(output.KeyPushPacked, pkg.Entries, pkg.Unpacked, pkg.Size)}
	if report.URL != "" {
		lines = append(lines, "PUT "+output.Printable(report.URL))
	}
	names := make([]string, 0, len(report.Headers))
	for k := range report.Headers {
		names = append(names, k)
	}
	slices.Sort(names)
	for _, k := range names {
		lines = append(lines, "  "+k+": "+output.Printable(report.Headers[k]))
	}
	say(env.Out, lines...)
	return ExitValid
}

func renderResult(c output.Catalog, env Env, opts pushOptions, res ingestpush.Result, pkg *ingestpush.Package) int {
	if opts.format == FormatJSON {
		return writeJSON(env.Out, resultReport{
			Name: res.Name, Digest: res.Digest, SizeBytes: res.SizeBytes, Created: res.Created, Triggered: res.Triggered, Reason: res.Reason,
			RunID: res.RunID, RunNumber: res.RunNumber, RunURL: res.RunURL, Attempts: res.Attempts,
		})
	}
	head := output.KeyPushStored
	if !res.Created {
		head = output.KeyPushDuplicate
	}
	lines := []string{c.T(head, output.Printable(res.Name), res.Digest, res.SizeBytes), c.T(output.KeyPushPacked, pkg.Entries, pkg.Unpacked, pkg.Size)}
	switch {
	case res.Triggered && res.RunID != "":
		lines = append(lines, c.T(output.KeyPushRun, res.RunNumber, output.Printable(res.RunURL)))
	case res.Reason != "":
		lines = append(lines, c.T(output.KeyPushNoRun, output.Printable(res.Reason)))
	}
	say(env.Out, lines...)
	return ExitValid
}

func writeJSON(w io.Writer, v any) int {
	raw, err := json.Marshal(v)
	if err != nil {
		return ExitUsage
	}
	if _, err := w.Write(append(raw, '\n')); err != nil {
		return ExitUsage
	}
	return ExitValid
}
