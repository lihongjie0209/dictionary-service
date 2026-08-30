package httptransport

import (
	"log/slog"

	"github.com/gin-gonic/gin"
	"github.com/lihongjie0209/dictionary-service/internal/apperror"
	"github.com/lihongjie0209/dictionary-service/internal/buildinfo"
	"github.com/lihongjie0209/dictionary-service/internal/dictionary"
	"github.com/lihongjie0209/dictionary-service/internal/health"
)

type Handler struct {
	logger       *slog.Logger
	health       *health.Service
	dictionaries *dictionary.Service
}

func NewHandler(healthService *health.Service, dictionaries *dictionary.Service, logger *slog.Logger) *Handler {
	return &Handler{health: healthService, dictionaries: dictionaries, logger: logger}
}

type MeResponseBody struct {
	Subject string `json:"subject"`
}

// Live godoc
// @Summary Check process liveness
// @Tags operations
// @Produce json
// @Success 200 {object} Response{body=health.Status}
// @Router /live [post]
func (h *Handler) Live(c *gin.Context) { OK(c, h.health.Live()) }

// Ready godoc
// @Summary Check database and Redis readiness
// @Tags operations
// @Produce json
// @Success 200 {object} Response{body=health.Status}
// @Failure 503 {object} Response{body=health.Status} "Code 50003: dependency unavailable"
// @Router /ready [post]
func (h *Handler) Ready(c *gin.Context) {
	status, ready := h.health.Ready(c.Request.Context())
	if !ready {
		c.JSON(503, Response{Code: apperror.CodeDependencyUnavailable, Message: "service is not ready", Body: status, RequestID: requestID(c)})
		return
	}
	OK(c, status)
}

// Me godoc
// @Summary Return the authenticated subject
// @Tags authentication
// @Produce json
// @Security Bearer
// @Success 200 {object} Response{body=MeResponseBody}
// @Failure 401 {object} Response "Code 20001: unauthorized"
// @Router /api/v1/me [post]
func (h *Handler) Me(c *gin.Context) {
	subject, _ := c.Get("subject")
	OK(c, gin.H{"subject": subject})
}

// Version godoc
// @Summary Return build and runtime version information
// @Tags operations
// @Produce json
// @Success 200 {object} Response{body=buildinfo.Info}
// @Router /api/v1/version [post]
func (h *Handler) Version(c *gin.Context) { OK(c, buildinfo.Current()) }
