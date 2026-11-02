package executor

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"hash"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"

	"connectrpc.com/connect"

	client "github.com/Khaymah/dhow/apps/runner/internal/Client"
	config "github.com/Khaymah/dhow/apps/runner/internal/Config"
	sandbox "github.com/Khaymah/dhow/apps/runner/internal/Sandbox"
	"github.com/Khaymah/dhow/internal/cachekey"
	runnerv1 "github.com/Khaymah/dhow/packages/proto/gen/go/dhow/runner/v1"
)

const hashOutputMax = 256

var errDigest = errors.New("the downloaded bytes do not match the digest")

type transferAPI interface {
	PutBlob(ctx context.Context, ref *runnerv1.LeaseRef, kind, digest string, size int64, open func() (io.ReadCloser, error)) error
	GetBlob(ctx context.Context, ref *runnerv1.LeaseRef, digest, token string) (client.Blob, error)
	CommitArtifact(ctx context.Context, ref *runnerv1.LeaseRef, name, digest string, size int64) error
	LookupCache(ctx context.Context, ref *runnerv1.LeaseRef, key string) (*runnerv1.CacheServiceLookupResponse, error)
	CommitCache(ctx context.Context, ref *runnerv1.LeaseRef, key, digest string, size int64) (bool, error)
}

func permanent(err error) error {
	if errors.Is(err, errDigest) {
		return connect.NewError(connect.CodeDataLoss, err)
	}
	return connect.NewError(connect.CodeFailedPrecondition, err)
}

func restores(spec *runnerv1.JobSpec) bool {
	return len(spec.GetInputs()) > 0 || spec.GetCache() != nil
}

func saves(spec *runnerv1.JobSpec) bool {
	return len(spec.GetArtifacts()) > 0 || (spec.GetCache() != nil && !spec.GetCache().GetReadOnly())
}

type packed struct {
	path   string
	digest string
	size   int64
}

type limitedFile struct {
	f        *os.File
	h        hash.Hash
	n        int64
	max      int64
	over     bool
	onExceed func()
}

func (w *limitedFile) Write(p []byte) (int, error) {
	if w.over {
		return len(p), nil
	}
	if w.n+int64(len(p)) > w.max {
		w.over = true
		w.onExceed()
		return len(p), nil
	}
	n, err := w.f.Write(p)
	w.n += int64(n)
	_, _ = w.h.Write(p[:n])
	return n, err
}

func (a *attempt) transferAPI() (transferAPI, bool) {
	t, ok := a.e.o.API.(transferAPI)
	return t, ok
}

func (a *attempt) runTransfer(ctx, jobCtx context.Context, argv []string, env map[string]string, stdin io.Reader, stdout io.Writer) (int32, result, bool) {
	stderr := a.pipe.Writer(runnerv1.LogStream_LOG_STREAM_STDERR, nil)
	defer func() { _ = stderr.Close() }()
	execCtx, stopExec := context.WithCancel(ctx)
	defer stopExec()
	box := a.currentBox()
	done := make(chan stepEnd, 1)
	go func() {
		res, err := box.Exec(execCtx, sandbox.StepSpec{
			Index: sandbox.TransferStepIndex, Name: "transfer", Argv: argv, Env: env, Stdin: stdin, Stdout: stdout, Stderr: stderr,
		})
		done <- stepEnd{res: res, err: err, ok: true}
	}()
	end, why, stopped := a.waitStep(ctx, jobCtx, jobCtx, done, stopExec)
	if stopped {
		a.pipe.Unblock()
		return 0, why, false
	}
	if end.err != nil {
		a.log.Error("a transfer could not run", "error", a.masked(end.err))
		return 0, failed(runnerv1.FailureReason_FAILURE_REASON_SANDBOX_FAILED, 0), false
	}
	return end.res.ExitCode, result{}, true
}

func (a *attempt) leaseError(err error, reason runnerv1.FailureReason, what string) result {
	switch {
	case client.IsLeaseLost(err), errors.Is(err, ErrLeaseLost):
		a.fence.markLost(ErrLeaseLost)
		return gone()
	case errors.Is(err, ErrSelfFenced):
		return lost(runnerv1.FailureReason_FAILURE_REASON_LEASE_LOST)
	case a.stopping():
		return lost(runnerv1.FailureReason_FAILURE_REASON_RUNNER_RESTARTED)
	}
	a.pipe.Systemf(nil, "dhow: %s: %s (%s)", reasonName(reason), what, client.Reason(err))
	return failed(reason, 0)
}

