// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package jsonfields

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestPreserveUnknownFields(t *testing.T) {
	type config struct {
		Known    string `json:"known,omitempty"`
		Hidden   string `json:"-"`
		Untagged string
		private  bool
	}
	names := Names(&config{})
	if !reflect.DeepEqual(names, []string{"known"}) {
		t.Fatal(names)
	}
	unknown, err := Collect([]byte(`{"KNOWN":"old","future":9007199254740993}`), names)
	if err != nil {
		t.Fatal(err)
	}
	if len(unknown) != 1 || string(unknown["future"]) != "9007199254740993" {
		t.Fatal(unknown)
	}
	merged, err := Merge([]byte(`{}`), unknown)
	if err != nil || string(merged) != `{"future":9007199254740993}` {
		t.Fatalf("%s: %v", merged, err)
	}
	merged, err = Merge([]byte(`{"future":false}`), unknown)
	if err != nil || string(merged) != `{"future":false}` {
		t.Fatalf("%s: %v", merged, err)
	}
	merged, err = Merge([]byte(`{}`), nil)
	if err != nil || string(merged) != `{}` {
		t.Fatalf("%s: %v", merged, err)
	}
	if _, err := Collect([]byte(`!`), names); err == nil {
		t.Fatal("accepted invalid input")
	}
	if _, err := Merge([]byte(`!`), unknown); err == nil {
		t.Fatal("accepted invalid input")
	}
	if _, err := Merge([]byte(`{}`), map[string]json.RawMessage{"invalid": json.RawMessage(`!`)}); err == nil {
		t.Fatal("accepted invalid preserved value")
	}
}
