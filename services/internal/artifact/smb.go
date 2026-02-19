package artifact

import (
	"fmt"
	"os"
	"path/filepath"
)

// ExportToShare writes JSON and PDF artifacts to the specified directory
// (typically a CIFS mount point) under a subdirectory named by RMA number.
func ExportToShare(jsonData, pdfData []byte, rmaNumber, mountPath string) error {
	dir := filepath.Join(mountPath, rmaNumber)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("create directory %s: %w", dir, err)
	}

	jsonPath := filepath.Join(dir, rmaNumber+".json")
	if err := os.WriteFile(jsonPath, jsonData, 0644); err != nil {
		return fmt.Errorf("write JSON: %w", err)
	}

	pdfPath := filepath.Join(dir, rmaNumber+".pdf")
	if err := os.WriteFile(pdfPath, pdfData, 0644); err != nil {
		return fmt.Errorf("write PDF: %w", err)
	}

	return nil
}
