package output

import (
	"io"
	"strings"
)

// Format represents the output format
type Format string

const (
	FormatTable Format = "table"
	FormatJSON  Format = "json"
	FormatYAML  Format = "yaml"
)

// Formatter is an interface for output formatting
type Formatter interface {
	// Write outputs the data to the writer
	Write(w io.Writer, data interface{}) error
}

// ParseFormat parses a format string
func ParseFormat(s string) Format {
	switch strings.ToLower(s) {
	case "json":
		return FormatJSON
	case "yaml", "yml":
		return FormatYAML
	default:
		return FormatTable
	}
}

// NewFormatter creates a new formatter for the given format
func NewFormatter(format Format) Formatter {
	switch format {
	case FormatJSON:
		return &JSONFormatter{}
	case FormatYAML:
		return &YAMLFormatter{}
	default:
		return &TableFormatter{}
	}
}

// NewTableFormatter creates a table formatter with specified fields
func NewTableFormatter(fields []string) Formatter {
	return &TableFormatter{Fields: fields}
}

// NewTableFormatterWithLabels creates a table formatter with specified fields and custom labels
func NewTableFormatterWithLabels(fields []string, labels map[string]string) Formatter {
	return &TableFormatter{Fields: fields, FieldLabels: labels}
}
