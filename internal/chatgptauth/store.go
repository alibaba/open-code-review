// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package chatgptauth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
)

var ErrNotFound = errors.New("no ChatGPT registration found; run ocr auth login --provider chatgpt")

type Auth struct {
	Revision          uint64    `json:"revision,omitempty"`
	ClientID          string    `json:"client_id"`
	Issuer            string    `json:"issuer"`
	Subject           string    `json:"subject"`
	Email             string    `json:"email,omitempty"`
	AccessToken       string    `json:"access_token,omitempty"`
	RefreshToken      string    `json:"refresh_token,omitempty"`
	IDToken           string    `json:"id_token,omitempty"`
	TokenType         string    `json:"token_type,omitempty"`
	Scopes            []string  `json:"scopes,omitempty"`
	ExpiresAt         time.Time `json:"expires_at"`
	EarliestRefreshAt time.Time `json:"earliest_refresh_at,omitempty"`
	Welcomed          bool      `json:"welcomed,omitempty"`
	Nonce             string    `json:"nonce,omitempty"`
}

func (a Auth) PlanEnabled() bool {
	var direct, invoke bool
	for _, s := range a.Scopes {
		direct = direct || s == "chatgpt.tokens.use.direct"
		invoke = invoke || s == "resource.invoke"
	}
	return direct && invoke
}
func (a *Auth) clear() {
	a.Revision++
	a.AccessToken = ""
	a.RefreshToken = ""
	a.IDToken = ""
	a.Nonce = ""
	a.TokenType = ""
	a.Scopes = nil
	a.ExpiresAt = time.Time{}
	a.EarliestRefreshAt = time.Time{}
}

type Database struct {
	Active   string `json:"active,omitempty"`
	Accounts []Auth `json:"accounts"`
}

func (d *Database) selected(id string) (*Auth, error) {
	if id == "" {
		id = d.Active
	}
	for i := range d.Accounts {
		if d.Accounts[i].ClientID == id {
			return &d.Accounts[i], nil
		}
	}
	return nil, ErrNotFound
}

type Store struct{ Dir string }

func DefaultStore() (Store, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return Store{}, err
	}
	return Store{Dir: filepath.Join(home, ".opencodereview", "auth", "chatgpt")}, nil
}
func (s Store) secure() error {
	if s.Dir == "" {
		return errors.New("ChatGPT store directory is required")
	}
	if err := os.MkdirAll(s.Dir, 0700); err != nil {
		return err
	}
	return os.Chmod(s.Dir, 0700)
}
func (s Store) load() (*Database, error) {
	b, err := os.ReadFile(filepath.Join(s.Dir, "accounts.json"))
	if errors.Is(err, os.ErrNotExist) {
		return &Database{}, nil
	}
	if err != nil {
		return nil, err
	}
	var d Database
	if err := json.Unmarshal(b, &d); err != nil {
		return nil, errors.New("invalid ChatGPT registration database")
	}
	seen := map[string]bool{}
	for _, a := range d.Accounts {
		if a.ClientID == "" || a.ClientID == DynamicClient || a.Issuer != Issuer || a.Subject == "" || seen[a.ClientID] {
			return nil, errors.New("invalid ChatGPT account registration")
		}
		seen[a.ClientID] = true
	}
	if d.Active != "" && !seen[d.Active] {
		return nil, errors.New("invalid active ChatGPT registration")
	}
	return &d, nil
}
func (s Store) Snapshot() (*Database, error) { return s.load() }
func (s Store) Selected(id string) (*Auth, error) {
	d, err := s.load()
	if err != nil {
		return nil, err
	}
	return d.selected(id)
}

var renameFile = os.Rename

func (s Store) write(name string, v any) error {
	if err := s.secure(); err != nil {
		return err
	}
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	b = append(b, '\n')
	f, err := os.CreateTemp(s.Dir, ".write-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	defer f.Close()
	if err := f.Chmod(0600); err != nil {
		return err
	}
	if _, err := f.Write(b); err != nil {
		return err
	}
	if err := f.Sync(); err != nil {
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return renameFile(f.Name(), filepath.Join(s.Dir, name))
}
func (s Store) save(d *Database) error { return s.write("accounts.json", d) }
func (s Store) withLock(ctx context.Context, fn func() error) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := s.secure(); err != nil {
		return err
	}
	f, err := os.OpenFile(filepath.Join(s.Dir, "session.lock"), os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return err
	}
	defer f.Close()
	if err := f.Chmod(0600); err != nil {
		return err
	}
	for {
		acquired, err := tryLock(f)
		if err != nil {
			return err
		}
		if acquired {
			defer unlock(f)
			if err := ctx.Err(); err != nil {
				return err
			}
			return fn()
		}
		timer := time.NewTimer(25 * time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
}
func (s Store) Host(ctx context.Context) (string, error) {
	var id string
	err := s.withLock(ctx, func() error {
		var record struct {
			ID string `json:"id"`
		}
		b, err := os.ReadFile(filepath.Join(s.Dir, "host.json"))
		if errors.Is(err, os.ErrNotExist) {
			u, err := uuid.NewRandom()
			if err != nil {
				return err
			}
			record.ID = u.URN()
			if err := s.write("host.json", record); err != nil {
				return err
			}
		} else if err != nil {
			return err
		} else if json.Unmarshal(b, &record) != nil {
			return errors.New("invalid ChatGPT host record")
		}
		u, err := uuid.Parse(record.ID)
		if err != nil || u.Version() != 4 || u.Variant() != uuid.RFC4122 || !strings.HasPrefix(record.ID, "urn:uuid:") {
			return errors.New("invalid ChatGPT host identifier")
		}
		id = record.ID
		return nil
	})
	return id, err
}
func (s Store) Clear(ctx context.Context, id string) error {
	return s.withLock(ctx, func() error {
		d, err := s.load()
		if err != nil {
			return err
		}
		a, err := d.selected(id)
		if err != nil {
			return err
		}
		a.clear()
		return s.save(d)
	})
}
func (s Store) Select(ctx context.Context, id string) error {
	if id == "" {
		return errors.New("a ChatGPT registration client ID is required")
	}
	return s.withLock(ctx, func() error {
		d, err := s.load()
		if err != nil {
			return err
		}
		if _, err := d.selected(id); err != nil {
			return fmt.Errorf("select ChatGPT account: %w", err)
		}
		d.Active = id
		return s.save(d)
	})
}
