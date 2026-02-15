package gateway

import (
	"fmt"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"k8s.io/client-go/rest"
)

// ProxyHandler handles proxying requests to sandbox pods
func (s *Service) ProxyHandler(c *gin.Context) {
	sandboxID := c.Param(sandboxIDParam)
	port := c.GetString("port")

	var targetURL *url.URL
	var proxy *httputil.ReverseProxy

	if s.config.UseK8sProxy {
		// Use K8s API Server proxy
		// URL format: /api/v1/namespaces/{namespace}/pods/{name}:{port}/proxy/{path}
		podName := fmt.Sprintf("sandbox-%s", sandboxID)

		// Get K8s REST config
		k8sConfig := s.k8sClient.GetConfig()
		host := k8sConfig.Host

		targetURL, _ = url.Parse(host)

		proxy = httputil.NewSingleHostReverseProxy(targetURL)

		originalDirector := proxy.Director
		proxy.Director = func(req *http.Request) {
			originalDirector(req)

			// Set K8s Authentication
			if k8sConfig.BearerToken != "" {
				req.Header.Set("Authorization", "Bearer "+k8sConfig.BearerToken)
			}
			// Note: If using client certs (e.g. minikube), we need to handle that in Transport,
			// but here we focus on Token auth which is common for remote clusters (ServiceAccount or Token).
			// For full support we might need to copy Transport from k8s client, but simpler approach first.

			// Construct proxy path
			// Target: /api/v1/namespaces/liteboxd/pods/sandbox-{id}:{port}/proxy/{path}

			// Extract path after /port/{port}
			realPath := ""
			parts := strings.Split(req.URL.Path, "/")
			portIndex := -1
			for i, p := range parts {
				if p == "port" && i+1 < len(parts) {
					portIndex = i + 2
					break
				}
			}
			if portIndex > 0 && portIndex < len(parts) {
				realPath = "/" + strings.Join(parts[portIndex:], "/")
			}
			if realPath == "" {
				realPath = "/"
			}

			// K8s API Proxy path
			k8sProxyPath := fmt.Sprintf("/api/v1/namespaces/%s/pods/%s:%s/proxy%s",
				s.k8sClient.SandboxNamespace(), podName, port, realPath)

			req.URL.Path = k8sProxyPath

			// Strip prefix logic is handled by constructing the new path directly above

			// Update host to K8s API host
			req.Host = targetURL.Host

			// Preserve X-Access-Token header when forwarding (K8s proxy forwards headers)
			// But wait, K8s proxy might conflict if we send Authorization header for K8s AND X-Access-Token for App?
			// X-Access-Token is custom, so it should pass through.
		}

		// 使用 client-go 的 TransportFor，与 k8s 客户端共用同一套 TLS 配置（CA/证书/Insecure），
		// 避免自建 tls.Config 时未加载 kubeconfig 中的 CA 导致 "certificate signed by unknown authority"。
		transport, err := rest.TransportFor(k8sConfig)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{
				"error": "failed to build k8s transport: " + err.Error(),
			})
			return
		}
		proxy.Transport = transport

	} else {
		// Direct Pod IP mode (Original logic)

		// Get pod IP
		podIP, err := s.k8sClient.GetPodIP(c.Request.Context(), sandboxID)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusNotFound, gin.H{
				"error": "sandbox not found",
			})
			return
		}

		// Build target URL
		targetURL = &url.URL{
			Scheme: "http",
			Host:   fmt.Sprintf("%s:%s", podIP, port),
		}

		// Create reverse proxy
		proxy = httputil.NewSingleHostReverseProxy(targetURL)

		// Modify the request to strip the gateway prefix
		director := proxy.Director
		proxy.Director = func(req *http.Request) {
			director(req)

			// Update the request path to remove the gateway prefix
			// Original path: /api/v1/sandbox/{id}/port/{port}/...
			// Target path: /...
			parts := strings.Split(req.URL.Path, "/")
			if len(parts) >= 6 {
				// parts[0]="", parts[1]="api", parts[2]="v1", parts[3]="sandbox", parts[4]={id}, parts[5]="port", parts[6]={port}, parts[7+]=...
				// Find the index after "port"
				portIndex := -1
				for i, p := range parts {
					if p == "port" && i+1 < len(parts) {
						portIndex = i + 2
						break
					}
				}
				if portIndex > 0 && portIndex < len(parts) {
					req.URL.Path = "/" + strings.Join(parts[portIndex:], "/")
				}
			}

			// Update host to target
			req.Host = targetURL.Host

			// Preserve X-Access-Token header when forwarding
			req.Header.Del("X-Access-Token")
		}

		// Set timeout
		proxy.Transport = &http.Transport{
			Proxy:                 http.ProxyFromEnvironment,
			DialContext:           (&http.Transport{}).DialContext,
			MaxIdleConns:          100,
			IdleConnTimeout:       90 * time.Second,
			TLSHandshakeTimeout:   10 * time.Second,
			ExpectContinueTimeout: 1 * time.Second,
			ResponseHeaderTimeout: 10 * time.Second,
		}
	}

	// Handle WebSocket upgrade if needed
	if isWebSocketUpgrade(c.Request) {
		s.handleWebSocketUpgrade(c, targetURL)
		return
	}

	// Serve the proxied request
	proxy.ServeHTTP(c.Writer, c.Request)
}

