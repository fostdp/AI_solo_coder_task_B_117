package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"

	"waterwheel-monitor/internal/alarm_mqtt"
	"waterwheel-monitor/internal/config"
	"waterwheel-monitor/internal/database"
	"waterwheel-monitor/internal/dtu_receiver"
	"waterwheel-monitor/internal/efficiency"
	"waterwheel-monitor/internal/forecasting"
	"waterwheel-monitor/internal/handlers"
	"waterwheel-monitor/internal/hydraulic_model"
	"waterwheel-monitor/internal/metrics"
	"waterwheel-monitor/internal/models"
	"waterwheel-monitor/internal/pipeline"
	"waterwheel-monitor/internal/scheduler"
	"waterwheel-monitor/internal/shape_optimizer"
	"waterwheel-monitor/internal/virtualbuild"

	"net/http/pprof"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	hydraulicParams := config.DefaultHydraulicParams()
	optimizerParams := config.DefaultOptimizerParams()
	alarmParams := config.DefaultAlarmParams()
	receiverParams := config.DefaultReceiverParams()
	schedulerParams := config.DefaultSchedulerParams()
	forecastParams := config.DefaultForecastingParams()
	compParams := config.DefaultComparisonParams()
	buildParams := config.DefaultBuildParams()

	db, err := database.New(cfg)
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}
	defer db.Close()
	log.Println("Database connected successfully")

	chans := pipeline.NewChannels(1024)
	defer chans.Close()

	mqttCfg := &models.MQTTConfig{
		BrokerURL:   cfg.MQTTBroker,
		ClientID:    cfg.MQTTClientID,
		Username:    cfg.MQTTUsername,
		Password:    cfg.MQTTPassword,
		TopicPrefix: cfg.MQTTTopicPrefix,
	}

	receiver := dtu_receiver.New(db, chans, receiverParams)
	hydraulic := hydraulic_model.New(db, chans, hydraulicParams)
	optimizer := shape_optimizer.New(db, chans, optimizerParams, hydraulicParams)
	alerter := alarm_mqtt.New(db, chans, mqttCfg, alarmParams)

	lpScheduler := scheduler.NewLPScheduler(db, schedulerParams)
	forecaster := forecasting.NewWaterLevelForecaster(db, forecastParams)
	comparer := efficiency.NewAncientsVsModern(db, compParams)
	builder := virtualbuild.NewBuildEngine(db, buildParams)

	rootCtx, rootCancel := context.WithCancel(context.Background())
	defer rootCancel()

	receiver.Start(rootCtx)
	hydraulic.Start(rootCtx)
	optimizer.Start(rootCtx)
	alerter.Start(rootCtx)

	go metricsCollector(rootCtx, chans, 5*time.Second)

	h := handlers.NewV2(db, chans, hydraulic, optimizer, cfg.EfficiencyAlertThreshold)
	v3 := handlers.NewV3(lpScheduler, forecaster, comparer, builder, compParams)

	r := gin.New()
	r.Use(gin.Recovery())
	r.Use(metrics.PrometheusMiddleware())

	r.Use(cors.New(cors.Config{
		AllowAllOrigins:  true,
		AllowMethods:     []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowHeaders:     []string{"Origin", "Content-Type", "Accept", "Authorization"},
		ExposeHeaders:    []string{"Content-Length"},
		AllowCredentials: true,
		MaxAge:           12 * time.Hour,
	}))

	debug := r.Group("/debug/pprof")
	{
		debug.GET("/", gin.WrapF(pprof.Index))
		debug.GET("/cmdline", gin.WrapF(pprof.Cmdline))
		debug.GET("/profile", gin.WrapF(pprof.Profile))
		debug.GET("/symbol", gin.WrapF(pprof.Symbol))
		debug.GET("/trace", gin.WrapF(pprof.Trace))
		debug.GET("/allocs", gin.WrapH(pprof.Handler("allocs")))
		debug.GET("/block", gin.WrapH(pprof.Handler("block")))
		debug.GET("/goroutine", gin.WrapH(pprof.Handler("goroutine")))
		debug.GET("/heap", gin.WrapH(pprof.Handler("heap")))
		debug.GET("/mutex", gin.WrapH(pprof.Handler("mutex")))
		debug.GET("/threadcreate", gin.WrapH(pprof.Handler("threadcreate")))
	}

	r.GET("/metrics", metrics.PrometheusHandler())

	r.Static("/static", "../../frontend")
	r.StaticFile("/", "../../frontend/index.html")

	api := r.Group("/api")
	{
		api.GET("/health", receiver.HandleHealth)

		api.GET("/waterwheels", h.GetWaterwheels)
		api.GET("/waterwheels/:id", h.GetWaterwheel)

		api.POST("/telemetry", receiver.HandleReportTelemetry)
		api.GET("/waterwheels/:id/telemetry", h.GetTelemetry)
		api.GET("/waterwheels/:id/telemetry/range", h.GetTelemetryRange)

		api.GET("/waterwheels/:id/efficiency", h.GetEfficiencyAnalysis)

		api.GET("/waterwheels/:id/alerts", h.GetAlerts)

		api.POST("/waterwheels/:id/optimize", h.RunOptimizationV2)
		api.GET("/waterwheels/:id/optimizations", h.GetOptimizationResults)

		// ============================================================
		// Feature V2: 四大新功能路由
		// ============================================================
		// 模块一: 多筒车协同灌溉调度
		api.GET("/irrigation/fields", v3.ListIrrigationFields)
		api.POST("/irrigation/schedule", v3.RunIrrigationSchedule)
		api.GET("/irrigation/schedules", v3.ListScheduleSolutions)

		// 模块二: 季节性水位预测与高度调节
		api.POST("/waterwheels/:id/forecast/generate", v3.GenerateForecast)
		api.GET("/waterwheels/:id/forecasts", v3.ListForecasts)
		api.POST("/waterwheels/:id/adjustment/propose", v3.ProposeHeightAdjustment)
		api.GET("/waterwheels/:id/adjustments", v3.ListAdjustments)
		api.POST("/adjustments/:adj_id/implement", v3.MarkAdjustmentDone)

		// 模块三: 古代筒车 vs 现代水泵 能效对比
		api.POST("/waterwheels/:id/comparison/run", v3.RunEfficiencyComparison)
		api.GET("/waterwheels/:id/comparisons", v3.ListEfficiencyComparisons)
		api.GET("/build-presets", v3.GetBuildPresets)

		// 模块四: 公众虚拟建造筒车
		api.POST("/virtual-build/simulate", v3.SimulateBuild)
		api.POST("/virtual-build/save", v3.SaveBuild)
		api.GET("/virtual-build/list", v3.ListBuilds)
		api.POST("/virtual-build/:build_id/like", v3.LikeBuild)
		api.POST("/virtual-build/blueprint", v3.GenerateBuildBlueprint)
	}

	srv := &http.Server{
		Addr:    ":" + cfg.ServerPort,
		Handler: r,
	}

	go func() {
		log.Printf("Server starting on port %s | Hydraulic workers:%d Optimizer workers:%d",
			cfg.ServerPort, 2, 1)
		log.Printf("Prometheus metrics available at :%s/metrics", cfg.ServerPort)
		log.Printf("pprof available at :%s/debug/pprof/", cfg.ServerPort)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Failed to start server: %v", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Println("Shutting down modules...")

	rootCancel()

	time.Sleep(200 * time.Millisecond)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		log.Fatalf("Server forced to shutdown: %v", err)
	}

	log.Println("Server exited cleanly")
}

func metricsCollector(ctx context.Context, chans *pipeline.Channels, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			log.Println("[Metrics Collector] Stopped")
			return
		case <-ticker.C:
			metrics.SetChannelDepth("raw", len(chans.RawCh))
			metrics.SetChannelDepth("enriched", len(chans.EnrichedCh))
			metrics.SetChannelDepth("alert", len(chans.AlertCh))
			metrics.SetChannelDepth("optimize_req", len(chans.OptimizeReqCh))
		}
	}
}

func mustAtoi(s string, fallback int) int {
	if v, err := strconv.Atoi(s); err == nil {
		return v
	}
	return fallback
}
