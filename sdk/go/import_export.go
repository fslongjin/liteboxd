package liteboxd

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"mime/multipart"
	"strconv"
)

// ImportExportService handles template import/export operations.
type ImportExportService struct {
	client *Client
}

// ImportTemplates imports templates from YAML.
// strategy: "create-only", "update-only", or "create-or-update"
// autoPrepull: whether to trigger prepull after import
func (ie *ImportExportService) ImportTemplates(ctx context.Context, yamlContent []byte, strategy ImportStrategy, autoPrepull bool) (*ImportTemplatesResponse, error) {
	// Create multipart form body
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)

	// Add file field
	part, err := writer.CreateFormFile("file", "templates.yaml")
	if err != nil {
		return nil, fmt.Errorf("failed to create form file: %w", err)
	}
	if _, err := part.Write(yamlContent); err != nil {
		return nil, fmt.Errorf("failed to write file content: %w", err)
	}

	// Add strategy field
	if err := writer.WriteField("strategy", string(strategy)); err != nil {
		return nil, fmt.Errorf("failed to write strategy field: %w", err)
	}

	// Add prepull field
	if autoPrepull {
		if err := writer.WriteField("prepull", strconv.FormatBool(autoPrepull)); err != nil {
			return nil, fmt.Errorf("failed to write prepull field: %w", err)
		}
	}

	// Close writer to finalize the form
	if err := writer.Close(); err != nil {
		return nil, fmt.Errorf("failed to close multipart writer: %w", err)
	}

	// Make request
	resp, err := ie.client.doRequestWithReader(ctx, "POST", ie.client.buildPath("templates", "import"), &body, writer.FormDataContentType(), nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	// Check for error status
	if resp.StatusCode >= 400 {
		return nil, handleErrorResponse(resp)
	}

	// Decode response
	var result ImportTemplatesResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &result, nil
}

// ExportAllTemplates exports all templates to YAML.
func (ie *ImportExportService) ExportAllTemplates(ctx context.Context, tag, names string) ([]byte, error) {
	queryParams := make(map[string]string)
	if tag != "" {
		queryParams["tag"] = tag
	}
	if names != "" {
		queryParams["names"] = names
	}
	return ie.client.doText(ctx, "GET", ie.client.buildPath("templates", "export"), queryParams)
}
