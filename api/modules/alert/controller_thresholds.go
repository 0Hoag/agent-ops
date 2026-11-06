package alert

import (
	"errors"
	"net/http"
	"time"

	"github.com/0Hoag/agent-ops/api/pkg/response"
	"github.com/gin-gonic/gin"
)

var defaultThresholds = Thresholds{MinCacheHit: 0.8, SilentAfter: 5 * time.Minute, PRStaleAfter: 24 * time.Hour}

type thresholdRequest struct {
	MinCacheHit    float64 `json:"min_cache_hit"`
	SilentSeconds  int64   `json:"silent_seconds"`
	PRStaleSeconds int64   `json:"pr_stale_seconds"`
}

// GetThresholds returns persisted thresholds for the selected organization, or defaults when unset.
// @Summary Get alert thresholds
// @Tags alerts
// @Produce json
// @Success 200 {object} ThresholdsResponse
// @Failure 401 {object} response.ErrorResponse
// @Failure 403 {object} response.ErrorResponse
// @Router /api/v1/alerts/thresholds [get]
func (c *Controller) GetThresholds(ctx *gin.Context) {
	org, ok := c.thresholdOrganization(ctx, false)
	if !ok {
		return
	}
	v, err := c.repository.GetThresholds(ctx.Request.Context(), org)
	if err != nil {
		response.Error(ctx, http.StatusInternalServerError, "failed to get alert thresholds")
		return
	}
	if v == nil {
		v = &defaultThresholds
	}
	response.OK(ctx, thresholdResponse(*v))
}

// UpdateThresholds replaces persisted thresholds for the selected organization.
// @Summary Update alert thresholds
// @Tags alerts
// @Accept json
// @Produce json
// @Param thresholds body thresholdRequest true "Alert thresholds; durations are positive integer seconds"
// @Success 200 {object} ThresholdsResponse
// @Failure 400 {object} response.ErrorResponse
// @Failure 401 {object} response.ErrorResponse
// @Failure 403 {object} response.ErrorResponse
// @Router /api/v1/alerts/thresholds [put]
func (c *Controller) UpdateThresholds(ctx *gin.Context) {
	org, ok := c.thresholdOrganization(ctx, true)
	if !ok {
		return
	}
	var req thresholdRequest
	maxSeconds := int64(^uint64(0)>>1) / int64(time.Second)
	if ctx.ShouldBindJSON(&req) != nil || req.MinCacheHit < 0 || req.MinCacheHit > 1 || req.SilentSeconds <= 0 || req.PRStaleSeconds <= 0 || req.SilentSeconds > maxSeconds || req.PRStaleSeconds > maxSeconds {
		response.Error(ctx, http.StatusBadRequest, "invalid alert thresholds")
		return
	}
	v := Thresholds{MinCacheHit: req.MinCacheHit, SilentAfter: time.Duration(req.SilentSeconds) * time.Second, PRStaleAfter: time.Duration(req.PRStaleSeconds) * time.Second}
	if err := c.repository.UpsertThresholds(ctx.Request.Context(), org, v); err != nil {
		if errors.Is(err, ErrInvalidThresholds) {
			response.Error(ctx, http.StatusBadRequest, "invalid alert thresholds")
			return
		}
		response.Error(ctx, http.StatusInternalServerError, "failed to update alert thresholds")
		return
	}
	response.OK(ctx, thresholdResponse(v))
}