// handleWebSocketUpgrade handles WebSocket upgrade requests
// Note: Full WebSocket support requires additional libraries like gorilla/websocket
func (s *Service) handleWebSocketUpgrade(c *gin.Context, target *url.URL) {
	if target == nil {
		c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{
			"error": "missing target for websocket proxy",
		})
		return
	}

	sandboxID := c.Param(sandboxIDParam)
	port := c.GetString("port")
	realPath := extractTargetPath(c.Request.URL.Path)
	var backendURL *url.URL
	var header http.Header
	var dialer *websocket.Dialer

	if s.config.UseK8sProxy {
		k8sConfig := s.k8sClient.GetConfig()
		podName := fmt.Sprintf("sandbox-%s", sandboxID)
		k8sProxyPath := fmt.Sprintf("/api/v1/namespaces/%s/pods/%s:%s/proxy%s",
			s.k8sClient.SandboxNamespace(), podName, port, realPath)
		backendURL = &url.URL{
			Scheme:   wsSchemeFromTarget(target),
			Host:     target.Host,
			Path:     k8sProxyPath,
			RawQuery: c.Request.URL.RawQuery,
		}
		header = cloneWebSocketHeaders(c.Request.Header, false)
		if k8sConfig.BearerToken != "" {
			header.Set("Authorization", "Bearer "+k8sConfig.BearerToken)
		}
		roundTripper, err := rest.TransportFor(k8sConfig)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{
				"error": "failed to build k8s transport: " + err.Error(),
			})
			return
		}
		transport, ok := roundTripper.(*http.Transport)
		if !ok {
			c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{
				"error": "failed to build k8s transport: unexpected transport type",
			})
			return
		}
		dialer = &websocket.Dialer{
			Proxy:            http.ProxyFromEnvironment,
			HandshakeTimeout: 45 * time.Second,
			TLSClientConfig:  transport.TLSClientConfig,
			NetDialContext:   transport.DialContext,
		}
	} else {
		backendURL = &url.URL{
			Scheme:   wsSchemeFromTarget(target),
			Host:     target.Host,
			Path:     realPath,
			RawQuery: c.Request.URL.RawQuery,
		}
		header = cloneWebSocketHeaders(c.Request.Header, true)
		dialer = &websocket.Dialer{
			Proxy:            http.ProxyFromEnvironment,
			HandshakeTimeout: 45 * time.Second,
		}
	}

	backendConn, resp, err := dialer.Dial(backendURL.String(), header)
	if err != nil {
		status := http.StatusBadGateway
		if resp != nil {
			status = resp.StatusCode
		}
		c.AbortWithStatusJSON(status, gin.H{
			"error": "failed to connect to backend websocket: " + err.Error(),
		})
		return
	}
	defer backendConn.Close()

	upgrader := websocket.Upgrader{
		CheckOrigin:     func(r *http.Request) bool { return true },
		ReadBufferSize:  1024,
		WriteBufferSize: 1024,
	}
	if protocol := backendConn.Subprotocol(); protocol != "" {
		upgrader.Subprotocols = []string{protocol}
	}

	clientConn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		return
	}
	defer clientConn.Close()

	errChan := make(chan error, 2)
	go func() {
		errChan <- relayWebSocketMessages(clientConn, backendConn)
	}()
	go func() {
		errChan <- relayWebSocketMessages(backendConn, clientConn)
	}()

	select {
	case <-c.Request.Context().Done():
	case <-errChan:
	}
}

// isWebSocketUpgrade checks if the request is a WebSocket upgrade request
func isWebSocketUpgrade(req *http.Request) bool {
	return strings.ToLower(req.Header.Get("Upgrade")) == "websocket" &&
		strings.Contains(strings.ToLower(req.Header.Get("Connection")), "upgrade")
}

func wsSchemeFromTarget(target *url.URL) string {
	if target == nil {
		return "ws"
	}
	if target.Scheme == "https" || target.Scheme == "wss" {
		return "wss"
	}
	return "ws"
}

func extractTargetPath(path string) string {
	parts := strings.Split(path, "/")
	portIndex := -1
	for i, p := range parts {
		if p == "port" && i+1 < len(parts) {
			portIndex = i + 2
			break
		}
	}
	if portIndex > 0 && portIndex < len(parts) {
		return "/" + strings.Join(parts[portIndex:], "/")
	}
	return "/"
}

func cloneWebSocketHeaders(src http.Header, removeAccessToken bool) http.Header {
	dst := http.Header{}
	for key, values := range src {
		lower := strings.ToLower(key)
		if lower == "connection" || lower == "upgrade" || lower == "sec-websocket-key" || lower == "sec-websocket-version" || lower == "sec-websocket-extensions" || lower == "host" {
			continue
		}
		for _, v := range values {
			dst.Add(key, v)
		}
	}
	if removeAccessToken {
		dst.Del(authorizationHeader)
	}
	return dst
}

func relayWebSocketMessages(dst, src *websocket.Conn) error {
	for {
		messageType, message, err := src.ReadMessage()
		if err != nil {
			return err
		}
		if err := dst.WriteMessage(messageType, message); err != nil {
			return err
		}
	}
}
