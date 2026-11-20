// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package tool

import (
	"fmt"
	"strconv"
	"strings"
)

// boolArg reads an optional boolean argument. Models sometimes send booleans
// as strings, so those are coerced; any other type is an error rather than a
// silent false, which would run the opposite search.
func boolArg(args map[string]any, name string) (bool, error) {
	switch v := args[name].(type) {
	case nil:
		return false, nil
	case bool:
		return v, nil
	case string:
		b, err := strconv.ParseBool(strings.TrimSpace(v))
		if err != nil {
			return false, fmt.Errorf("%s must be a boolean, got %q", name, v)
		}
		return b, nil
	default:
		return false, fmt.Errorf("%s must be a boolean, got %T", name, v)
	}
}
