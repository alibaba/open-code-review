// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package tool

import "fmt"

// boolArg reads an optional boolean argument. A missing key means false.
// Models sometimes send booleans as strings, so exactly "true" and "false"
// are accepted too; anything else is an error naming the field rather than a
// silent false, which would run the opposite search.
func boolArg(args map[string]any, name string) (bool, error) {
	value, present := args[name]
	if !present {
		return false, nil
	}
	switch v := value.(type) {
	case bool:
		return v, nil
	case string:
		switch v {
		case "true":
			return true, nil
		case "false":
			return false, nil
		}
		return false, fmt.Errorf("%s must be a boolean, got %q", name, v)
	case nil:
		return false, fmt.Errorf("%s must be a boolean, got null", name)
	default:
		return false, fmt.Errorf("%s must be a boolean, got %T", name, v)
	}
}

// NormalizeBoolArgs replaces each named argument that is present with its
// boolean value, so callers that validate raw arguments against the
// published boolean schema accept "true" and "false" the same way the tools
// do. Other arguments are left untouched.
func NormalizeBoolArgs(args map[string]any, names ...string) error {
	for _, name := range names {
		if _, present := args[name]; !present {
			continue
		}
		value, err := boolArg(args, name)
		if err != nil {
			return err
		}
		args[name] = value
	}
	return nil
}
