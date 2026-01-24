package handler

import (
	"net/http"
	"strings"

	"github.com/fslongjin/liteboxd/internal/model"
	"github.com/fslongjin/liteboxd/internal/service"
	"github.com/gin-gonic/gin"
)

// PrepullHandler handles prepull-related HTTP requests
type PrepullHandler struct {
	prepullSvc  *service.PrepullService
	templateSvc *service.TemplateService
}

// NewPrepullHandler creates a new PrepullHandler
func NewPrepullHandler(prepullSvc *service.PrepullService, templateSvc *service.TemplateService) *PrepullHandler {
	return &PrepullHandler{
		prepullSvc:  prepullSvc,
		templateSvc: templateSvc,
	}
}

// RegisterRoutes registers prepull routes
func (h *PrepullHandler) RegisterRoutes(r *gin.RouterGroup) {
	images := r.Group("/images")
	{
		images.POST("/prepull", h.CreatePrepull)
		images.GET("/prepull", h.ListPrepulls)
		images.DELETE("/prepull/:id", h.DeletePrepull)
	}

	// Template prepull endpoint
	templates := r.Group("/templates")
	{
		templates.POST("/:name/prepull", h.PrepullTemplate)
	}
}

// CreatePrepull handles POST /images/prepull
func (h *PrepullHandler) CreatePrepull(c *gin.Context) {
	var req model.CreatePrepullRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": gin.H{
				"code":    "INVALID_REQUEST",
				"message": err.Error(),
			},
		})
		return
	}

	prepull, err := h.prepullSvc.Create(c.Request.Context(), &req, "")
	if err != nil {
		if strings.Contains(err.Error(), "already in progress") {
			c.JSON(http.StatusConflict, gin.H{
				"error": gin.H{
					"code":    "PREPULL_IN_PROGRESS",
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

	c.JSON(http.StatusAccepted, prepull.ToPrepullResponse())
}

// ListPrepulls handles GET /images/prepull
func (h *PrepullHandler) ListPrepulls(c *gin.Context) {
	image := c.Query("image")
	status := c.Query("status")

	result, err := h.prepullSvc.List(c.Request.Context(), image, status)
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

// DeletePrepull handles DELETE /images/prepull/:id
func (h *PrepullHandler) DeletePrepull(c *gin.Context) {
	id := c.Param("id")

	err := h.prepullSvc.Delete(c.Request.Context(), id)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			c.JSON(http.StatusNotFound, gin.H{
				"error": gin.H{
					"code":    "PREPULL_NOT_FOUND",
					"message": "Prepull task '" + id + "' not found",
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

// PrepullTemplate handles POST /templates/:name/prepull
func (h *PrepullHandler) PrepullTemplate(c *gin.Context) {
	name := c.Param("name")

	// Get template to find its image
	template, err := h.templateSvc.Get(c.Request.Context(), name)
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

	if template.Spec == nil || template.Spec.Image == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": gin.H{
				"code":    "INVALID_REQUEST",
				"message": "Template has no image configured",
			},
		})
		return
	}

	prepull, err := h.prepullSvc.PrepullTemplateImage(c.Request.Context(), name, template.Spec.Image)
	if err != nil {
		if strings.Contains(err.Error(), "already in progress") {
			c.JSON(http.StatusConflict, gin.H{
				"error": gin.H{
					"code":    "PREPULL_IN_PROGRESS",
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

	c.JSON(http.StatusAccepted, prepull.ToPrepullResponse())
}
