// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

// Smoke checks Sem through OCR's actual MCP client without calling a model.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/alibaba/open-code-review/internal/mcp"
)

func main() {
	repo := flag.String("repo", "", "Git checkout to analyze")
	entity := flag.String("entity", "", "Entity name to retrieve")
	binary := flag.String("sem", "sem", "Sem executable")
	flag.Parse()
	if *repo == "" || *entity == "" {
		flag.Usage()
		os.Exit(2)
	}
	if err := run(*repo, *entity, *binary); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(repo, entity, binary string) error {
	root, err := filepath.Abs(repo)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	client, err := mcp.NewClient(ctx, "sem", binary, []string{"mcp"},
		[]string{"SEM_CLOUD=0", "SEM_MCP_CLOUD=0"}, root, "sem-example")
	if err != nil {
		return err
	}
	defer client.Close()
	result, err := client.CallTool(ctx, "sem_context", map[string]any{
		"entity_name": entity, "token_budget": 1500,
	})
	if err != nil {
		return err
	}
	fmt.Println(result)
	return nil
}