func (a *attempt) download(ctx context.Context, api transferAPI, digest, token string, size int64) (string, error) {
	path := filepath.Join(a.dir, "download-"+digest+".tgz")
	err := a.retry(ctx, func(rctx context.Context) error {
		blob, err := api.GetBlob(rctx, a.ref, digest, token)
		if err != nil {
			return err
		}
		defer func() { _ = blob.Body.Close() }()
		if size >= 0 && blob.Size != size {
			return permanent(errDigest)
		}
		f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, config.PrivateFileMode) //nolint:gosec // The attempt directory of this runner.
		if err != nil {
			return permanent(err)
		}
		h := sha256.New()
		n, err := io.Copy(io.MultiWriter(f, h), io.LimitReader(blob.Body, blob.Size+1))
		if cerr := f.Close(); err == nil {
			err = cerr
		}
		if err != nil {
			return err
		}
		if n != blob.Size || hex.EncodeToString(h.Sum(nil)) != digest {
			return permanent(errDigest)
		}
		return nil
	})
	if err != nil {
		_ = os.Remove(path)
		return "", err
	}
	return path, nil
}

func (a *attempt) unpack(ctx, jobCtx context.Context, path string) (result, bool) {
	f, err := os.Open(path) //nolint:gosec // A file this attempt downloaded into its own directory.
	if err != nil {
		return failed(runnerv1.FailureReason_FAILURE_REASON_ARTIFACT_DOWNLOAD_FAILED, 0), false
	}
	defer func() { _ = f.Close() }()
	env := map[string]string{sandbox.UnpackMaxEnv: strconv.FormatInt(a.grant.GetSpec().GetResources().GetDiskBytes(), 10)}
	code, why, ok := a.runTransfer(ctx, jobCtx, []string{sandbox.AgentPath, sandbox.UnpackCommand}, env, f, io.Discard)
	switch {
	case !ok:
		return why, false
	case code == sandbox.TransferExitOK:
		return result{}, true
	case code == sandbox.TransferExitUnsafe:
		return failed(runnerv1.FailureReason_FAILURE_REASON_ARTIFACT_UNSAFE, code), false
	default:
		return failed(runnerv1.FailureReason_FAILURE_REASON_ARTIFACT_DOWNLOAD_FAILED, code), false
	}
}

func (a *attempt) restoreTransfers(ctx, jobCtx context.Context) (result, bool) {
	api, ok := a.transferAPI()
	if !ok {
		return failed(runnerv1.FailureReason_FAILURE_REASON_SANDBOX_FAILED, 0), false
	}
	for _, in := range a.grant.GetSpec().GetInputs() {
		a.pipe.Systemf(nil, "dhow: artifact %s: download and unpack (%s)", in.GetName(), in.GetDigest()[:12])
		path, err := a.download(ctx, api, in.GetDigest(), in.GetDownloadToken(), in.GetSizeBytes())
		if err != nil {
			return a.leaseError(err, runnerv1.FailureReason_FAILURE_REASON_ARTIFACT_DOWNLOAD_FAILED, "the artifact "+in.GetName()+" could not be downloaded"), false
		}
		r, ok := a.unpack(ctx, jobCtx, path)
		_ = os.Remove(path)
		if !ok {
			if r.reason == runnerv1.FailureReason_FAILURE_REASON_ARTIFACT_UNSAFE {
				a.pipe.Systemf(nil, "dhow: ARTIFACT_UNSAFE: the artifact %s has an entry that would leave the workspace", in.GetName())
			}
			return r, false
		}
	}
	if a.grant.GetSpec().GetCache() == nil {
		return result{}, true
	}
	return a.restoreCache(ctx, jobCtx, api)
}

func (a *attempt) cacheKey(ctx, jobCtx context.Context) (string, result, bool) {
	c := a.grant.GetSpec().GetCache()
	sums := make([]string, 0, len(c.GetHashes()))
	for _, h := range c.GetHashes() {
		var out bytes.Buffer
		w := &capped{w: &out, max: hashOutputMax}
		argv := append([]string{sandbox.AgentPath, sandbox.HashFilesCommand, "--"}, h.GetPatterns()...)
		code, why, ok := a.runTransfer(ctx, jobCtx, argv, nil, nil, w)
		if !ok {
			return "", why, false
		}
		if code != sandbox.TransferExitOK {
			return "", result{}, true
		}
		sums = append(sums, strings.TrimSpace(out.String()))
	}
	key, err := cachekey.Render(c.GetKeyExpression(), sums)
	if err != nil {
		return "", result{}, true
	}
	return key, result{}, true
}

type capped struct {
	w   io.Writer
	max int
	n   int
}

func (c *capped) Write(p []byte) (int, error) {
	room := c.max - c.n
	if room <= 0 {
		return len(p), nil
	}
	if len(p) > room {
		_, _ = c.w.Write(p[:room])
		c.n = c.max
		return len(p), nil
	}
	c.n += len(p)
	return c.w.Write(p)
}

