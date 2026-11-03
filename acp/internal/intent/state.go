// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package intent

import "sync"

// Pending is the single clarification slot kept for the current command. It is
// deliberately not a conversation memory: phase 6 stores it per session, a new
// command replaces it, and completing or cancelling clears it.
type Pending struct {
	Action     string // "review" or "scan"
	ReviewType string // workspace | range | commit, when Action is review
	From       string
	To         string
	Commit     string
	Paths      []string
	Extra      []string
	Missing    []string
}

// State holds at most one pending clarification.
type State struct {
	mu      sync.Mutex
	pending *Pending
}

// NewState returns an empty clarification state.
func NewState() *State { return &State{} }

// Pending returns a deep copy of the pending clarification, or nil.
func (s *State) Pending() *Pending {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return clonePending(s.pending)
}

func (s *State) setPending(p *Pending) {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.pending = clonePending(p)
}

// Clear drops any pending clarification. Phase 6 calls it on session/cancel and
// on task completion; the parser also clears it when a command replaces or
// resolves the pending one.
func (s *State) Clear() {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.pending = nil
}

func clonePending(p *Pending) *Pending {
	if p == nil {
		return nil
	}
	c := *p
	c.Paths = append([]string(nil), p.Paths...)
	c.Extra = append([]string(nil), p.Extra...)
	c.Missing = append([]string(nil), p.Missing...)
	return &c
}
