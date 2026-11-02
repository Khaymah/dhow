package client

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"

	"connectrpc.com/connect"

	config "github.com/Khaymah/dhow/apps/runner/internal/Config"
	runnerv1 "github.com/Khaymah/dhow/packages/proto/gen/go/dhow/runner/v1"
)

const (
	BlobKindArtifact = "artifact"
	BlobKindCache    = "cache"
	blobErrorMax     = 4 << 10
)

type Blob struct {
	Body io.ReadCloser
	Size int64
}

func blobError(res *http.Response) error {
	var body struct {
		Code string `json:"Code"`
	}
	_ = json.NewDecoder(io.LimitReader(res.Body, blobErrorMax)).Decode(&body)
	if body.Code == "" {
		body.Code = http.StatusText(res.StatusCode)
	}
	code := connect.CodeUnavailable
	switch res.StatusCode {
	case http.StatusConflict, http.StatusPreconditionFailed, http.StatusInsufficientStorage:
		code = connect.CodeFailedPrecondition
	case http.StatusUnauthorized:
		code = connect.CodeUnauthenticated
	case http.StatusForbidden:
		code = connect.CodePermissionDenied
	case http.StatusNotFound, http.StatusGone:
		code = connect.CodeNotFound
	case http.StatusRequestEntityTooLarge:
		code = connect.CodeOutOfRange
	case http.StatusBadRequest, http.StatusUnprocessableEntity, http.StatusLengthRequired, http.StatusMethodNotAllowed:
		code = connect.CodeInvalidArgument
	case http.StatusUpgradeRequired:
		return ErrUpgradeRequired
	}
	return connect.NewError(code, errors.New(body.Code))
}

func (c *Client) blobRequest(ctx context.Context, method, digest string, ref *runnerv1.LeaseRef, body io.Reader, size int64) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, method, c.blobBase+digest, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set(config.ProtocolHeader, config.ProtocolVersion)
	req.Header.Set(config.VersionHeader, c.version)
	req.Header.Set("Dhow-Lease-Id", ref.GetLeaseId())
	req.Header.Set("Dhow-Fencing-Token", strconv.FormatInt(ref.GetFencingToken(), 10))
	if body != nil {
		req.ContentLength = size
		req.Header.Set("Content-Type", "application/gzip")
	}
	return req, nil
}

func (c *Client) doBlob(ctx context.Context, build func() (*http.Request, error)) (*http.Response, error) {
	if c.tokens == nil {
		return nil, ErrNoIdentity
	}
	for attempt := 0; ; attempt++ {
		token, err := c.tokens.get(ctx)
		if err != nil {
			return nil, err
		}
		req, err := build()
		if err != nil {
			return nil, err
		}
		req.Header.Set("Authorization", "Bearer "+token)
		res, err := c.http.Do(req)
		if err != nil {
			return nil, err
		}
		if res.StatusCode < 300 {
			return res, nil
		}
		err = blobError(res)
		_ = res.Body.Close()
		if Code(err) == connect.CodeUnauthenticated && attempt == 0 {
			c.tokens.invalidate(token)
			continue
		}
		return nil, err
	}
}

func (c *Client) PutBlob(ctx context.Context, ref *runnerv1.LeaseRef, kind, digest string, size int64, open func() (io.ReadCloser, error)) error {
	res, err := c.doBlob(ctx, func() (*http.Request, error) {
		body, err := open()
		if err != nil {
			return nil, err
		}
		req, err := c.blobRequest(ctx, http.MethodPut, digest, ref, body, size)
		if err != nil {
			_ = body.Close()
			return nil, err
		}
		req.Header.Set("Dhow-Blob-Kind", kind)
		return req, nil
	})
	if err != nil {
		return err
	}
	_, _ = io.Copy(io.Discard, io.LimitReader(res.Body, blobErrorMax))
	return res.Body.Close()
}

func (c *Client) GetBlob(ctx context.Context, ref *runnerv1.LeaseRef, digest, token string) (Blob, error) {
	res, err := c.doBlob(ctx, func() (*http.Request, error) {
		req, err := c.blobRequest(ctx, http.MethodGet, digest, ref, nil, 0)
		if err != nil {
			return nil, err
		}
		req.Header.Set("Dhow-Download-Token", token)
		return req, nil
	})
	if err != nil {
		return Blob{}, err
	}
	if res.ContentLength < 0 {
		_ = res.Body.Close()
		return Blob{}, fmt.Errorf("the server sent a blob without a length")
	}
	return Blob{Body: res.Body, Size: res.ContentLength}, nil
}

func (c *Client) CommitArtifact(ctx context.Context, ref *runnerv1.LeaseRef, name, digest string, size int64) error {
	_, err := call(ctx, c, config.RPCTimeout, c.artifacts.Commit, &runnerv1.ArtifactServiceCommitRequest{
		Lease: ref, Name: name, Digest: digest, SizeBytes: size,
	})
	return err
}

func (c *Client) LookupCache(ctx context.Context, ref *runnerv1.LeaseRef, key string) (*runnerv1.CacheServiceLookupResponse, error) {
	return call(ctx, c, config.RPCTimeout, c.cache.Lookup, &runnerv1.CacheServiceLookupRequest{Lease: ref, Key: key})
}

func (c *Client) CommitCache(ctx context.Context, ref *runnerv1.LeaseRef, key, digest string, size int64) (bool, error) {
	res, err := call(ctx, c, config.RPCTimeout, c.cache.Commit, &runnerv1.CacheServiceCommitRequest{
		Lease: ref, Key: key, Digest: digest, SizeBytes: size,
	})
	if err != nil {
		return false, err
	}
	return res.GetStored(), nil
}
