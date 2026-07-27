package main

import (
	"fmt"
	"net/http"
	"time"

	"github.com/open-code-review/open-code-review/internal/release"
)

// runUpdate handles the `ocr update` command.
//
// Usage:
//
//	ocr update              Check for and install the latest version
//	ocr update --check      Only check if an update is available
//	ocr update --force      Reinstall even if already on the latest version
//	ocr update --version v1.7.17   Update to a specific version
func runUpdate(args []string) error {
	fs := newOcrFlagSet("ocr update")
	var checkOnly, force, help bool
	var targetVersion string
	fs.BoolVarP(&checkOnly, "check", "c", false, "Only check if an update is available; do not install")
	fs.BoolVarP(&force, "force", "f", false, "Reinstall even if already on the latest version")
	fs.StringVar(&targetVersion, "version", "", "Update to a specific version (e.g. v1.7.17)")
	fs.BoolVar(&help, "help", false, "Show update command help")

	fs.fs.Usage = printUpdateUsage

	if err := fs.Parse(args); err != nil {
		return err
	}
	if help || fs.showHelp {
		printUpdateUsage()
		return nil
	}

	// Use a short-timeout client for API calls and a long-timeout client
	// for binary downloads, since http.Client.Timeout covers the entire
	// request lifecycle including reading the response body.
	apiClient := &http.Client{Timeout: 30 * time.Second}
	downloadClient := &http.Client{Timeout: 10 * time.Minute}

	method := release.DetectInstallMethod()

	// Determine the target version, normalised to "vX.Y.Z" form.
	var version string
	if targetVersion != "" {
		version = release.NormaliseVersion(targetVersion)
		if _, _, _, ok := release.ParseSemver(version); !ok {
			return fmt.Errorf("invalid version: %s", targetVersion)
		}
	} else {
		fmt.Print("Checking for the latest release...\n")
		latest, err := release.FetchLatestRelease(apiClient)
		if err != nil {
			return fmt.Errorf("failed to check latest release: %w", err)
		}
		version = latest.TagName
	}

	// Compare using bare versions (no "v" prefix).
	currentVersion := release.BareVersion(Version)
	latestVersion := release.BareVersion(version)

	// Check mode: just report.
	if checkOnly {
		if release.IsNewerVersion(latestVersion, currentVersion) {
			fmt.Printf("A new version is available: %s (current: %s)\n", version, displayVersion())
			fmt.Printf("Run 'ocr update' to install it.\n")
		} else if targetVersion != "" {
			fmt.Printf("Version %s is not newer than current (%s)\n", version, displayVersion())
		} else {
			fmt.Printf("ocr is up to date (%s)\n", displayVersion())
		}
		return nil
	}

	// Decide whether to proceed.
	if !force && !release.IsNewerVersion(latestVersion, currentVersion) {
		fmt.Printf("ocr is already up to date (%s)\n", displayVersion())
		fmt.Println("Use --force to reinstall anyway.")
		return nil
	}

	if force {
		fmt.Printf("Force updating to %s...\n", version)
	} else {
		fmt.Printf("Updating from %s to %s...\n", displayVersion(), version)
	}

	// Route based on install method.
	switch method {
	case release.InstallNPM:
		return npmUpdateHint(version)
	case release.InstallSource:
		fmt.Println("Detected source build installation.")
		fmt.Println("To update, pull the latest source and rebuild:")
		fmt.Println("  git pull && make build && sudo cp dist/opencodereview /usr/local/bin/ocr")
		fmt.Printf("\nOr install a pre-built binary:\n  curl -fsSL https://raw.githubusercontent.com/%s/main/install.sh | sh\n",
			release.GitHubRepo)
		return nil
	default: // InstallStatic
		result, err := release.DownloadAndReplace(downloadClient, version)
		if err != nil {
			return fmt.Errorf("update failed: %w", err)
		}
		fmt.Printf("\nSuccessfully updated ocr to %s\n", version)
		fmt.Printf("Installed at: %s\n", result.Path)
		fmt.Println("\nNote: The running process still uses the old binary. Run 'ocr version' to verify.")
		return nil
	}
}

// npmUpdateHint prints instructions for updating an NPM-installed ocr.
func npmUpdateHint(version string) error {
	fmt.Println("Detected NPM installation. Self-update is handled by the NPM wrapper.")
	fmt.Println("To update manually, run:")
	fmt.Printf("  npm install -g %s@%s\n", release.NPMPackageName, release.BareVersion(version))
	fmt.Printf("\nOr let the wrapper auto-update on the next run (set OCR_NO_UPDATE=1 to disable).\n")
	return nil
}

// displayVersion returns the version string for display, falling back to
// "dev" when unset (e.g. during development builds).
func displayVersion() string {
	if Version == "" || Version == "dev" {
		return "dev"
	}
	return Version
}

func printUpdateUsage() {
	fmt.Println(`Update ocr to the latest or a specific version.

Usage:
  ocr update [flags]

Flags:
  --check, -c        Only check if an update is available; do not install
  --force, -f        Reinstall even if already on the latest version
  --version <ver>    Update to a specific version (e.g. v1.7.17)
  -h, --help         Show this help message

Examples:
  ocr update                  Update to the latest release
  ocr update --check          Check if an update is available
  ocr update --version v1.7.17   Install a specific version
  ocr update --force          Reinstall the current version

Install methods:
  Static binary  Downloaded and replaced in-place (with sha256 verification)
  NPM            Prints the npm command to run (wrapper auto-updates by default)
  Source build   Prints build instructions`)
}
