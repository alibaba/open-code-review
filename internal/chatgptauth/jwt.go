// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package chatgptauth

import (
	"context"
	"crypto"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"math/big"
	"strings"
	"sync"
	"time"
)

type identity struct {
	Issuer          string          `json:"iss"`
	Subject         string          `json:"sub"`
	Email           string          `json:"email"`
	Audience        json.RawMessage `json:"aud"`
	AuthorizedParty string          `json:"azp"`
	Expires         int64           `json:"exp"`
	Issued          int64           `json:"iat"`
	NotBefore       int64           `json:"nbf"`
	Nonce           string          `json:"nonce"`
}
type jwk struct {
	Kid string `json:"kid"`
	Kty string `json:"kty"`
	Alg string `json:"alg"`
	Use string `json:"use"`
	N   string `json:"n"`
	E   string `json:"e"`
}
type keyCache struct {
	mu    sync.Mutex
	keys  []jwk
	until time.Time
}

func (c *Client) signingKey(ctx context.Context, kid string) (*rsa.PublicKey, error) {
	c.keys.mu.Lock()
	defer c.keys.mu.Unlock()
	find := func() (jwk, bool) {
		for _, k := range c.keys.keys {
			if k.Kid == kid {
				return k, true
			}
		}
		return jwk{}, false
	}
	k, ok := find()
	if !ok || time.Now().After(c.keys.until) {
		var d discovery
		if err := c.discover(ctx, &d); err != nil {
			return nil, err
		}
		var set struct {
			Keys []jwk `json:"keys"`
		}
		if err := c.get(ctx, d.JWKS, &set); err != nil {
			return nil, err
		}
		c.keys.keys = set.Keys
		c.keys.until = time.Now().Add(5 * time.Minute)
		k, ok = find()
	}
	if !ok || k.Kty != "RSA" || k.Alg != "RS256" || (k.Use != "" && k.Use != "sig") {
		return nil, errors.New("ID-token signing key is not supported")
	}
	n, err := base64.RawURLEncoding.DecodeString(k.N)
	if err != nil {
		return nil, errors.New("invalid RSA signing key")
	}
	e, err := base64.RawURLEncoding.DecodeString(k.E)
	if err != nil || len(e) == 0 || len(e) > 4 {
		return nil, errors.New("invalid RSA exponent")
	}
	var exponent int64
	for _, b := range e {
		exponent = exponent*256 + int64(b)
	}
	modulus := new(big.Int).SetBytes(n)
	if modulus.BitLen() < 2048 || exponent < 3 || exponent > 1<<31-1 || exponent%2 == 0 {
		return nil, errors.New("invalid RSA signing key")
	}
	return &rsa.PublicKey{N: modulus, E: int(exponent)}, nil
}
func (c *Client) verify(ctx context.Context, token, clientID, nonce string) (identity, error) {
	var claims identity
	p := strings.Split(token, ".")
	if len(p) != 3 || len(token) > 1<<20 {
		return claims, errors.New("invalid ID token")
	}
	decode := func(s string, v any) error {
		b, err := base64.RawURLEncoding.DecodeString(s)
		if err != nil {
			return err
		}
		return json.Unmarshal(b, v)
	}
	var header struct {
		Alg  string   `json:"alg"`
		Kid  string   `json:"kid"`
		Crit []string `json:"crit"`
	}
	if err := decode(p[0], &header); err != nil || header.Alg != "RS256" || header.Kid == "" || len(header.Crit) > 0 {
		return claims, errors.New("invalid ID-token signing header")
	}
	key, err := c.signingKey(ctx, header.Kid)
	if err != nil {
		return claims, err
	}
	sig, err := base64.RawURLEncoding.DecodeString(p[2])
	if err != nil {
		return claims, errors.New("invalid ID-token signature")
	}
	digest := sha256.Sum256([]byte(p[0] + "." + p[1]))
	if rsa.VerifyPKCS1v15(key, crypto.SHA256, digest[:], sig) != nil {
		return claims, errors.New("invalid ID-token signature")
	}
	if err := decode(p[1], &claims); err != nil {
		return claims, errors.New("invalid ID-token claims")
	}
	var audiences []string
	var single string
	if json.Unmarshal(claims.Audience, &single) == nil {
		audiences = []string{single}
	} else if json.Unmarshal(claims.Audience, &audiences) != nil {
		return claims, errors.New("invalid ID-token audience")
	}
	found := false
	for _, a := range audiences {
		found = found || a == clientID
	}
	now := time.Now().Unix()
	if claims.Issuer != Issuer || claims.Subject == "" || !found || claims.Expires <= now || claims.Issued <= 0 || claims.Issued > now+5 || claims.NotBefore > now+5 || (claims.AuthorizedParty != "" && claims.AuthorizedParty != clientID) || (len(audiences) > 1 && claims.AuthorizedParty != clientID) {
		return claims, errors.New("ID-token issuer, audience, subject or lifetime did not match")
	}
	if nonce != "" && claims.Nonce != nonce {
		return claims, errors.New("ID-token nonce did not match")
	}
	return claims, nil
}
