package metrics

import (
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var (
	httpRequestsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "http_requests_total",
			Help: "Total number of HTTP requests",
		},
		[]string{"method", "path", "status"},
	)

	httpRequestDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "http_request_duration_seconds",
			Help:    "HTTP request duration in seconds",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"method", "path"},
	)

	httpRequestsInFlight = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "http_requests_in_flight",
			Help: "Current number of in-flight HTTP requests",
		},
	)

	telemetryReceived = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "telemetry_received_total",
			Help: "Total telemetry reports received",
		},
	)

	alertsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "alerts_total",
			Help: "Total alerts generated",
		},
		[]string{"severity"},
	)

	optimizationCount = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "optimization_runs_total",
			Help: "Total optimization runs executed",
		},
	)

	optimizationDuration = promauto.NewHistogram(
		prometheus.HistogramOpts{
			Name:    "optimization_duration_seconds",
			Help:    "Optimization run duration in seconds",
			Buckets: []float64{0.5, 1, 2, 5, 10, 30, 60},
		},
	)

	databaseQueryDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "db_query_duration_seconds",
			Help:    "Database query duration in seconds",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"query"},
	)

	modelDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "hydraulic_model_duration_seconds",
			Help:    "Hydraulic model calculation duration in seconds",
		},
		[]string{"type"},
	)

	channelDepth = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "channel_depth",
			Help: "Current depth of pipeline channels",
		},
		[]string{"channel"},
	)

	efficiencyGauge = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "waterwheel_efficiency",
			Help: "Current efficiency of each waterwheel",
		},
		[]string{"wheel_id", "type"},
	)
)

func PrometheusMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		httpRequestsInFlight.Inc()
		defer httpRequestsInFlight.Dec()

		start := time.Now()
		c.Next()

		duration := time.Since(start).Seconds()
		status := strconv.Itoa(c.Writer.Status())
		path := c.FullPath()
		if path == "" {
			path = c.Request.URL.Path
		}

		httpRequestsTotal.WithLabelValues(c.Request.Method, path, status).Inc()
		httpRequestDuration.WithLabelValues(c.Request.Method, path).Observe(duration)
	}
}

func PrometheusHandler() gin.HandlerFunc {
	h := promhttp.Handler()
	return func(c *gin.Context) {
		h.ServeHTTP(c.Writer, c.Request)
	}
}

func IncTelemetryReceived() {
	telemetryReceived.Inc()
}

func IncAlert(severity string) {
	alertsTotal.WithLabelValues(severity).Inc()
}

func IncOptimization() {
	optimizationCount.Inc()
}

func ObserveOptimizationDuration(d time.Duration) {
	optimizationDuration.Observe(d.Seconds())
}

func ObserveDatabaseQuery(query string, d time.Duration) {
	databaseQueryDuration.WithLabelValues(query).Observe(d.Seconds())
}

func ObserveModelDuration(typ string, d time.Duration) {
	modelDuration.WithLabelValues(typ).Observe(d.Seconds())
}

func SetChannelDepth(ch string, depth int) {
	channelDepth.WithLabelValues(ch).Set(float64(depth))
}

func SetWheelEfficiency(wheelID string, typ string, value float64) {
	efficiencyGauge.WithLabelValues(wheelID, typ).Set(value)
}

func PrometheusHandlerFunc() http.Handler {
	return promhttp.Handler()
}
