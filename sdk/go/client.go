package liteboxd

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"strings"
	"time"
)

const (
	defaultBaseURL = "http://localhost:8080/api/v1"
	defaultTimeout = 30 * time.Second
	userAgent      = "liteboxd-go-sdk/1.0.0"
)

// Client is the main API client for LiteBoxd.
type Client struct {
	baseURL    *url.URL
	httpClient *http.Client
	authToken  string

	// Service clients
	Sandbox      *SandboxService
	Template     *TemplateService
	Prepull      *PrepullService
	ImportExport *ImportExportService
}

// NewClient creates a new LiteBoxd API client.
func NewClient(baseURL string, opts ...Option) *Client {
	// Parse base URL
	parsedURL, err := url.Parse(baseURL)
	if err != nil {
		parsedURL, _ = url.Parse(defaultBaseURL)
	}

	// Ensure base URL ends without trailing slash for path joining
	parsedURL.Path = strings.TrimSuffix(parsedURL.Path, "/")

	c := &Client{
		baseURL:    parsedURL,
		httpClient: &http.Client{Timeout: defaultTimeout},
	}

	// Apply options
	for _, opt := range opts {
		opt(c)
	}

	// Initialize service clients
	c.Sandbox = &SandboxService{client: c}
	c.Template = &TemplateService{client: c}
	c.Prepull = &PrepullService{client: c}
	c.ImportExport = &ImportExportService{client: c}

	return c
}

// doRequest performs an HTTP request with the given context, method, path, body, and query parameters.
func (c *Client) doRequest(ctx context.Context, method, requestPath string, body interface{}, queryParams map[string]string) (*http.Response, error) {
	// Build URL by appending path to base URL
	u := *c.baseURL
	u.Path = c.baseURL.Path + "/" + requestPath
	u.RawQuery = ""

	// Add query parameters
	if len(queryParams) > 0 {
		q := u.Query()
		for k, v := range queryParams {
			q.Set(k, v)
		}
		u.RawQuery = q.Encode()
	}

	// Encode body
	var bodyReader io.Reader
	if body != nil {
		jsonData, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal request body: %w", err)
		}
		bodyReader = bytes.NewReader(jsonData)
	}

	// Create request
	req, err := http.NewRequestWithContext(ctx, method, u.String(), bodyReader)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Set headers
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", userAgent)

	if c.authToken != "" {
		req.Header.Set("Authorization", "Bearer "+c.authToken)
	}

	// Execute request
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to execute request: %w", err)
	}

	return resp, nil
}

// doJSON performs a request and decodes the JSON response into the result.
func (c *Client) doJSON(ctx context.Context, method, requestPath string, body, result interface{}, queryParams map[string]string) error {
	resp, err := c.doRequest(ctx, method, requestPath, body, queryParams)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	// Check for error status
	if resp.StatusCode >= 400 {
		return handleErrorResponse(resp)
	}

	// Decode response
	if result != nil {
		if err := json.NewDecoder(resp.Body).Decode(result); err != nil {
			return fmt.Errorf("failed to decode response: %w", err)
		}
	}

	return nil
}

// doEmptyResponse performs a request and expects an empty response (for 204 No Content, etc.)
func (c *Client) doEmptyResponse(ctx context.Context, method, requestPath string, body interface{}, queryParams map[string]string) error {
	resp, err := c.doRequest(ctx, method, requestPath, body, queryParams)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	// Check for error status
	if resp.StatusCode >= 400 {
		return handleErrorResponse(resp)
	}

	return nil
}

// doRequestWithReader performs an HTTP request with a custom body reader.
func (c *Client) doRequestWithReader(ctx context.Context, method, requestPath string, bodyReader io.Reader, contentType string, queryParams map[string]string) (*http.Response, error) {
	// Build URL by appending path to base URL
	u := *c.baseURL
	u.Path = c.baseURL.Path + "/" + requestPath
	u.RawQuery = ""

	// Add query parameters
	if len(queryParams) > 0 {
		q := u.Query()
		for k, v := range queryParams {
			q.Set(k, v)
		}
		u.RawQuery = q.Encode()
	}

	// Create request
	req, err := http.NewRequestWithContext(ctx, method, u.String(), bodyReader)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Set headers
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", userAgent)

	if c.authToken != "" {
		req.Header.Set("Authorization", "Bearer "+c.authToken)
	}

	// Execute request
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to execute request: %w", err)
	}

	return resp, nil
}

// doText performs a request and returns the response body as text.
func (c *Client) doText(ctx context.Context, method, requestPath string, queryParams map[string]string) ([]byte, error) {
	resp, err := c.doRequest(ctx, method, requestPath, nil, queryParams)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	// Check for error status
	if resp.StatusCode >= 400 {
		return nil, handleErrorResponse(resp)
	}

	return io.ReadAll(resp.Body)
}

// buildPath builds an API path from segments.
func (c *Client) buildPath(segments ...string) string {
	return path.Join(segments...)
}
