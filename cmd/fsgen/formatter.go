package main

import (
	"fmt"
	"go/format"
)

// FormatSource applies go/format.Source to the given Go source bytes.
// Returns the formatted bytes, or an error including context about which file
// failed. This function is deterministic: identical input always produces
// identical output regardless of invocation order or OS.
func FormatSource(relPath string, src []byte) ([]byte, error) {
	formatted, err := format.Source(src)
	if err != nil {
		// Include the first 2 KB of unformatted source in the error for diagnostics.
		preview := src
		if len(preview) > 2048 {
			preview = preview[:2048]
		}
		return nil, fmt.Errorf("formatter: go/format failed for %q: %w\n--- source preview ---\n%s", relPath, err, preview)
	}
	return formatted, nil
}
