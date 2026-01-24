package output

import (
	"fmt"
	"io"
	"os"
	"reflect"
	"strings"
	"time"

	"github.com/olekukonko/tablewriter"
)

// TableFormatter formats output as a table
type TableFormatter struct {
	// Fields specifies which fields to include (empty = all fields)
	Fields []string
	// FieldLabels provides custom labels for fields (key = field path, value = display label)
	FieldLabels map[string]string
}

// Write outputs the data as a table
func (f *TableFormatter) Write(w io.Writer, data interface{}) error {
	// Handle slices
	val := reflect.ValueOf(data)
	if val.Kind() == reflect.Slice {
		if val.Len() == 0 {
			fmt.Fprintln(w, "No items found")
			return nil
		}
		return f.writeSlice(w, val)
	}

	// Handle single items - convert to slice
	slice := reflect.ValueOf([]interface{}{data})
	return f.writeSlice(w, slice)
}

func (f *TableFormatter) writeSlice(w io.Writer, val reflect.Value) error {
	if val.Len() == 0 {
		fmt.Fprintln(w, "No items found")
		return nil
	}

	// Get first element to determine columns
	first := val.Index(0).Interface()
	firstVal := reflect.ValueOf(first)

	// Handle maps (like API responses with "items" key)
	if firstVal.Kind() == reflect.Map {
		if items, ok := first.(map[string]interface{})["items"]; ok {
			if itemsSlice, ok := items.([]interface{}); ok {
				if len(itemsSlice) == 0 {
					fmt.Fprintln(w, "No items found")
					return nil
				}
				val = reflect.ValueOf(itemsSlice)
				first = val.Index(0).Interface()
				firstVal = reflect.ValueOf(first)
			}
		}
	}

	// Get filtered headers
	headers := f.getHeaders(firstVal)
	if len(headers) == 0 {
		fmt.Fprintln(w, "No items found")
		return nil
	}

	// Convert headers to display labels
	displayHeaders := make([]string, len(headers))
	for i, h := range headers {
		if label, ok := f.FieldLabels[h]; ok {
			displayHeaders[i] = label
		} else {
			// Use last part of nested path for display (e.g., "spec.image" -> "IMAGE")
			parts := strings.Split(h, ".")
			displayHeaders[i] = strings.ToUpper(parts[len(parts)-1])
		}
	}

	// Create table
	table := tablewriter.NewWriter(w)
	table.SetHeader(displayHeaders)
	table.SetAutoWrapText(false)
	table.SetAutoFormatHeaders(true)
	table.SetHeaderAlignment(tablewriter.ALIGN_LEFT)
	table.SetAlignment(tablewriter.ALIGN_LEFT)
	table.SetCenterSeparator("")
	table.SetColumnSeparator("")
	table.SetRowSeparator("")
	table.SetHeaderLine(false)
	table.SetBorder(false)
	table.SetTablePadding("\t") // Use tabs to separate columns
	table.SetNoWhiteSpace(true)

	// Add rows
	for i := 0; i < val.Len(); i++ {
		item := val.Index(i).Interface()
		row := f.getRow(reflect.ValueOf(item), firstVal, headers)
		table.Append(row)
	}

	table.Render()
	return nil
}

func (f *TableFormatter) getHeaders(val reflect.Value) []string {
	headers := f.Fields
	if len(headers) == 0 {
		// Default: return all top-level fields
		switch val.Kind() {
		case reflect.Struct:
			t := val.Type()
			headers = make([]string, 0, val.NumField())
			for i := 0; i < val.NumField(); i++ {
				field := t.Field(i)
				if !field.IsExported() {
					continue
				}
				tag := jsonTag(field)
				if tag != "" && tag != "-" {
					headers = append(headers, tag)
				}
			}
		case reflect.Map:
			keys := val.MapKeys()
			headers = make([]string, len(keys))
			for i, k := range keys {
				headers[i] = k.String()
			}
		default:
			headers = []string{"value"}
		}
	}
	return headers
}

func (f *TableFormatter) getRow(val reflect.Value, firstVal reflect.Value, headers []string) []string {
	row := make([]string, len(headers))

	for i, h := range headers {
		// Try to get value using nested path support
		v := getFieldValue(val, h)
		row[i] = f.formatValue(v)
	}

	return row
}

func (f *TableFormatter) formatValue(v interface{}) string {
	if v == nil || (reflect.ValueOf(v).Kind() == reflect.Ptr && reflect.ValueOf(v).IsNil()) {
		return ""
	}

	// Format time values
	if t, ok := v.(time.Time); ok {
		// If time is zero, return empty
		if t.IsZero() {
			return ""
		}
		// Show relative time for recent items, absolute for older ones
		if time.Since(t) < 24*time.Hour {
			return formatDuration(time.Since(t))
		}
		return t.Format("2006-01-02 15:04")
	}

	return fmt.Sprintf("%v", v)
}

func formatDuration(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%ds ago", int(d.Seconds()))
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	}
	if d < 24*time.Hour {
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	}
	return fmt.Sprintf("%dd ago", int(d.Hours()/24))
}

func jsonTag(field reflect.StructField) string {
	tag := field.Tag.Get("json")
	if tag == "" {
		return ""
	}
	// Parse json tag (e.g., "name,omitempty" -> "name")
	for i, c := range tag {
		if c == ',' || c == ' ' {
			return tag[:i]
		}
	}
	return tag
}

func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}

// getFieldValue recursively gets a field value from a struct using dot notation (e.g., "spec.image")
func getFieldValue(v reflect.Value, fieldPath string) interface{} {
	parts := strings.Split(fieldPath, ".")
	for _, part := range parts {
		// Handle pointer types
		if v.Kind() == reflect.Ptr && !v.IsNil() {
			v = v.Elem()
		}
		if !v.IsValid() {
			return nil
		}
		switch v.Kind() {
		case reflect.Struct:
			field := v.FieldByNameFunc(func(name string) bool {
				// Try both exact name and JSON tag name
				if name == part {
					return true
				}
				f, _ := v.Type().FieldByName(name)
				tag := f.Tag.Get("json")
				if tag != "" {
					tag = strings.Split(tag, ",")[0]
					return tag == part
				}
				return false
			})
			if field.IsValid() {
				v = field
			} else {
				return nil
			}
		case reflect.Map:
			for _, key := range v.MapKeys() {
				if key.String() == part {
					v = v.MapIndex(key)
					break
				}
			}
		default:
			return nil
		}
	}
	if v.IsValid() && (v.Kind() != reflect.Ptr || !v.IsNil()) {
		return v.Interface()
	}
	return nil
}

// WriteError writes an error message
func WriteError(w io.Writer, err error) {
	fmt.Fprintf(w, "Error: %v\n", err)
}

// WriteString writes a string
func WriteString(w io.Writer, s string) {
	fmt.Fprintln(w, s)
}

// GetDefaultWriter returns the default writer for output
func GetDefaultWriter() io.Writer {
	return os.Stdout
}