func (a *attempt) restoreCache(ctx, jobCtx context.Context, api transferAPI) (result, bool) {
	key, why, ok := a.cacheKey(ctx, jobCtx)
	if !ok {
		return why, false
	}
	if key == "" {
		a.pipe.Systemf(nil, "dhow: warning: the cache key could not be computed, so the job runs without the cache")
		return result{}, true
	}
	a.cacheKeyValue = key
	var hit *runnerv1.CacheServiceLookupResponse
	err := a.retry(ctx, func(rctx context.Context) error {
		var err error
		hit, err = api.LookupCache(rctx, a.ref, key)
		return err
	})
	if err != nil {
		if client.IsLeaseLost(err) || errors.Is(err, ErrSelfFenced) {
			return a.leaseError(err, runnerv1.FailureReason_FAILURE_REASON_LEASE_LOST, "the cache lookup failed"), false
		}
		a.pipe.Systemf(nil, "dhow: warning: the cache could not be read (%s)", client.Reason(err))
		return result{}, true
	}
	if !hit.GetHit() {
		a.pipe.Systemf(nil, "dhow: cache: no entry for this key yet")
		return result{}, true
	}
	a.cacheHit = true
	a.pipe.Systemf(nil, "dhow: cache: restoring %s", hit.GetDigest()[:12])
	path, err := a.download(ctx, api, hit.GetDigest(), hit.GetDownloadToken(), hit.GetSizeBytes())
	if err != nil {
		if client.IsLeaseLost(err) || errors.Is(err, ErrSelfFenced) {
			return a.leaseError(err, runnerv1.FailureReason_FAILURE_REASON_LEASE_LOST, "the cache download failed"), false
		}
		a.pipe.Systemf(nil, "dhow: warning: the cache could not be downloaded, so the job runs without it")
		return result{}, true
	}
	defer func() { _ = os.Remove(path) }()
	r, ok := a.unpack(ctx, jobCtx, path)
	if !ok && r.reason == runnerv1.FailureReason_FAILURE_REASON_ARTIFACT_UNSAFE {
		a.pipe.Systemf(nil, "dhow: ARTIFACT_UNSAFE: the cache entry has an entry that would leave the workspace")
		return r, false
	}
	if !ok && r.state != runnerv1.AttemptResult_ATTEMPT_RESULT_FAILED {
		return r, false
	}
	if !ok {
		a.pipe.Systemf(nil, "dhow: warning: the cache could not be unpacked")
	}
	return result{}, true
}

func (a *attempt) pack(ctx, jobCtx context.Context, name string, paths []string, maxBytes int64) (packed, int32, result, bool) {
	path := filepath.Join(a.dir, "upload-"+name+".tgz")
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, config.PrivateFileMode) //nolint:gosec // The attempt directory of this runner.
	if err != nil {
		return packed{}, 0, failed(runnerv1.FailureReason_FAILURE_REASON_ARTIFACT_UPLOAD_FAILED, 0), false
	}
	box := a.currentBox()
	w := &limitedFile{f: f, h: sha256.New(), max: maxBytes, onExceed: func() { _ = box.Signal(ctx, syscall.SIGKILL) }}
	argv := append([]string{sandbox.AgentPath, sandbox.PackCommand, "--"}, paths...)
	code, why, ok := a.runTransfer(ctx, jobCtx, argv, nil, nil, w)
	cerr := f.Close()
	if !ok {
		_ = os.Remove(path)
		return packed{}, 0, why, false
	}
	if w.over {
		_ = os.Remove(path)
		return packed{}, code, failed(runnerv1.FailureReason_FAILURE_REASON_ARTIFACT_TOO_LARGE, 0), false
	}
	if code != sandbox.TransferExitOK || cerr != nil {
		_ = os.Remove(path)
		return packed{}, code, result{}, true
	}
	return packed{path: path, digest: hex.EncodeToString(w.h.Sum(nil)), size: w.n}, code, result{}, true
}

func (a *attempt) upload(ctx context.Context, api transferAPI, kind string, p packed) error {
	return a.retry(ctx, func(rctx context.Context) error {
		return api.PutBlob(rctx, a.ref, kind, p.digest, p.size, func() (io.ReadCloser, error) {
			f, err := os.Open(p.path)
			if err != nil {
				return nil, permanent(err)
			}
			return f, nil
		})
	})
}

func packFailure(code int32) runnerv1.FailureReason {
	switch code {
	case sandbox.TransferExitUnsupported, sandbox.TransferExitUnsafe:
		return runnerv1.FailureReason_FAILURE_REASON_ARTIFACT_UNSAFE
	case sandbox.TransferExitTooLarge:
		return runnerv1.FailureReason_FAILURE_REASON_ARTIFACT_TOO_LARGE
	default:
		return runnerv1.FailureReason_FAILURE_REASON_ARTIFACT_UPLOAD_FAILED
	}
}

