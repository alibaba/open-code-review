// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package knowledge

import "testing"

func TestDiscoveryAcceptsTheVerifiedEnclosingCaller(t *testing.T) {
	const p = "orders/write.py"
	const first = "    record = Record.create("
	const output = "        amount=charged,"
	sources := map[string]discoverySource{
		"s1": {ID: "s1", Kind: "code", Path: p, StartLine: 40, Text: first + "\n" + output,
			Caller: &discoveryCaller{Function: "finalize_order", Callee: "charge", Role: "result_use", CallLine: 24, ResultLine: 41}},
	}
	for _, tt := range []struct {
		name, identifier, quote string
		want                    bool
	}{
		{"uses result in this routine", "finalize_order", output, true},
		{"includes full use", "finalize_order", first + "\n" + output, true},
		{"omits the actual use", "finalize_order", first, false},
		{"different routine", "cancel_order", output, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			rule := discoveryRule{Category: CategoryBusinessRule, Scope: p, Claim: "Orders store the charged amount.",
				Identifiers: []string{tt.identifier}, Citations: []discoveryQuote{{Source: "s1", Quote: tt.quote}}}
			_, err := rule.fact("", sources)
			if (err == nil) != tt.want {
				t.Fatalf("accepted = %v, want %v (%v)", err == nil, tt.want, err)
			}
		})
	}
}
