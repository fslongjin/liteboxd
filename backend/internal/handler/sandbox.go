package handler

import (
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/fslongjin/liteboxd/backend/internal/lifecycle"
	"github.com/fslongjin/liteboxd/backend/internal/model"
	"github.com/fslongjin/liteboxd/backend/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

type SandboxHandler struct {
	svc          *service.SandboxService
	reconcileSvc *service.SandboxReconcileService
	drainState   *lifecycle.DrainManager
}

func NewSandboxHandler(svc *service.SandboxService, reconcileSvc *service.SandboxReconcileService, drainState *lifecycle.DrainManager) *SandboxHandler {
	return &SandboxHandler{svc: svc, reconcileSvc: reconcileSvc, drainState: drainState}
}

func (h *SandboxHandler) RegisterRoutes(r *gin.RouterGroup) {
	sandboxes := r.Group("/sandboxes")
	{
		sandboxes.POST("", h.Create)
		sandboxes.GET("", h.List)
		sandboxes.GET("/metadata", h.ListMetadata)
		sandboxes.POST("/reconcile", h.TriggerReconcile)
		sandboxes.GET("/reconcile/runs", h.ListReconcileRuns)
		sandboxes.GET("/reconcile/runs/:id", h.GetReconcileRun)
		sandboxes.GET("/:id", h.Get)
		sandboxes.GET("/:id/status-history", h.GetStatusHistory)
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

func (h *SandboxHandler) ListMetadata(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))

	parseRFC3339 := func(name string) (*time.Time, error) {
		v := c.Query(name)
		if v == "" {
			return nil, nil
		}
		t, err := time.Parse(time.RFC3339, v)
		if err != nil {
			return nil, err
		}
		return &t, nil
	}

	createdFrom, err := parseRFC3339("created_from")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid created_from, expected RFC3339"})
		return
	}
	createdTo, err := parseRFC3339("created_to")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid created_to, expected RFC3339"})
		return
	}
	deletedFrom, err := parseRFC3339("deleted_from")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid deleted_from, expected RFC3339"})
		return
	}
	deletedTo, err := parseRFC3339("deleted_to")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid deleted_to, expected RFC3339"})
		return
	}

	resp, err := h.svc.ListMetadata(c.Request.Context(), model.SandboxMetadataListOptions{
		ID:              c.Query("id"),
		Template:        c.Query("template"),
		DesiredState:    c.Query("desired_state"),
		LifecycleStatus: c.Query("lifecycle_status"),
		CreatedFrom:     createdFrom,
		CreatedTo:       createdTo,
		DeletedFrom:     deletedFrom,
		DeletedTo:       deletedTo,
		Page:            page,
		PageSize:        pageSize,
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, resp)
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

func (h *SandboxHandler) GetStatusHistory(c *gin.Context) {
	id := c.Param("id")
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))
	beforeID, _ := strconv.ParseInt(c.DefaultQuery("before_id", "0"), 10, 64)

	resp, err := h.svc.GetStatusHistory(c.Request.Context(), id, limit, beforeID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, resp)
}

func (h *SandboxHandler) TriggerReconcile(c *gin.Context) {
	resp, err := h.reconcileSvc.Run(c.Request.Context(), "manual")
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusAccepted, resp)
}

func (h *SandboxHandler) ListReconcileRuns(c *gin.Context) {
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}
	resp, err := h.reconcileSvc.ListRuns(c.Request.Context(), limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, resp)
}

func (h *SandboxHandler) GetReconcileRun(c *gin.Context) {
	id := c.Param("id")
	resp, err := h.reconcileSvc.GetRun(c.Request.Context(), id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if resp == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "reconcile run not found"})
		return
	}
	c.JSON(http.StatusOK, resp)
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
