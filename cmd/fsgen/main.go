// Command fsgen generates typed Go API packages from spec/fatsecret.yaml.
//
// Usage:
//
//	go run ./cmd/fsgen --spec spec/fatsecret.yaml --out .
//	go run ./cmd/fsgen --spec spec/fatsecret.yaml --out . --dry-run
//
// Flags:
//
//	--spec     path to fatsecret.yaml (required)
//	--out      output directory root (required); pkg/api/* written relative to this dir
//	--dry-run  print output filenames without writing files
//
// The generator is idempotent: running it twice on the same spec produces
// byte-identical output files because go/format is deterministic and no
// timestamps are embedded.
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
)

func main() {
	var (
		specPath      = flag.String("spec", "", "path to spec/fatsecret.yaml (required)")
		overridesPath = flag.String("overrides", "", "path to spec/name_overrides.yaml (optional; defaults to <spec-dir>/name_overrides.yaml)")
		outDir        = flag.String("out", "", "output directory root (required)")
		dryRun        = flag.Bool("dry-run", false, "print filenames without writing")
	)
	flag.Parse()

	failed := false
	if *specPath == "" {
		fmt.Fprintln(os.Stderr, "error: --spec is required")
		failed = true
	}
	if *outDir == "" {
		fmt.Fprintln(os.Stderr, "error: --out is required")
		failed = true
	}
	if failed {
		flag.Usage()
		os.Exit(1)
	}

	// Default overrides path next to spec file.
	if *overridesPath == "" {
		*overridesPath = filepath.Join(filepath.Dir(*specPath), "name_overrides.yaml")
	}

	if err := Pipeline(*specPath, *overridesPath, *outDir, *dryRun); err != nil {
		fmt.Fprintf(os.Stderr, "fsgen: %v\n", err)
		os.Exit(1)
	}
}
