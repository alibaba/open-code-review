// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package storage

import (
	"bytes"
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// s3Backend mirrors objects to an S3-compatible bucket using the REST API and
// SigV4. It speaks only the four verbs this package needs (PUT, GET, list, and
// DELETE), which keeps the CLI free of a cloud SDK dependency.
type s3Backend struct {
	cfg    S3Config
	client *http.Client
	signer *signer
	// base is the scheme and host that every object URL is built from, plus the
	// bucket path when path-style addressing is in use.
	scheme string
	host   string
}

// s3RequestTimeout bounds one HTTP round trip. Session objects are small enough
// that a request still running after this long is a stuck connection rather
// than a slow transfer.
const s3RequestTimeout = 5 * time.Minute

// maxListPages bounds pagination so a server that keeps reporting truncation
// cannot spin the caller forever.
const maxListPages = 1000

// maxObjectBytes bounds one object read. Session files are JSONL transcripts and
// stay far below this; the cap exists so a misconfigured or hostile
// S3-compatible endpoint cannot make the process allocate without limit when a
// GET is answered with an arbitrarily large body.
const maxObjectBytes = 64 << 20

func newS3Backend(cfg S3Config) (*s3Backend, error) {
	return newS3BackendWithClient(cfg, &http.Client{Timeout: s3RequestTimeout})
}

func newS3BackendWithClient(cfg S3Config, client *http.Client) (*s3Backend, error) {
	endpoint := cfg.Endpoint
	if endpoint == "" {
		endpoint = "https://s3." + cfg.Region + ".amazonaws.com"
	}
	parsed, err := url.Parse(endpoint)
	if err != nil {
		return nil, fmt.Errorf("storage: parse s3 endpoint %q: %w", endpoint, err)
	}
	if parsed.Scheme == "" || parsed.Host == "" {
		return nil, fmt.Errorf("storage: s3 endpoint %q must include a scheme and host", endpoint)
	}
	scheme := parsed.Scheme
	host := parsed.Host
	if !cfg.PathStyle {
		host = cfg.Bucket + "." + host
	}
	return &s3Backend{
		cfg:    cfg,
		client: client,
		signer: newSigner(cfg.AccessKeyID, cfg.SecretAccessKey, cfg.SessionToken, cfg.Region),
		scheme: scheme,
		host:   host,
	}, nil
}

func (b *s3Backend) Name() string { return TypeS3 }

// objectPath returns the unescaped and escaped request path for a key. An empty
// key addresses the bucket itself, which is how a listing request is built: the
// bucket path carries no trailing slash, so a bucket-level GET is
// /<bucket>?list-type=2 rather than /<bucket>//?list-type=2.
func (b *s3Backend) objectPath(key string) (path, escaped string) {
	base := ""
	if b.cfg.PathStyle {
		base = "/" + b.cfg.Bucket
	}
	trimmed := strings.Trim(key, "/")
	if trimmed == "" {
		if base == "" {
			return "/", "/"
		}
		return base, base
	}
	segments := strings.Split(trimmed, "/")
	encoded := make([]string, 0, len(segments))
	for _, segment := range segments {
		encoded = append(encoded, awsEscape(segment))
	}
	return base + "/" + trimmed, base + "/" + strings.Join(encoded, "/")
}

// request builds a signed request for key. An empty key addresses the bucket
// itself; a nil body sends no payload.
func (b *s3Backend) request(ctx context.Context, method, key string, query url.Values, body []byte) (*http.Request, error) {
	path, escaped := b.objectPath(key)
	u := &url.URL{
		Scheme:   b.scheme,
		Host:     b.host,
		Path:     path,
		RawPath:  escaped,
		RawQuery: encodeQuery(query),
	}

	var reader io.Reader
	payloadHash := emptyHash
	if body != nil {
		reader = bytes.NewReader(body)
		payloadHash = hexSHA256(body)
	}

	req, err := http.NewRequestWithContext(ctx, method, u.String(), reader)
	if err != nil {
		return nil, fmt.Errorf("storage: build %s request for %q: %w", method, key, err)
	}
	req.Header.Set("Content-Type", "application/octet-stream")
	if body != nil {
		req.ContentLength = int64(len(body))
	}
	b.signer.sign(req, payloadHash)
	return req, nil
}

// requireKey rejects a key that would address the bucket itself, for the verbs
// that only make sense on an object.
func requireKey(key string) error {
	if strings.Trim(key, "/") == "" {
		return fmt.Errorf("storage: empty object key")
	}
	return nil
}

func (b *s3Backend) Put(ctx context.Context, key string, data []byte) error {
	if err := requireKey(key); err != nil {
		return err
	}
	req, err := b.request(ctx, http.MethodPut, key, nil, data)
	if err != nil {
		return err
	}
	return b.do(req, key)
}

func (b *s3Backend) Get(ctx context.Context, key string) ([]byte, error) {
	if err := requireKey(key); err != nil {
		return nil, err
	}
	req, err := b.request(ctx, http.MethodGet, key, nil, nil)
	if err != nil {
		return nil, err
	}
	resp, err := b.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("storage: get %q: %w", key, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("%w: %s", ErrNotExist, key)
	}
	if err := statusError("get", key, resp); err != nil {
		return nil, err
	}
	return readBounded(resp.Body, key, maxObjectBytes)
}

// readBounded reads one object, refusing a source that carries more than limit
// bytes. The read is capped one byte past the limit, so a source exactly at the
// limit is still accepted while a larger one is reported instead of buffered.
func readBounded(r io.Reader, key string, limit int64) ([]byte, error) {
	data, err := io.ReadAll(io.LimitReader(r, limit+1))
	if err != nil {
		return nil, fmt.Errorf("storage: read %q: %w", key, err)
	}
	if int64(len(data)) > limit {
		return nil, fmt.Errorf("storage: get %q: object exceeds %d bytes", key, limit)
	}
	return data, nil
}

func (b *s3Backend) Delete(ctx context.Context, key string) error {
	if err := requireKey(key); err != nil {
		return err
	}
	req, err := b.request(ctx, http.MethodDelete, key, nil, nil)
	if err != nil {
		return err
	}
	resp, err := b.client.Do(req)
	if err != nil {
		return fmt.Errorf("storage: delete %q: %w", key, err)
	}
	defer resp.Body.Close()
	// S3 reports deleting an absent object as 204, so 404 here means the bucket
	// or the credentials differ from the ones that wrote the object. Treat it as
	// done anyway: the caller asked for the object to not exist, and it does not.
	if resp.StatusCode == http.StatusNotFound {
		return nil
	}
	return statusError("delete", key, resp)
}

// List enumerates the keys under prefix, following continuation tokens.
func (b *s3Backend) List(ctx context.Context, prefix string) ([]string, error) {
	var keys []string
	token := ""
	for page := 0; page < maxListPages; page++ {
		query := url.Values{"list-type": {"2"}}
		if prefix != "" {
			query.Set("prefix", prefix)
		}
		if token != "" {
			query.Set("continuation-token", token)
		}
		// The list request addresses the bucket itself, so the key is empty and
		// only the query carries the selector.
		req, err := b.request(ctx, http.MethodGet, "", query, nil)
		if err != nil {
			return nil, err
		}
		resp, err := b.client.Do(req)
		if err != nil {
			return nil, fmt.Errorf("storage: list %q: %w", prefix, err)
		}
		// Check the status before decoding. statusError quotes up to 512 bytes of
		// the body, and an XML decoder that already drained it would leave the
		// error without the detail the server sent.
		if statusErr := statusError("list", prefix, resp); statusErr != nil {
			resp.Body.Close()
			return nil, statusErr
		}
		var result listBucketResult
		decodeErr := xml.NewDecoder(resp.Body).Decode(&result)
		resp.Body.Close()
		if decodeErr != nil {
			return nil, fmt.Errorf("storage: decode list response for %q: %w", prefix, decodeErr)
		}
		for _, object := range result.Contents {
			keys = append(keys, object.Key)
		}
		if !result.IsTruncated || result.NextContinuationToken == "" {
			return keys, nil
		}
		token = result.NextContinuationToken
	}
	return nil, fmt.Errorf("storage: list %q exceeded %d pages", prefix, maxListPages)
}

func (b *s3Backend) do(req *http.Request, key string) error {
	resp, err := b.client.Do(req)
	if err != nil {
		return fmt.Errorf("storage: %s %q: %w", strings.ToLower(req.Method), key, err)
	}
	defer resp.Body.Close()
	return statusError(strings.ToLower(req.Method), key, resp)
}
