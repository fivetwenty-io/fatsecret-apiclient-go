package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/fivetwenty-io/fatsecret-apiclient-go/cmd/fsgen/ir"
)

// Pipeline executes the full generate pipeline:
//
//  1. Parse spec YAML → *ir.Spec
//  2. Parse overrides YAML → []ir.OverrideYAML
//  3. Build namespaces (naming algo, type resolution, dup check) → []ir.Namespace
//  4. Render templates → []RenderedFile
//  5. Render compatibility matrix → RenderedFile
//  6. Format each rendered file via go/format
//  7. Write (or dry-run print) output files
//
// When dryRun is true, file names are printed to stdout but no files are written.
func Pipeline(specPath, overridesPath, outDir string, dryRun bool) error {
	// Step 1: parse spec.
	spec, err := ir.ParseSpec(specPath)
	if err != nil {
		return fmt.Errorf("pipeline: parse spec: %w", err)
	}

	// Step 2: parse overrides.
	overrides, err := ir.ParseOverrides(overridesPath)
	if err != nil {
		return fmt.Errorf("pipeline: parse overrides: %w", err)
	}

	// Step 3: build namespaces.
	namespaces, err := ir.BuildNamespaces(spec, overrides)
	if err != nil {
		return fmt.Errorf("pipeline: build namespaces: %w", err)
	}

	// Step 4: render namespace templates.
	rendered, err := RenderAll(namespaces)
	if err != nil {
		return fmt.Errorf("pipeline: render namespaces: %w", err)
	}

	// Step 5: render compatibility matrix.
	matrix, err := RenderCompatibility(spec)
	if err != nil {
		return fmt.Errorf("pipeline: render compatibility matrix: %w", err)
	}
	rendered = append(rendered, matrix)

	// Step 6+7: format and write (or dry-run).
	for _, f := range rendered {
		formatted, err := FormatSource(f.RelPath, f.Content)
		if err != nil {
			return err
		}
		f.Content = formatted

		if dryRun {
			fmt.Println(f.RelPath)
			continue
		}

		absPath := filepath.Join(outDir, filepath.FromSlash(f.RelPath))
		if err := writeFile(absPath, f.Content); err != nil {
			return fmt.Errorf("pipeline: write %q: %w", absPath, err)
		}
	}

	return nil
}

// writeFile creates parent directories as needed and writes content to path.
// Existing files are overwritten; permissions are 0644.
func writeFile(path string, content []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil { //nolint:gosec // G301: 0750 is appropriate for generated output directories
		return fmt.Errorf("mkdir %q: %w", filepath.Dir(path), err)
	}
	return os.WriteFile(path, content, 0o644) // #nosec G306 -- generated Go source files use standard 0644 so build tools can read them
}
