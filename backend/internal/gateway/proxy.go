package gateway

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/fslongjin/liteboxd/backend/internal/logx"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"k8s.io/client-go/rest"
	k8sws "k8s.io/client-go/transport/websocket"
)

const wsProxyCloseWriteTimeout = 3 * time.Second

var wsProxySessionSeq atomic.Uint64

type safeWSConn struct {
	name string
	conn *websocket.Conn
	mu   sync.Mutex
}

func newSafeWSConn(name string, conn *websocket.Conn) *safeWSConn {
	return &safeWSConn{name: name, conn: conn}
}

func (c *safeWSConn) WriteMessage(messageType int, data []byte) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.conn.WriteMessage(messageType, data)
}

func (c *safeWSConn) WriteControl(messageType int, data []byte, deadline time.Time) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	err := c.conn.WriteControl(messageType, data, deadline)
	if errors.Is(err, websocket.ErrCloseSent) {
		return nil
	}
	return err
}

func (c *safeWSConn) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.conn.Close()
}

type wsRelayResult struct {
	Direction string
	Source    string
	Target    string
	Stage     string
	Err       error
	CloseErr  *websocket.CloseError
}

func (r wsRelayResult) notifyPeer() string {
	switch r.Stage {
	case "read":
		return r.Target
	case "write":
		return r.Source
	default:
		return ""
	}
}

func (r wsRelayResult) failureSide() string {
	switch r.Stage {
	case "read":
		return r.Source
	case "write":
		return r.Target
	default:
		return ""
	}
}

func (r wsRelayResult) logLevel() slog.Level {
	if r.CloseErr != nil && (r.CloseErr.Code == websocket.CloseNormalClosure || r.CloseErr.Code == websocket.CloseGoingAway) {
		return slog.LevelInfo
	}
	if r.Stage == "context" {
		return slog.LevelInfo
	}
	return slog.LevelWarn
}

func newWSProxySessionID() string {
	return fmt.Sprintf("wsproxy-%d", wsProxySessionSeq.Add(1))
}

// ProxyHandler handles proxying requests to sandbox pods
func (s *Service) ProxyHandler(c *gin.Context) {
	logger := logx.LoggerWithRequestID(c.Request.Context()).With("component", "gateway_proxy")

	if s.drainState != nil && s.drainState.IsDraining() {
		logger.Warn("proxy request rejected while draining")
		c.AbortWithStatusJSON(http.StatusServiceUnavailable, gin.H{
			"error": "service is draining",
		})
		return
	}

	sandboxID := c.Param(sandboxIDParam)
	port := c.GetString("port")
	logger = logger.With("sandbox_id", sandboxID, "port", port)

	var targetURL *url.URL
	var proxy *httputil.ReverseProxy

	if s.config.UseK8sProxy {
		logger.Debug("using k8s apiserver proxy mode")
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
			logger.Error("failed to build k8s transport", "error", err)
			c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{
				"error": "failed to build k8s transport: " + err.Error(),
			})
			return
		}
		proxy.Transport = transport

	} else {
		logger.Debug("using direct pod ip proxy mode")
		// Direct Pod IP mode (Original logic)

		// Get pod IP
		podIP, err := s.k8sClient.GetPodIP(c.Request.Context(), sandboxID)
		if err != nil {
			logger.Warn("sandbox not found for proxy", "error", err)
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
		logger.Debug("detected websocket upgrade request")
		s.handleWebSocketUpgrade(c, targetURL)
		return
	}

	logger.Debug("proxying http request", "target", targetURL.String())
	// Serve the proxied request
	proxy.ServeHTTP(c.Writer, c.Request)
}

