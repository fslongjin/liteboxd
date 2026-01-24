package output

import (
	"encoding/json"
	"io"
)

// JSONFormatter formats output as JSON
type JSONFormatter struct{}

// Write outputs the data as JSON
func (f *JSONFormatter) Write(w io.Writer, data interface{}) error {
	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	return encoder.Encode(data)
}
