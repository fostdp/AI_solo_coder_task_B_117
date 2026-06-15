package handlers

import (
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	"waterwheel-monitor/internal/database"
	"waterwheel-monitor/internal/hydraulic_model"
	"waterwheel-monitor/internal/models"
	"waterwheel-monitor/internal/pipeline"
	"waterwheel-monitor/internal/shape_optimizer"
)

type HandlerV2 struct {
	db        *database.Database
	chans     *pipeline.Channels
	hydraulic *hydraulic_model.HydraulicModel
	optimizer *shape_optimizer.ShapeOptimizer
	threshold float64
}

func NewV2(db *database.Database, chans *pipeline.Channels,
	hydraulic *hydraulic_model.HydraulicModel, optimizer *shape_optimizer.ShapeOptimizer,
	threshold float64) *HandlerV2 {
	return &HandlerV2{
		db:        db,
		chans:     chans,
		hydraulic: hydraulic,
		optimizer: optimizer,
		threshold: threshold,
	}
}

func (h *HandlerV2) GetWaterwheels(c *gin.Context) {
	wheels, err := h.db.GetWaterwheels(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, wheels)
}

func (h *HandlerV2) GetWaterwheel(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	wheel, err := h.db.GetWaterwheelByID(c.Request.Context(), id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "waterwheel not found"})
		return
	}
	c.JSON(http.StatusOK, wheel)
}

func (h *HandlerV2) GetTelemetry(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}

	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "100"))
	if limit <= 0 || limit > 10000 {
		limit = 100
	}

	data, err := h.db.GetLatestTelemetry(c.Request.Context(), id, limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, data)
}

func (h *HandlerV2) GetTelemetryRange(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}

	startStr := c.Query("start")
	endStr := c.Query("end")

	start, err := time.Parse(time.RFC3339, startStr)
	if err != nil {
		start = time.Now().Add(-24 * time.Hour)
	}
	end, err := time.Parse(time.RFC3339, endStr)
	if err != nil {
		end = time.Now()
	}

	data, err := h.db.GetTelemetryRange(c.Request.Context(), id, start, end)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, data)
}

func (h *HandlerV2) GetEfficiencyAnalysis(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}

	wheel, err := h.db.GetWaterwheelByID(c.Request.Context(), id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "waterwheel not found"})
		return
	}

	latest, err := h.db.GetLatestTelemetry(c.Request.Context(), id, 1)
	if err != nil || len(latest) == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "no telemetry data"})
		return
	}

	analysis := h.hydraulic.Analyze(wheel, &latest[0])
	c.JSON(http.StatusOK, analysis)
}

func (h *HandlerV2) GetAlerts(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}

	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))
	if limit <= 0 || limit > 500 {
		limit = 50
	}

	alerts, err := h.db.GetAlerts(c.Request.Context(), id, limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, alerts)
}

func (h *HandlerV2) RunOptimizationV2(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}

	wheel, err := h.db.GetWaterwheelByID(c.Request.Context(), id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "waterwheel not found"})
		return
	}

	latest, err := h.db.GetLatestTelemetry(c.Request.Context(), id, 1)
	if err != nil || len(latest) == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "no telemetry data available for optimization"})
		return
	}

	resultCh := make(chan *models.OptimizationResult, 1)
	req := pipeline.OptimizeRequest{
		Wheel:    wheel,
		Data:     &latest[0],
		ResultCh: resultCh,
	}

	select {
	case h.chans.OptimizeReqCh <- req:
	default:
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "optimizer queue full, try later"})
		return
	}

	select {
	case result := <-resultCh:
		c.JSON(http.StatusOK, result)
	case <-time.After(30 * time.Second):
		c.JSON(http.StatusRequestTimeout, gin.H{"error": "optimization timeout (30s)"})
		return
	case <-c.Request.Context().Done():
		c.JSON(http.StatusRequestTimeout, gin.H{"error": "client disconnected"})
	}
}

func (h *HandlerV2) GetOptimizationResults(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}

	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "10"))
	if limit <= 0 || limit > 100 {
		limit = 10
	}

	results, err := h.db.GetOptimizationResults(c.Request.Context(), id, limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, results)
}
