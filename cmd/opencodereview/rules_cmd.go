package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/open-code-review/open-code-review/internal/config/rules"
)

func runRules(args []string) error {
	if len(args) == 0 {
		printRulesUsage()
		return nil
	}
	switch args[0] {
	case "check":
		return runRulesCheck(args[1:])
	case "-h", "--help":
		printRulesUsage()
		return nil
	default:
		return fmt.Errorf("unknown rules sub-command: %s\nRun 'ocr rules -h' for usage", args[0])
	}
}

func runRulesCheck(args []string) error {
	a := newOcrFlagSet("ocr rules check")
	var repoDir, rulePath, rulesDir string
	a.StringVar(&repoDir, "repo", "", "root directory of the git repository (default: current dir)")
	a.StringVar(&rulePath, "rule", "", "path to JSON file with custom review rules")
	a.StringVar(&rulesDir, "rules-dir", "", "directory with enterprise/project review rules")
	if err := a.Parse(args); err != nil {
		return err
	}
	if a.showHelp {
		printRulesCheckUsage()
		return nil
	}
	if rulesDir == "" {
		rulesDir = os.Getenv("OCR_RULES_DIR")
	}

	rest := a.fs.Args()
	if len(rest) == 0 {
		printRulesCheckUsage()
		return nil
	}
	filePath := rest[0]

	resolvedRepo, err := resolveRepoDir(repoDir)
	if err != nil {
		return err
	}

	resolver, _, err := rules.NewResolverWithOptions(resolvedRepo, rules.ResolverOptions{
		CustomRulePath: rulePath,
		RulesDir:       rulesDir,
	})
	if err != nil {
		return fmt.Errorf("load rules: %w", err)
	}

	dr, ok := resolver.(rules.DetailResolver)
	if !ok {
		return fmt.Errorf("resolver does not support detail inspection")
	}

	detail := dr.ResolveDetail(strings.ToLower(filePath))

	sourceLabel := map[string]string{
		"custom":             "Custom (--rule)",
		"project":            "Project (.opencodereview/rule.json)",
		"enterprise_project": "Enterprise project (--rules-dir/projects/<project>/rule.json)",
		"enterprise_global":  "Enterprise global (--rules-dir/global.json)",
		"global":             "Global (~/.opencodereview/rule.json)",
		"system":             "System built-in",
	}

	fmt.Printf("File: %s\n", filePath)
	fmt.Printf("Source: %s\n", sourceLabel[detail.Source])
	fmt.Printf("Pattern: %s\n", detail.Pattern)
	fmt.Println("Rule:")
	fmt.Println(strings.Repeat("─", 40))
	fmt.Println(detail.Rule)
	fmt.Println(strings.Repeat("─", 40))

	return nil
}

func printRulesUsage() {
	fmt.Println(`Usage:
  ocr rules <sub-command>

Sub-commands:
  check <file>   Show which review rule applies to a given file path

Use "ocr rules check -h" for more information.`)
}

func printRulesCheckUsage() {
	fmt.Println(`Usage:
  ocr rules check [flags] <file-path>

Show which review rule applies to the given file path, including its source layer and matched pattern.

Flags:
  --repo    Root directory of the git repository (default: current dir)
  --rule    Path to a custom rule JSON file
  --rules-dir
            Directory with enterprise/project review rules

Examples:
  ocr rules check src/main/java/com/example/Foo.java
  ocr rules check --rule custom.json src/main/resources/mapper/UserMapper.xml
  ocr rules check --rules-dir /ocr-data/rules src/main/java/com/example/Foo.java`)
}
