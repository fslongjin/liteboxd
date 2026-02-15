package handler

import (
	"io"
	"net/http"
	"strconv"

	"github.com/fslongjin/liteboxd/backend/internal/lifecycle"
	"github.com/fslongjin/liteboxd/backend/internal/model"
	"github.com/fslongjin/liteboxd/backend/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

type SandboxHandler struct {
	svc        *service.SandboxService
	drainState *lifecycle.DrainManager
}

func NewSandboxHandler(svc *service.SandboxService, drainState *lifecycle.DrainManager) *SandboxHandler {
	return &SandboxHandler{svc: svc, drainState: drainState}
}

func (h *SandboxHandler) RegisterRoutes(r *gin.RouterGroup) {
	sandboxes := r.Group("/sandboxes")
	{
		sandboxes.POST("", h.Create)
		sandboxes.GET("", h.List)
		sandboxes.GET("/:id", h.Get)
		sandboxes.DELETE("/:id", h.Delete)
		sandboxes.POST("/:id/exec", h.Exec)
		sandboxes.GET("/:id/exec/interactive", h.ExecInteractive)
		sandboxes.GET("/:id/logs", h.GetLogs)
		sandboxes.POST("/:id/files", h.UploadFile)
		sandboxes.GET("/:id/files", h.DownloadFile)
	}
}

func (h *SandboxHandler) Create(c *gin.Context) {
	var req model.CreateSandboxRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	sandbox, err := h.svc.Create(c.Request.Context(), &req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, sandbox)
}

func (h *SandboxHandler) List(c *gin.Context) {
	resp, err := h.svc.List(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, resp)
}

func (h *SandboxHandler) Get(c *gin.Context) {
	id := c.Param("id")

	sandbox, err := h.svc.Get(c.Request.Context(), id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, sandbox)
}

func (h *SandboxHandler) Delete(c *gin.Context) {
	id := c.Param("id")

	err := h.svc.Delete(c.Request.Context(), id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusNoContent, nil)
}

func (h *SandboxHandler) Exec(c *gin.Context) {
	id := c.Param("id")

	var req model.ExecRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	resp, err := h.svc.Exec(c.Request.Context(), id, &req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, resp)
}

func (h *SandboxHandler) UploadFile(c *gin.Context) {
	id := c.Param("id")
	path := c.PostForm("path")

	if path == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "path is required"})
		return
	}

	file, _, err := c.Request.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "file is required"})
		return
	}
	defer file.Close()

	content, err := io.ReadAll(file)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to read file"})
		return
	}

	err = h.svc.UploadFile(c.Request.Context(), id, path, content)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "file uploaded successfully"})
}

func (h *SandboxHandler) DownloadFile(c *gin.Context) {
	id := c.Param("id")
	path := c.Query("path")

	if path == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "path is required"})
		return
	}

	content, err := h.svc.DownloadFile(c.Request.Context(), id, path)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.Data(http.StatusOK, "application/octet-stream", content)
}

func (h *SandboxHandler) GetLogs(c *gin.Context) {
	id := c.Param("id")

	resp, err := h.svc.GetLogs(c.Request.Context(), id, 100)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, resp)
}

// WebSocket upgrader for interactive exec
var wsUpgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true // Allow all origins (CORS handled by middleware)
	},
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
}

// ExecInteractive handles WebSocket-based interactive exec sessions.
func (h *SandboxHandler) ExecInteractive(c *gin.Context) {
	if h.drainState != nil && h.drainState.IsDraining() {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "service is draining"})
		return
	}

	id := c.Param("id")

	// Parse query parameters
	command := c.QueryArray("command")
	if len(command) == 0 {
		command = []string{"sh"}
	}
	tty := c.DefaultQuery("tty", "true") == "true"
	rows, _ := strconv.Atoi(c.DefaultQuery("rows", "24"))
	cols, _ := strconv.Atoi(c.DefaultQuery("cols", "80"))

	if rows <= 0 {
		rows = 24
	}
	if cols <= 0 {
		cols = 80
	}

	// Upgrade to WebSocket
	ws, err := wsUpgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to upgrade to websocket: " + err.Error()})
		return
	}
	defer ws.Close()

	release := func() {}
	if h.drainState != nil {
		release = h.drainState.TrackWebSocket()
	}
	defer release()

	// Bridge WebSocket to K8s exec
	h.svc.ExecInteractive(c.Request.Context(), ws, id, command, tty, rows, cols)
}
