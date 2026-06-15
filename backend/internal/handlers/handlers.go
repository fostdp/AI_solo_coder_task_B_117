package handlers

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	"waterwheel-monitor/internal/database"
	"waterwheel-monitor/internal/efficiency"
	"waterwheel-monitor/internal/models"
	"waterwheel-monitor/internal/mqtt"
	"waterwheel-monitor/internal/optimizer"
)

type Handler struct {
	db         *database.Database
	effCalc    *efficiency.Calculator
	ga         *optimizer.GAOptimizer
	alertMQTT  *mqtt.AlertClient
	alertThresh float64
}

func New(db *database.Database, effCalc *efficiency.Calculator, ga *optimizer.GAOptimizer,
	alertMQTT *mqtt.AlertClient, alertThresh float64) *Handler {
	return &Handler{
		db:          db,
		effCalc:     effCalc,
		ga:          ga,
		alertMQTT:   alertMQTT,
		alertThresh: alertThresh,
	}
}

func (h *Handler) GetWaterwheels(c *gin.Context) {
	wheels, err := h.db.GetWaterwheels(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, wheels)
}

func (h *Handler) GetWaterwheel(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
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

func (h *Handler) ReportTelemetry(c *gin.Context) {
	var data models.TelemetryData
	if err := c.ShouldBindJSON(&data); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if data.WaterwheelID <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "waterwheel_id is required"})
		return
	}

	wheel, err := h.db.GetWaterwheelByID(c.Request.Context(), data.WaterwheelID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid waterwheel_id"})
		return
	}

	h.effCalc.EnrichTelemetry(wheel, &data)

	if err := h.db.InsertTelemetry(c.Request.Context(), &data); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	go h.checkAndAlert(c.Request.Context(), wheel, &data)

	c.JSON(http.StatusOK, gin.H{
		"status":                  "ok",
		"mechanical_efficiency":   data.MechanicalEfficiency,
		"hydraulic_efficiency":    data.HydraulicEfficiency,
		"torque":                  data.Torque,
		"power_output":            data.PowerOutput,
	})
}

func (h *Handler) checkAndAlert(ctx context.Context, wheel *models.Waterwheel, data *models.TelemetryData) {
	if data.MechanicalEfficiency == nil || data.HydraulicEfficiency == nil {
		return
	}

	currentEff := *data.MechanicalEfficiency * *data.HydraulicEfficiency

	histAvg, err := h.db.GetHistoricalAvgEfficiency(ctx, wheel.ID, 168)
	if err != nil || histAvg <= 0 {
		return
	}

	threshold := histAvg * h.alertThresh
	if currentEff >= threshold {
		return
	}

	alert := &models.Alert{
		WaterwheelID:    wheel.ID,
		AlertType:       "low_efficiency",
		Message:         fmt.Sprintf("筒车【%s】效率异常：当前综合效率 %.2f%%，低于历史平均(%.2f%%)的 %d%%",
			wheel.Name, currentEff*100, histAvg*100, int(h.alertThresh*100)),
		Severity:        "warning",
		EfficiencyValue: currentEff,
		HistoricalAvg:   histAvg,
		Time:            time.Now(),
	}

	if err := h.db.InsertAlert(ctx, alert); err != nil {
		log.Printf("Failed to insert alert: %v", err)
		return
	}

	if h.alertMQTT != nil {
		if err := h.alertMQTT.PublishAlert(alert, wheel.Name, h.alertThresh); err != nil {
			log.Printf("Failed to publish MQTT alert: %v", err)
		}
	}
}

func (h *Handler) GetTelemetry(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
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

func (h *Handler) GetTelemetryRange(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
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

func (h *Handler) GetEfficiencyAnalysis(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
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

	analysis := h.effCalc.Analyze(wheel, &latest[0])
	c.JSON(http.StatusOK, analysis)
}

func (h *Handler) GetAlerts(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
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

func (h *Handler) RunOptimization(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
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

	result := h.ga.Optimize(wheel, &latest[0])

	if err := h.db.InsertOptimizationResult(c.Request.Context(), result); err != nil {
		log.Printf("Failed to save optimization result: %v", err)
	}

	c.JSON(http.StatusOK, result)
}

func (h *Handler) GetOptimizationResults(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
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

func (h *Handler) HealthCheck(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"status": "ok",
		"time":   time.Now().UTC(),
	})
}
