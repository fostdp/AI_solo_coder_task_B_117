package handlers

import (
	"context"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	"waterwheel-monitor/internal/config"
	"waterwheel-monitor/internal/efficiency"
	"waterwheel-monitor/internal/forecasting"
	"waterwheel-monitor/internal/models"
	"waterwheel-monitor/internal/scheduler"
	"waterwheel-monitor/internal/virtualbuild"
)

type V3Handler struct {
	scheduler   *scheduler.LPScheduler
	forecaster  *forecasting.WaterLevelForecaster
	comparison  *efficiency.AncientsVsModern
	builder     *virtualbuild.BuildEngine
	compParams  *config.ComparisonParams
}

func NewV3(
	s *scheduler.LPScheduler,
	f *forecasting.WaterLevelForecaster,
	c *efficiency.AncientsVsModern,
	b *virtualbuild.BuildEngine,
	cp *config.ComparisonParams,
) *V3Handler {
	return &V3Handler{scheduler: s, forecaster: f, comparison: c, builder: b, compParams: cp}
}

// ============================================================
// 模块一: 灌溉调度
// ============================================================

func (h *V3Handler) ListIrrigationFields(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()
	fields, err := h.scheduler.ListFields(ctx)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, fields)
}

func (h *V3Handler) RunIrrigationSchedule(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
	defer cancel()

	var req models.ScheduleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "参数无效: " + err.Error()})
		return
	}
	if req.FieldID <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "田块ID必填"})
		return
	}
	sol, err := h.scheduler.ScheduleIrrigation(ctx, req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, sol)
}

func (h *V3Handler) ListScheduleSolutions(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()
	fid, _ := strconv.ParseInt(c.Query("field_id"), 10, 64)
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))
	sols, err := h.scheduler.ListSolutions(ctx, fid, limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, sols)
}

// ============================================================
// 模块二: 水位预测与高度调节
// ============================================================

func (h *V3Handler) GenerateForecast(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), 15*time.Second)
	defer cancel()
	wid, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || wid <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "筒车ID无效"})
		return
	}
	horizon, _ := strconv.Atoi(c.DefaultQuery("horizon_days", "30"))
	f, err := h.forecaster.GenerateForecast(ctx, wid, horizon)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, f)
}

func (h *V3Handler) ProposeHeightAdjustment(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), 15*time.Second)
	defer cancel()
	wid, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || wid <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "筒车ID无效"})
		return
	}
	var body struct {
		ForecastID    int64   `json:"forecast_id"`
		CurrentHeight float64 `json:"current_height_m"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		body.CurrentHeight, _ = strconv.ParseFloat(c.DefaultQuery("current_height_m", "0"), 64)
		body.ForecastID, _ = strconv.ParseInt(c.Query("forecast_id"), 10, 64)
	}
	adj, err := h.forecaster.ProposeHeightAdjustment(ctx, wid, body.ForecastID, body.CurrentHeight)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, adj)
}

func (h *V3Handler) ListForecasts(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()
	wid, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))
	list, err := h.forecaster.ListForecasts(ctx, wid, limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, list)
}

func (h *V3Handler) ListAdjustments(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()
	wid, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))
	list, err := h.forecaster.ListAdjustments(ctx, wid, limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, list)
}

func (h *V3Handler) MarkAdjustmentDone(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()
	aid, err := strconv.ParseInt(c.Param("adj_id"), 10, 64)
	if err != nil || aid <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "调节建议ID无效"})
		return
	}
	if err := h.forecaster.MarkAdjustmentImplemented(ctx, aid); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

// ============================================================
// 模块三: 古今能效对比
// ============================================================

func (h *V3Handler) RunEfficiencyComparison(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), 30*time.Second)
	defer cancel()
	wid, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || wid <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "筒车ID无效"})
		return
	}
	days, _ := strconv.Atoi(c.DefaultQuery("period_days", strconv.Itoa(h.compParams.DefaultCompareDays)))
	scenario := c.DefaultQuery("scenario", "standard")
	comp, err := h.comparison.CompareWaterwheelPump(ctx, wid, days, scenario)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, comp)
}

func (h *V3Handler) ListEfficiencyComparisons(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()
	wid, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))
	list, err := h.comparison.ListComparisons(ctx, wid, limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, list)
}

func (h *V3Handler) GetBuildPresets(c *gin.Context) {
	c.JSON(http.StatusOK, h.comparison.GetBuildPresets())
}

// ============================================================
// 模块四: 虚拟建造筒车
// ============================================================

func (h *V3Handler) SimulateBuild(c *gin.Context) {
	var build models.VirtualBuild
	if err := c.ShouldBindJSON(&build); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "参数无效: " + err.Error()})
		return
	}
	flow, _ := strconv.ParseFloat(c.DefaultQuery("flow_velocity", "0"), 64)
	drop, _ := strconv.ParseFloat(c.DefaultQuery("water_drop", "0"), 64)
	sim, err := h.builder.ValidateAndSimulate(&build, flow, drop)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"simulation":   sim,
		"build_params": build,
	})
}

func (h *V3Handler) SaveBuild(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
	defer cancel()
	var body struct {
		Build             models.VirtualBuild `json:"build"`
		FlowVelocity      float64             `json:"flow_velocity"`
		WaterDrop         float64             `json:"water_drop"`
		GenerateBlueprint bool                `json:"generate_blueprint"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "参数无效: " + err.Error()})
		return
	}
	_, err := h.builder.ValidateAndSimulate(&body.Build, body.FlowVelocity, body.WaterDrop)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "模拟失败: " + err.Error()})
		return
	}
	if body.GenerateBlueprint {
		body.Build.Blueprint = h.builder.GenerateBlueprint(&body.Build)
	}
	id, err := h.builder.SaveBuild(ctx, &body.Build)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"id": id, "build": body.Build})
}

func (h *V3Handler) ListBuilds(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()
	userID := c.DefaultQuery("user_id", "")
	publicOnly := c.DefaultQuery("public", "1") == "1"
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))
	list, err := h.builder.ListBuilds(ctx, userID, publicOnly, limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, list)
}

func (h *V3Handler) LikeBuild(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()
	bid, err := strconv.ParseInt(c.Param("build_id"), 10, 64)
	if err != nil || bid <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "作品ID无效"})
		return
	}
	if err := h.builder.LikeBuild(ctx, bid); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

func (h *V3Handler) GenerateBuildBlueprint(c *gin.Context) {
	var build models.VirtualBuild
	if err := c.ShouldBindJSON(&build); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "参数无效"})
		return
	}
	svg := h.builder.GenerateBlueprint(&build)
	c.Header("Content-Type", "image/svg+xml")
	c.String(http.StatusOK, svg)
}
