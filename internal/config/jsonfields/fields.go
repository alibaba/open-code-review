// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

// Package jsonfields preserves unknown configuration fields across updates.
package jsonfields

import (
	"encoding/json"
	"reflect"
	"strings"
)

// Names returns the JSON field names of a flat configuration struct.
func Names(value any) []string {
	typeOf := reflect.TypeOf(value)
	for typeOf.Kind() == reflect.Pointer {
		typeOf = typeOf.Elem()
	}

	fields := make([]string, 0, typeOf.NumField())
	for i := 0; i < typeOf.NumField(); i++ {
		field := typeOf.Field(i)
		if field.PkgPath != "" {
			continue
		}
		tag := field.Tag.Get("json")
		name, _, _ := strings.Cut(tag, ",")
		if name != "" && name != "-" {
			fields = append(fields, name)
		}
	}
	return fields
}

// Collect preserves fields not owned by the current configuration version.
func Collect(data []byte, knownFields []string) (map[string]json.RawMessage, error) {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return nil, err
	}

	known := make(map[string]struct{}, len(knownFields))
	for _, field := range knownFields {
		known[field] = struct{}{}
	}
	for field := range fields {
		if _, ok := known[strings.ToLower(field)]; ok {
			delete(fields, field)
		}
	}
	return fields, nil
}

// Merge adds preserved fields without overriding current values.
func Merge(data []byte, unknown map[string]json.RawMessage) ([]byte, error) {
	if len(unknown) == 0 {
		return data, nil
	}

	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return nil, err
	}
	known := make(map[string]struct{}, len(fields))
	for field := range fields {
		known[strings.ToLower(field)] = struct{}{}
	}
	for field, value := range unknown {
		if _, exists := known[strings.ToLower(field)]; !exists {
			fields[field] = value
		}
	}
	return json.Marshal(fields)
}
