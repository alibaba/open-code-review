// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package diff

import (
	"fmt"
	"strings"
	"testing"

	"github.com/alibaba/open-code-review/internal/model"
)

func BenchmarkResolveCommentRepeatedPrefix(b *testing.B) {
	for _, lines := range []int{5000, 20000} {
		b.Run(fmt.Sprintf("lines=%d", lines), func(b *testing.B) {
			d := model.Diff{NewPath: "values.go", NewFileContent: strings.Repeat("value(),\n", lines) + "last(),\n"}
			quote := strings.Repeat("value(),\n", 255) + "last(),"
			b.ReportAllocs()
			b.ResetTimer()
			for b.Loop() {
				cm := model.LlmComment{Path: "values.go", ExistingCode: quote}
				if !ResolveComment(&cm, &d) || cm.StartLine != lines-254 || cm.EndLine != lines+1 {
					b.Fatalf("unexpected match: %+v", cm)
				}
			}
		})
	}
}

func BenchmarkResolveCommentShortPrefix(b *testing.B) {
	d := model.Diff{NewPath: "values.go", NewFileContent: "first(),\nsecond(),\n" + strings.Repeat("value(),\n", 5000)}
	quote := "first(),\nsecond(),"
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		cm := model.LlmComment{Path: "values.go", ExistingCode: quote}
		if !ResolveComment(&cm, &d) || cm.StartLine != 1 || cm.EndLine != 2 {
			b.Fatalf("unexpected match: %+v", cm)
		}
	}
}