// handleWebSocketUpgrade handles WebSocket upgrade requests
// Note: Full WebSocket support requires additional libraries like gorilla/websocket
func (s *Service) handleWebSocketUpgrade(c *gin.Context, target *url.URL) {
	sessionID := newWSProxySessionID()
	logger := logx.LoggerWithRequestID(c.Request.Context()).With(
		"component", "gateway_proxy",
		"ws_session_id", sessionID,
		"sandbox_id", c.Param(sandboxIDParam),
		"port", c.GetString("port"),
	)

	if target == nil {
		logger.Error("missing target for websocket proxy")
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
		podName := fmt.Sprintf("sandbox-%s", sandboxID)
		k8sProxyPath := fmt.Sprintf("/api/v1/namespaces/%s/pods/%s:%s/proxy%s",
			s.k8sClient.SandboxNamespace(), podName, port, realPath)
		backendURL = &url.URL{
			Scheme:   target.Scheme,
			Host:     target.Host,
			Path:     k8sProxyPath,
			RawQuery: c.Request.URL.RawQuery,
		}
		header = cloneWebSocketHeaders(c.Request.Header, false)
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

	var (
		backendConn *websocket.Conn
		resp        *http.Response
		err         error
	)
	if s.config.UseK8sProxy {
		backendConn, resp, err = dialK8sBackendWebSocket(c.Request.Context(), s.k8sClient.GetConfig(), backendURL, header)
	} else {
		backendConn, resp, err = dialer.Dial(backendURL.String(), header)
	}
	if err != nil {
		status := http.StatusBadGateway
		if resp != nil {
			status = resp.StatusCode
		}
		logger.Warn("failed to connect backend websocket", "status", status, "error", err)
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
		logger.Warn("failed to upgrade client websocket", "error", err)
		return
	}

	release := func() {}
	if s.drainState != nil {
		release = s.drainState.TrackWebSocket()
	}
	defer release()
	logger.Info("websocket proxy connected",
		"backend_url", backendURL.String(),
		"client_remote_addr", c.Request.RemoteAddr,
		"request_path", c.Request.URL.Path,
		"request_query", c.Request.URL.RawQuery,
		"use_k8s_proxy", s.config.UseK8sProxy,
		"subprotocol", backendConn.Subprotocol(),
	)

	runWebSocketProxySession(c.Request.Context(), logger, sessionID, clientConn, backendConn)
	logger.Info("websocket proxy disconnected")
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

func dialK8sBackendWebSocket(ctx context.Context, config *rest.Config, backendURL *url.URL, header http.Header) (*websocket.Conn, *http.Response, error) {
	if config == nil {
		return nil, nil, errors.New("missing k8s config")
	}
	if backendURL == nil {
		return nil, nil, errors.New("missing websocket backend url")
	}

	requestHeader := cloneHeader(header)
	requestHeader.Del("Authorization")

	roundTripper, connectionHolder, err := k8sws.RoundTripperFor(config)
	if err != nil {
		return nil, nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, backendURL.String(), nil)
	if err != nil {
		return nil, nil, err
	}
	req.Header = requestHeader

	resp, err := roundTripper.RoundTrip(req)
	if err != nil {
		return nil, resp, err
	}
	if resp != nil && resp.Body != nil {
		if closeErr := resp.Body.Close(); closeErr != nil {
			if conn := connectionHolder.Connection(); conn != nil {
				_ = conn.Close()
			}
			return nil, resp, fmt.Errorf("close websocket upgrade response body: %w", closeErr)
		}
	}
	conn := connectionHolder.Connection()
	if conn == nil {
		return nil, resp, errors.New("k8s websocket dial did not return a connection")
	}
	return conn, resp, nil
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

func cloneHeader(src http.Header) http.Header {
	if src == nil {
		return http.Header{}
	}
	dst := make(http.Header, len(src))
	for key, values := range src {
		dst[key] = append([]string(nil), values...)
	}
	return dst
}

func runWebSocketProxySession(ctx context.Context, logger *slog.Logger, sessionID string, clientConn, backendConn *websocket.Conn) {
	client := newSafeWSConn("client", clientConn)
	backend := newSafeWSConn("backend", backendConn)
	defer func() {
		_ = client.Close()
		_ = backend.Close()
	}()

	errCh := make(chan wsRelayResult, 2)
	go func() {
		errCh <- relayWebSocketMessages(backend, clientConn, "client_to_backend")
	}()
	go func() {
		errCh <- relayWebSocketMessages(client, backendConn, "backend_to_client")
	}()

	var first wsRelayResult
	select {
	case <-ctx.Done():
		first = wsRelayResult{
			Direction: "proxy_context",
			Source:    "context",
			Target:    "both",
			Stage:     "context",
			Err:       ctx.Err(),
		}
	case first = <-errCh:
	}

	logRelayResult(logger, sessionID, "websocket relay finished", first)
	handleRelayTermination(logger, sessionID, client, backend, first)

	// Give the opposite relay goroutine a brief chance to observe the propagated close,
	// which improves close-frame delivery without stalling shutdown for long.
	select {
	case second := <-errCh:
		logRelayResult(logger, sessionID, "websocket relay peer finished", second)
	case <-time.After(100 * time.Millisecond):
	}
}

func relayWebSocketMessages(dst *safeWSConn, src *websocket.Conn, direction string) wsRelayResult {
	srcName := "backend"
	if direction == "client_to_backend" {
		srcName = "client"
	}

	for {
		messageType, message, err := src.ReadMessage()
		if err != nil {
			return newWSRelayResult(direction, srcName, dst.name, "read", err)
		}
		if err := dst.WriteMessage(messageType, message); err != nil {
			return newWSRelayResult(direction, srcName, dst.name, "write", err)
		}
	}
}

func newWSRelayResult(direction, source, target, stage string, err error) wsRelayResult {
	result := wsRelayResult{
		Direction: direction,
		Source:    source,
		Target:    target,
		Stage:     stage,
		Err:       err,
	}
	var closeErr *websocket.CloseError
	if errors.As(err, &closeErr) {
		result.CloseErr = closeErr
	}
	return result
}

func logRelayResult(logger *slog.Logger, sessionID, message string, result wsRelayResult) {
	if logger == nil {
		return
	}
	attrs := []any{
		"ws_session_id", sessionID,
		"direction", result.Direction,
		"source", result.Source,
		"target", result.Target,
		"stage", result.Stage,
	}
	if result.CloseErr != nil {
		attrs = append(attrs, "close_code", result.CloseErr.Code, "close_text", result.CloseErr.Text)
	}
	if result.Err != nil {
		attrs = append(attrs, "error", result.Err)
	}
	logger.Log(context.Background(), result.logLevel(), message, attrs...)
}

func handleRelayTermination(logger *slog.Logger, sessionID string, client, backend *safeWSConn, result wsRelayResult) {
	switch result.Stage {
	case "context":
		propagateProxyClose(logger, sessionID, client, websocket.CloseGoingAway, "proxy_context_canceled", "context_cancel")
		propagateProxyClose(logger, sessionID, backend, websocket.CloseGoingAway, "proxy_context_canceled", "context_cancel")
		return
	}

	peer := peerConn(client, backend, result.notifyPeer())
	if peer == nil {
		if logger != nil {
			logger.Warn("websocket relay peer resolution failed",
				"ws_session_id", sessionID,
				"source", result.Source,
				"target", result.Target,
				"stage", result.Stage,
			)
		}
		return
	}

	if result.CloseErr != nil {
		code := sanitizeCloseCode(result.CloseErr.Code, websocket.CloseNormalClosure)
		propagateProxyClose(logger, sessionID, peer, code, result.CloseErr.Text, "propagate_peer_close")
		return
	}

	code, text := proxyCloseForUnexpected(result)
	propagateProxyClose(logger, sessionID, peer, code, text, "synthetic_close")
}

func peerConn(client, backend *safeWSConn, name string) *safeWSConn {
	switch name {
	case "client":
		return client
	case "backend":
		return backend
	default:
		return nil
	}
}

func sanitizeCloseCode(code, fallback int) int {
	switch code {
	case 0, websocket.CloseNoStatusReceived, websocket.CloseAbnormalClosure, websocket.CloseTLSHandshake:
		return fallback
	default:
		return code
	}
}

func proxyCloseForUnexpected(result wsRelayResult) (int, string) {
	switch result.failureSide() {
	case "backend":
		if result.Stage == "read" {
			return websocket.CloseInternalServerErr, "upstream_read_failed"
		}
		return websocket.CloseInternalServerErr, "upstream_write_failed"
	case "client":
		if result.Stage == "read" {
			return websocket.CloseGoingAway, "client_read_failed"
		}
		return websocket.CloseGoingAway, "client_write_failed"
	default:
		return websocket.CloseInternalServerErr, "proxy_relay_failed"
	}
}

func writeProxyClose(conn *safeWSConn, code int, text string) error {
	if conn == nil {
		return nil
	}
	deadline := time.Now().Add(wsProxyCloseWriteTimeout)
	return conn.WriteControl(websocket.CloseMessage, websocket.FormatCloseMessage(code, text), deadline)
}

func propagateProxyClose(logger *slog.Logger, sessionID string, conn *safeWSConn, code int, text, mode string) {
	if conn == nil {
		return
	}
	if logger != nil {
		logger.Info("websocket proxy sending close",
			"ws_session_id", sessionID,
			"mode", mode,
			"target", conn.name,
			"close_code", code,
			"close_text", text,
		)
	}
	if err := writeProxyClose(conn, code, text); err != nil {
		if logger != nil {
			logger.Warn("websocket proxy close send failed",
				"ws_session_id", sessionID,
				"mode", mode,
				"target", conn.name,
				"close_code", code,
				"close_text", text,
				"error", err,
			)
		}
		return
	}
	if logger != nil {
		logger.Info("websocket proxy close sent",
			"ws_session_id", sessionID,
			"mode", mode,
			"target", conn.name,
			"close_code", code,
			"close_text", text,
		)
	}
}
