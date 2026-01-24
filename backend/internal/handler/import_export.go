package handler

import (
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/fslongjin/liteboxd/backend/internal/model"
	"github.com/fslongjin/liteboxd/backend/internal/service"
	"github.com/gin-gonic/gin"
)

// ImportExportHandler handles template import/export HTTP requests
type ImportExportHandler struct {
	importSvc *service.ImportExportService
}

// NewImportExportHandler creates a new ImportExportHandler
func NewImportExportHandler(importSvc *service.ImportExportService) *ImportExportHandler {
	return &ImportExportHandler{importSvc: importSvc}
}

// RegisterRoutes registers import/export routes
func (h *ImportExportHandler) RegisterRoutes(r *gin.RouterGroup) {
	templates := r.Group("/templates")
	{
		templates.POST("/import", h.Import)
		templates.GET("/export", h.ExportAll)
		templates.GET("/:name/export", h.ExportOne)
	}
}

// Import handles POST /templates/import
func (h *ImportExportHandler) Import(c *gin.Context) {
	// Parse form data
	var req model.ImportTemplatesRequest
	if err := c.ShouldBind(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": gin.H{
				"code":    "INVALID_REQUEST",
				"message": err.Error(),
			},
		})
		return
	}

	// Set default strategy
	if req.Strategy == "" {
		req.Strategy = model.ImportStrategyCreateOrUpdate
	}

	// Get file from form
	file, err := c.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": gin.H{
				"code":    "INVALID_REQUEST",
				"message": "file is required",
			},
		})
		return
	}

	// Read file content
	src, err := file.Open()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": gin.H{
				"code":    "INTERNAL_ERROR",
				"message": "failed to open file",
			},
		})
		return
	}
	defer src.Close()

	content, err := io.ReadAll(src)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": gin.H{
				"code":    "INTERNAL_ERROR",
				"message": "failed to read file",
			},
		})
		return
	}

	// Import templates
	response, err := h.importSvc.ImportFromYAML(c.Request.Context(), content, req.Strategy, req.Prepull)
	if err != nil {
		if strings.Contains(err.Error(), "invalid YAML") {
			c.JSON(http.StatusBadRequest, gin.H{
				"error": gin.H{
					"code":    "INVALID_YAML",
					"message": err.Error(),
				},
			})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": gin.H{
				"code":    "INTERNAL_ERROR",
				"message": err.Error(),
			},
		})
		return
	}

	c.JSON(http.StatusOK, response)
}

// ExportAll handles GET /templates/export
func (h *ImportExportHandler) ExportAll(c *gin.Context) {
	tag := c.Query("tag")
	namesStr := c.Query("names")

	var names []string
	if namesStr != "" {
		names = strings.Split(namesStr, ",")
	}

	yamlContent, err := h.importSvc.ExportAllToYAML(c.Request.Context(), tag, names)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": gin.H{
				"code":    "INTERNAL_ERROR",
				"message": err.Error(),
			},
		})
		return
	}

	c.Header("Content-Type", "application/x-yaml")
	c.String(http.StatusOK, string(yamlContent))
}

// ExportOne handles GET /templates/:name/export
func (h *ImportExportHandler) ExportOne(c *gin.Context) {
	name := c.Param("name")
	version := 0
	if v := c.Query("version"); v != "" {
		_, err := scanIntParam(v, &version)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"error": gin.H{
					"code":    "INVALID_REQUEST",
					"message": "invalid version number",
				},
			})
			return
		}
	}

	yamlContent, err := h.importSvc.ExportToYAML(c.Request.Context(), name, version)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			c.JSON(http.StatusNotFound, gin.H{
				"error": gin.H{
					"code":    "TEMPLATE_NOT_FOUND",
					"message": err.Error(),
				},
			})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": gin.H{
				"code":    "INTERNAL_ERROR",
				"message": err.Error(),
			},
		})
		return
	}

	c.Header("Content-Type", "application/x-yaml")
	c.String(http.StatusOK, string(yamlContent))
}

// Helper function for scanning int parameter
func scanIntParam(s string, dest *int) (int, error) {
	n, err := scanInt(s, dest)
	return n, err
}

func scanInt(s string, dest *int) (int, error) {
	if dest != nil {
		*dest = 0
	}
	var n int
	_, err := fmt.Sscanf(s, "%d", &n)
	if err != nil {
		return n, err
	}
	if dest != nil {
		*dest = n
	}
	return n, nil
}
