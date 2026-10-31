// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

// Package storage mirrors review artifacts to a store outside the local cache
// directory, so that a run's session records survive a machine or container
// that is not the one that produced them.
//
// The local directory under $HOME/.opencodereview stays authoritative for the
// CLI: every backend here is a replica that is written to and read through, and
// the default configuration keeps no replica at all, so an unconfigured
// install behaves exactly as it did before this package existed.
package storage

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

// ErrNotExist reports that the requested object is not stored in the backend.
// Callers translate it into their own missing-artifact error, the same way they
// translate os.IsNotExist for the local cache.
var ErrNotExist = errors.New("storage: object does not exist")

// Backend stores opaque byte objects under caller-chosen keys.
//
// Implementations must be safe for concurrent use: a review run mirrors several
// sessions and reads are issued from the viewer while a run is writing.
type Backend interface {
	// Name identifies the backend in user-facing messages, e.g. "s3".
	Name() string
	// Put stores data at key, replacing any previous object.
	Put(ctx context.Context, key string, data []byte) error
	// Get returns the object stored at key, or ErrNotExist.
	Get(ctx context.Context, key string) ([]byte, error)
	// List returns every key that begins with prefix, in lexicographic order.
	List(ctx context.Context, prefix string) ([]string, error)
	// Delete removes key. Deleting an absent key is not an error, so a retried
	// delete of an already-removed session succeeds.
	Delete(ctx context.Context, key string) error
}

// Storage types accepted by Config.Type.
const (
	// TypeLocal keeps the local directory as the only store. It is the default,
	// and it opens no backend at all.
	TypeLocal = "local"
	// TypeS3 mirrors objects to an S3-compatible object store.
	TypeS3 = "s3"
	// TypeFilesystem mirrors objects into a shared directory, e.g. a mounted
	// network volume that outlives the container that wrote it.
	TypeFilesystem = "filesystem"
)

// S3Config describes an S3-compatible endpoint.
type S3Config struct {
	// Endpoint is the service URL, e.g. https://s3.mycompany.internal or
	// https://s3.us-west-2.amazonaws.com. Empty selects the AWS endpoint for
	// Region.
	Endpoint string
	// Bucket is the bucket that holds the mirrored objects.
	Bucket string
	// Region is the SigV4 signing region.
	Region string
	// AccessKeyID and SecretAccessKey are the credentials used to sign requests.
	AccessKeyID     string
	SecretAccessKey string
	// SessionToken is set for temporary credentials, and is sent as
	// x-amz-security-token.
	SessionToken string
	// PathStyle addresses objects as <endpoint>/<bucket>/<key> instead of
	// <bucket>.<endpoint>/<key>. Most S3-compatible servers (MinIO, Ceph,
	// localstack) require it.
	PathStyle bool
}

// Config selects and configures the replica backend.
type Config struct {
	// Type is one of TypeLocal, TypeS3 or TypeFilesystem. Empty means TypeLocal.
	Type string
	// Prefix is prepended to every object key, so several environments can share
	// one bucket or directory.
	Prefix string
	// Path is the root directory of the filesystem backend.
	Path string
	// S3 configures the s3 backend.
	S3 S3Config
}

// normalized returns the config with defaults applied, and reports whether a
// local (that is, no-replica) configuration was selected.
func (c Config) normalized() (Config, bool) {
	out := c
	out.Type = strings.ToLower(strings.TrimSpace(out.Type))
	if out.Type == "" {
		out.Type = TypeLocal
	}
	out.Prefix = strings.Trim(strings.TrimSpace(out.Prefix), "/")
	if out.Prefix != "" {
		out.Prefix += "/"
	}
	out.Path = strings.TrimSpace(out.Path)
	out.S3.Endpoint = strings.TrimRight(strings.TrimSpace(out.S3.Endpoint), "/")
	out.S3.Bucket = strings.TrimSpace(out.S3.Bucket)
	out.S3.Region = strings.TrimSpace(out.S3.Region)
	return out, out.Type == TypeLocal
}

// Validate reports whether the configuration can be opened.
func (c Config) Validate() error {
	cfg, _ := c.normalized()
	switch cfg.Type {
	case TypeLocal:
		return nil
	case TypeFilesystem:
		if cfg.Path == "" {
			return fmt.Errorf("storage type %q requires path", TypeFilesystem)
		}
		return nil
	case TypeS3:
		if cfg.S3.Bucket == "" {
			return fmt.Errorf("storage type %q requires s3.bucket", TypeS3)
		}
		if cfg.S3.Region == "" {
			return fmt.Errorf("storage type %q requires s3.region", TypeS3)
		}
		if cfg.S3.AccessKeyID == "" || cfg.S3.SecretAccessKey == "" {
			return fmt.Errorf("storage type %q requires s3.access_key_id and s3.secret_access_key", TypeS3)
		}
		return nil
	default:
		return fmt.Errorf("unknown storage type %q: use %s, %s or %s",
			cfg.Type, TypeLocal, TypeS3, TypeFilesystem)
	}
}

// Open returns the backend described by cfg.
//
// A local configuration returns a nil Backend and a nil error: there is nothing
// to replicate to, and callers are expected to skip mirroring rather than write
// the same bytes twice. Every other type returns a usable Backend. When a prefix
// is configured, it is applied here, so callers pass store-relative keys.
func Open(cfg Config) (Backend, error) {
	normalized, local := cfg.normalized()
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	if local {
		return nil, nil
	}
	var backend Backend
	var err error
	switch normalized.Type {
	case TypeFilesystem:
		backend = newFileBackend(normalized.Path)
	case TypeS3:
		backend, err = newS3Backend(normalized.S3)
	default:
		return nil, fmt.Errorf("unknown storage type %q", normalized.Type)
	}
	if err != nil {
		return nil, err
	}
	if normalized.Prefix != "" {
		return &prefixedBackend{inner: backend, prefix: normalized.Prefix}, nil
	}
	return backend, nil
}

// Key joins non-empty key segments with "/" and drops the ones that are empty,
// so callers can build keys from optional prefixes.
func Key(segments ...string) string {
	parts := make([]string, 0, len(segments))
	for _, segment := range segments {
		trimmed := strings.Trim(segment, "/")
		if trimmed != "" {
			parts = append(parts, trimmed)
		}
	}
	return strings.Join(parts, "/")
}
