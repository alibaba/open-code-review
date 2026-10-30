// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package main

import (
	"fmt"
	"strings"
)

// Token counts run into the millions on a large scan, and a fixed-width frame
// has no room for that in full. The abbreviations follow the convention other
// system monitors use (btop, htop, btop's own token column): keep one decimal
// while the leading digit is single, drop it once there are two or more, and
// step up to M at a million.
//
// This differs from the abbreviation the [ocr] progress lines use, and
// deliberately so: that one rounds, so 3800 tokens print as "4K" in the log
// and as "3.8k" here. The dashboard is the dense surface, and the digit is
// worth the extra character.
//
//	380      -> "380"      999      -> "999"
//	1_000    -> "1k"       3_800    -> "3.8k"
//	12_400   -> "12k"      123_000  -> "123k"
//	1_050_000 -> "1.1M"     12_400_000 -> "12M"
func humanTokens(n int64) string {
	switch {
	case n < 0:
		return "0"
	case n < 1000:
		return fmt.Sprintf("%d", n)
	case n < 10_000:
		return trimZero(float64(n)/1000) + "k"
	case n < 1_000_000:
		return fmt.Sprintf("%dk", n/1000)
	case n < 10_000_000:
		return trimZero(float64(n)/1_000_000) + "M"
	default:
		return fmt.Sprintf("%dM", n/1_000_000)
	}
}

// trimZero renders a float with one decimal place and drops a trailing ".0"
// or bare ".", so the column does not jitter between "4k" and "4.0k" as a
// count crosses a boundary.
func trimZero(f float64) string {
	s := fmt.Sprintf("%.1f", f)
	if !strings.Contains(s, ".") {
		return s
	}
	s = strings.TrimRight(s, "0")
	return strings.TrimSuffix(s, ".")
}

// humanDuration formats a run's elapsed time for the dashboard's header:
// seconds below a minute, then m:ss, and hours once it is reached. It is the
// same spelling the summary line at the end of a run uses, so the number a
// user watches tick up is the number they read at the end, and a run longer
// than an hour reads as hours rather than as minutes past sixty.
func humanDuration(seconds int64) string {
	switch {
	case seconds < 0:
		return "0s"
	case seconds < 60:
		return fmt.Sprintf("%ds", seconds)
	case seconds < 3600:
		return fmt.Sprintf("%dm%02ds", seconds/60, seconds%60)
	default:
		return fmt.Sprintf("%dh%02dm", seconds/3600, (seconds%3600)/60)
	}
}
