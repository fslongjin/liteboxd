package output

import (
	"io"

	"gopkg.in/yaml.v3"
)

// YAMLFormatter formats output as YAML
type YAMLFormatter struct{}

// Write outputs the data as YAML
func (f *YAMLFormatter) Write(w io.Writer, data interface{}) error {
	encoder := yaml.NewEncoder(w)
	return encoder.Encode(data)
}
