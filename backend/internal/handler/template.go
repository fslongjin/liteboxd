package handler

import (
	"net/http"
	"strconv"

	"github.com/fslongjin/liteboxd/internal/model"
	"github.com/fslongjin/liteboxd/internal/service"
	"github.com/gin-gonic/gin"
)

// TemplateHandler handles template-related HTTP requests
type TemplateHandler struct {
	svc *service.TemplateService
}

// NewTemplateHandler creates a new TemplateHandler
func NewTemplateHandler(svc *service.TemplateService) *TemplateHandler {
	return &TemplateHandler{svc: svc}
}

// RegisterRoutes registers template routes
func (h *TemplateHandler) RegisterRoutes(r *gin.RouterGroup) {
	templates := r.Group("/templates")
	{
		templates.POST("", h.Create)
		templates.GET("", h.List)
		templates.GET("/:name", h.Get)
		templates.PUT("/:name", h.Update)
		templates.DELETE("/:name", h.Delete)

		// Version management
		templates.GET("/:name/versions", h.ListVersions)
		templates.GET("/:name/versions/:version", h.GetVersion)
		templates.POST("/:name/rollback", h.Rollback)
	}
}

// Create handles POST /templates
func (h *TemplateHandler) Create(c *gin.Context) {
	var req model.CreateTemplateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": gin.H{
				"code":    "INVALID_REQUEST",
				"message": err.Error(),
			},
		})
		return
	}

	template, err := h.svc.Create(c.Request.Context(), &req)
	if err != nil {
		if isConflictError(err) {
			c.JSON(http.StatusConflict, gin.H{
				"error": gin.H{
					"code":    "TEMPLATE_EXISTS",
					"message": err.Error(),
				},
			})
			return
		}
		c.JSON(http.StatusBadRequest, gin.H{
			"error": gin.H{
				"code":    "INVALID_REQUEST",
				"message": err.Error(),
			},
		})
		return
	}

	c.JSON(http.StatusCreated, template)
}

// List handles GET /templates
func (h *TemplateHandler) List(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("pageSize", "20"))

	opts := model.TemplateListOptions{
		Tag:      c.Query("tag"),
		Search:   c.Query("search"),
		Page:     page,
		PageSize: pageSize,
	}

	result, err := h.svc.List(c.Request.Context(), opts)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": gin.H{
				"code":    "INTERNAL_ERROR",
				"message": err.Error(),
			},
		})
		return
	}

	c.JSON(http.StatusOK, result)
}

// Get handles GET /templates/:name
func (h *TemplateHandler) Get(c *gin.Context) {
	name := c.Param("name")

	template, err := h.svc.Get(c.Request.Context(), name)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": gin.H{
				"code":    "INTERNAL_ERROR",
				"message": err.Error(),
			},
		})
		return
	}

	if template == nil {
		c.JSON(http.StatusNotFound, gin.H{
			"error": gin.H{
				"code":    "TEMPLATE_NOT_FOUND",
				"message": "Template '" + name + "' not found",
			},
		})
		return
	}

	c.JSON(http.StatusOK, template)
}

// Update handles PUT /templates/:name
func (h *TemplateHandler) Update(c *gin.Context) {
	name := c.Param("name")

	var req model.UpdateTemplateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": gin.H{
				"code":    "INVALID_REQUEST",
				"message": err.Error(),
			},
		})
		return
	}

	template, err := h.svc.Update(c.Request.Context(), name, &req)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": gin.H{
				"code":    "INVALID_REQUEST",
				"message": err.Error(),
			},
		})
		return
	}

	if template == nil {
		c.JSON(http.StatusNotFound, gin.H{
			"error": gin.H{
				"code":    "TEMPLATE_NOT_FOUND",
				"message": "Template '" + name + "' not found",
			},
		})
		return
	}

	c.JSON(http.StatusOK, template)
}

// Delete handles DELETE /templates/:name
func (h *TemplateHandler) Delete(c *gin.Context) {
	name := c.Param("name")

	err := h.svc.Delete(c.Request.Context(), name)
	if err != nil {
		if isNotFoundError(err) {
			c.JSON(http.StatusNotFound, gin.H{
				"error": gin.H{
					"code":    "TEMPLATE_NOT_FOUND",
					"message": "Template '" + name + "' not found",
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

	c.Status(http.StatusNoContent)
}

// ListVersions handles GET /templates/:name/versions
func (h *TemplateHandler) ListVersions(c *gin.Context) {
	name := c.Param("name")

	result, err := h.svc.ListVersions(c.Request.Context(), name)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": gin.H{
				"code":    "INTERNAL_ERROR",
				"message": err.Error(),
			},
		})
		return
	}

	if result == nil {
		c.JSON(http.StatusNotFound, gin.H{
			"error": gin.H{
				"code":    "TEMPLATE_NOT_FOUND",
				"message": "Template '" + name + "' not found",
			},
		})
		return
	}

	c.JSON(http.StatusOK, result)
}

// GetVersion handles GET /templates/:name/versions/:version
func (h *TemplateHandler) GetVersion(c *gin.Context) {
	name := c.Param("name")
	versionStr := c.Param("version")

	version, err := strconv.Atoi(versionStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": gin.H{
				"code":    "INVALID_REQUEST",
				"message": "Invalid version number",
			},
		})
		return
	}

	ver, err := h.svc.GetVersion(c.Request.Context(), name, version)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": gin.H{
				"code":    "INTERNAL_ERROR",
				"message": err.Error(),
			},
		})
		return
	}

	if ver == nil {
		c.JSON(http.StatusNotFound, gin.H{
			"error": gin.H{
				"code":    "VERSION_NOT_FOUND",
				"message": "Version " + versionStr + " not found for template '" + name + "'",
			},
		})
		return
	}

	c.JSON(http.StatusOK, ver)
}

// Rollback handles POST /templates/:name/rollback
func (h *TemplateHandler) Rollback(c *gin.Context) {
	name := c.Param("name")

	var req model.RollbackRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": gin.H{
				"code":    "INVALID_REQUEST",
				"message": err.Error(),
			},
		})
		return
	}

	result, err := h.svc.Rollback(c.Request.Context(), name, &req)
	if err != nil {
		if isNotFoundError(err) {
			c.JSON(http.StatusNotFound, gin.H{
				"error": gin.H{
					"code":    "TEMPLATE_NOT_FOUND",
					"message": err.Error(),
				},
			})
			return
		}
		c.JSON(http.StatusBadRequest, gin.H{
			"error": gin.H{
				"code":    "INVALID_REQUEST",
				"message": err.Error(),
			},
		})
		return
	}

	c.JSON(http.StatusOK, result)
}

// Helper functions

func isConflictError(err error) bool {
	return err != nil && (contains(err.Error(), "already exists") || contains(err.Error(), "UNIQUE constraint"))
}

func isNotFoundError(err error) bool {
	return err != nil && contains(err.Error(), "not found")
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsSubstring(s, substr))
}

func containsSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
