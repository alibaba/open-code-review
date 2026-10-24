// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package storage

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"
)

// signer implements AWS Signature Version 4 for the S3 service, which is the
// signature every S3-compatible server (MinIO, Ceph RGW, localstack, R2, the
// cloud providers) accepts.
type signer struct {
	accessKey    string
	secretKey    string
	sessionToken string
	region       string
	now          func() time.Time
}

const (
	algorithm = "AWS4-HMAC-SHA256"
	serviceS3 = "s3"
)

func newSigner(accessKey, secretKey, sessionToken, region string) *signer {
	return &signer{
		accessKey:    accessKey,
		secretKey:    secretKey,
		sessionToken: sessionToken,
		region:       region,
		now:          time.Now,
	}
}

// sign adds the SigV4 headers for one request. payloadHash is the hex SHA-256 of
// the body, or emptyHash for a request that sends none. Every header named in
// extra is covered by the signature as well, for a request that carries a header
// whose value must not be tamperable in transit (a byte range, for instance).
func (s *signer) sign(req *http.Request, payloadHash string, extra ...string) {
	now := s.now().UTC()
	amzDate := now.Format("20060102T150405Z")
	dateStamp := now.Format("20060102")

	req.Header.Set("x-amz-date", amzDate)
	req.Header.Set("x-amz-content-sha256", payloadHash)
	if s.sessionToken != "" {
		req.Header.Set("x-amz-security-token", s.sessionToken)
	}

	headerValues := map[string]string{
		"host":                 req.Host,
		"x-amz-content-sha256": payloadHash,
		"x-amz-date":           amzDate,
	}
	if s.sessionToken != "" {
		headerValues["x-amz-security-token"] = s.sessionToken
	}
	for _, name := range extra {
		lowered := strings.ToLower(strings.TrimSpace(name))
		if lowered == "" {
			continue
		}
		headerValues[lowered] = req.Header.Get(lowered)
	}

	signedHeaders := make([]string, 0, len(headerValues))
	for name := range headerValues {
		signedHeaders = append(signedHeaders, name)
	}
	sort.Strings(signedHeaders)

	var canonicalHeaders strings.Builder
	for _, name := range signedHeaders {
		canonicalHeaders.WriteString(name + ":" + headerValues[name] + "\n")
	}

	canonicalRequest := strings.Join([]string{
		req.Method,
		canonicalURI(req.URL),
		canonicalQuery(req.URL),
		canonicalHeaders.String(),
		strings.Join(signedHeaders, ";"),
		payloadHash,
	}, "\n")

	scope := strings.Join([]string{dateStamp, s.region, serviceS3, "aws4_request"}, "/")
	stringToSign := strings.Join([]string{
		algorithm,
		amzDate,
		scope,
		hexSHA256([]byte(canonicalRequest)),
	}, "\n")

	signature := hex.EncodeToString(hmacSHA256(s.signingKey(dateStamp), []byte(stringToSign)))

	req.Header.Set("Authorization", fmt.Sprintf(
		"%s Credential=%s/%s, SignedHeaders=%s, Signature=%s",
		algorithm, s.accessKey, scope, strings.Join(signedHeaders, ";"), signature,
	))
}

func (s *signer) signingKey(dateStamp string) []byte {
	kDate := hmacSHA256([]byte("AWS4"+s.secretKey), []byte(dateStamp))
	kRegion := hmacSHA256(kDate, []byte(s.region))
	kService := hmacSHA256(kRegion, []byte(serviceS3))
	return hmacSHA256(kService, []byte("aws4_request"))
}

// emptyHash is the hex SHA-256 of an empty body, which SigV4 requires for
// requests that carry none.
var emptyHash = hexSHA256(nil)

func hmacSHA256(key, data []byte) []byte {
	mac := hmac.New(sha256.New, key)
	mac.Write(data)
	return mac.Sum(nil)
}

func hexSHA256(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// canonicalURI returns the URI-encoded path SigV4 hashes. The path is escaped
// when the request is built; AWS hashes that encoded form, so it is returned
// unchanged rather than escaped a second time.
func canonicalURI(u *url.URL) string {
	if u.EscapedPath() == "" {
		return "/"
	}
	return u.EscapedPath()
}

// canonicalQuery returns the sorted, URI-encoded query string SigV4 hashes.
func canonicalQuery(u *url.URL) string {
	if u.RawQuery == "" {
		return ""
	}
	values, err := url.ParseQuery(u.RawQuery)
	if err != nil {
		return u.RawQuery
	}
	return encodeQuery(values)
}

// encodeQuery renders values the way SigV4 canonicalizes a query string: sorted
// by name and value, with unreserved characters left literal. The request URL is
// built from this same string, so the URL and the signature always agree.
func encodeQuery(values url.Values) string {
	if len(values) == 0 {
		return ""
	}
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	var pairs []string
	for _, key := range keys {
		items := append([]string(nil), values[key]...)
		sort.Strings(items)
		for _, value := range items {
			pairs = append(pairs, awsEscape(key)+"="+awsEscape(value))
		}
	}
	return strings.Join(pairs, "&")
}

// awsEscape percent-encodes one path segment or query component the way SigV4
// requires: unreserved characters stay literal and everything else, space
// included, becomes %XX.
func awsEscape(value string) string {
	const unreserved = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789-_.~"
	var b strings.Builder
	b.Grow(len(value))
	for i := 0; i < len(value); i++ {
		c := value[i]
		if strings.IndexByte(unreserved, c) >= 0 {
			b.WriteByte(c)
			continue
		}
		fmt.Fprintf(&b, "%%%02X", c)
	}
	return b.String()
}
