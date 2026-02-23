//go:build windows

package printer

import (
	"fmt"

	winprinter "github.com/alexbrainman/printer"
)

// GetDefaultPrinterName returns the name of the default Windows printer.
func GetDefaultPrinterName() (string, error) {
	name := winprinter.Default()
	if name == "" {
		return "", fmt.Errorf("no default printer found")
	}
	return name, nil
}

// ListPrinters returns available printer names.
func ListPrinters() ([]string, error) {
	return nil, fmt.Errorf("ListPrinters not implemented")
}
