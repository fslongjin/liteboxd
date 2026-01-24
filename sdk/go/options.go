package liteboxd

import (
	"net/http"
	"time"
)

// Option is a functional option for client configuration.
type Option func(*Client)

// WithHTTPClient sets a custom HTTP client.
func WithHTTPClient(client *http.Client) Option {
	return func(c *Client) {
		c.httpClient = client
	}
}

// WithTimeout sets the default timeout for requests.
func WithTimeout(timeout time.Duration) Option {
	return func(c *Client) {
		c.httpClient.Timeout = timeout
	}
}

// WithAuthToken sets the authentication token for API requests.
func WithAuthToken(token string) Option {
	return func(c *Client) {
		c.authToken = token
	}
}

// WithUserAgent sets a custom User-Agent header.
func WithUserAgent(ua string) Option {
	return func(c *Client) {
		// This would need to be stored as a field on Client
		// For now, we can add it to the Client struct if needed
	}
}