func (a *attempt) saveTransfers(ctx, jobCtx context.Context) (result, bool) {
	api, ok := a.transferAPI()
	if !ok {
		return failed(runnerv1.FailureReason_FAILURE_REASON_SANDBOX_FAILED, 0), false
	}
	for _, art := range a.grant.GetSpec().GetArtifacts() {
		if r, ok := a.saveArtifact(ctx, jobCtx, api, art); !ok {
			return r, false
		}
	}
	return a.saveCache(ctx, jobCtx, api)
}

func (a *attempt) saveArtifact(ctx, jobCtx context.Context, api transferAPI, art *runnerv1.ArtifactUpload) (result, bool) {
	a.pipe.Systemf(nil, "dhow: artifact %s: pack and upload", art.GetName())
	p, code, why, ok := a.pack(ctx, jobCtx, art.GetName(), art.GetPaths(), art.GetMaxBytes())
	if !ok {
		if why.reason == runnerv1.FailureReason_FAILURE_REASON_ARTIFACT_TOO_LARGE {
			a.pipe.Systemf(nil, "dhow: ARTIFACT_TOO_LARGE: the artifact %s is larger than %d bytes", art.GetName(), art.GetMaxBytes())
		}
		return why, false
	}
	if p.path == "" {
		reason := packFailure(code)
		a.pipe.Systemf(nil, "dhow: %s: the artifact %s could not be packed (exit code %d)", reasonName(reason), art.GetName(), code)
		return failed(reason, code), false
	}
	defer func() { _ = os.Remove(p.path) }()
	if err := a.upload(ctx, api, client.BlobKindArtifact, p); err != nil {
		reason := runnerv1.FailureReason_FAILURE_REASON_ARTIFACT_UPLOAD_FAILED
		if client.Code(err) == connect.CodeOutOfRange {
			reason = runnerv1.FailureReason_FAILURE_REASON_ARTIFACT_TOO_LARGE
		}
		return a.leaseError(err, reason, "the artifact "+art.GetName()+" could not be uploaded"), false
	}
	err := a.retry(ctx, func(rctx context.Context) error {
		return api.CommitArtifact(rctx, a.ref, art.GetName(), p.digest, p.size)
	})
	if err != nil {
		return a.leaseError(err, runnerv1.FailureReason_FAILURE_REASON_ARTIFACT_UPLOAD_FAILED, "the artifact "+art.GetName()+" could not be recorded"), false
	}
	a.pipe.Systemf(nil, "dhow: artifact %s: %d bytes, sha256 %s", art.GetName(), p.size, p.digest)
	return result{}, true
}

func (a *attempt) saveCache(ctx, jobCtx context.Context, api transferAPI) (result, bool) {
	c := a.grant.GetSpec().GetCache()
	if c == nil || c.GetReadOnly() {
		return result{}, true
	}
	switch {
	case a.cacheKeyValue == "":
		return result{}, true
	case a.cacheHit:
		a.pipe.Systemf(nil, "dhow: cache: the key was found at the start, so it is not saved again")
		return result{}, true
	}
	p, code, why, ok := a.pack(ctx, jobCtx, "cache", c.GetPaths(), c.GetMaxBytes())
	if !ok {
		if why.reason == runnerv1.FailureReason_FAILURE_REASON_ARTIFACT_TOO_LARGE {
			a.pipe.Systemf(nil, "dhow: warning: the cache is larger than %d bytes, so it is not saved", c.GetMaxBytes())
			return result{}, true
		}
		return why, false
	}
	if p.path == "" {
		a.pipe.Systemf(nil, "dhow: warning: the cache paths could not be packed (exit code %d), so the cache is not saved", code)
		return result{}, true
	}
	defer func() { _ = os.Remove(p.path) }()
	err := a.upload(ctx, api, client.BlobKindCache, p)
	var stored bool
	if err == nil {
		err = a.retry(ctx, func(rctx context.Context) error {
			var err error
			stored, err = api.CommitCache(rctx, a.ref, a.cacheKeyValue, p.digest, p.size)
			return err
		})
	}
	switch {
	case err != nil && (client.IsLeaseLost(err) || errors.Is(err, ErrSelfFenced)):
		return a.leaseError(err, runnerv1.FailureReason_FAILURE_REASON_LEASE_LOST, "the cache could not be saved"), false
	case err != nil:
		a.pipe.Systemf(nil, "dhow: warning: the cache was not saved (%s)", client.Reason(err))
	case stored:
		a.pipe.Systemf(nil, "dhow: cache: saved %d bytes", p.size)
	default:
		a.pipe.Systemf(nil, "dhow: cache: another job saved this key first")
	}
	return result{}, true
}
