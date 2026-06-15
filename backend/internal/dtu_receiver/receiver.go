package dtu_receiver

import (
	"context"
	"log"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"waterwheel-monitor/internal/config"
	"waterwheel-monitor/internal/database"
	"waterwheel-monitor/internal/metrics"
	"waterwheel-monitor/internal/models"
	"waterwheel-monitor/internal/pipeline"
)

type DTUReceiver struct {
	db      *database.Database
	chans   *pipeline.Channels
	params  *config.ReceiverParams
	workers int
}

func New(db *database.Database, chans *pipeline.Channels, params *config.ReceiverParams) *DTUReceiver {
	return &DTUReceiver{
		db:      db,
		chans:   chans,
		params:  params,
		workers: params.WorkerCount,
	}
}

func (r *DTUReceiver) Start(ctx context.Context) {
	for i := 0; i < r.workers; i++ {
		go r.dispatchWorker(ctx, i)
	}
	log.Printf("[DTU Receiver] Started with %d worker goroutines", r.workers)
}

func (r *DTUReceiver) dispatchWorker(ctx context.Context, id int) {
	for {
		select {
		case <-ctx.Done():
			log.Printf("[DTU Receiver] Worker %d stopped", id)
			return
		}
	}
}

func (r *DTUReceiver) HandleReportTelemetry(c *gin.Context) {
	var data models.TelemetryData
	if err := c.ShouldBindJSON(&data); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if data.WaterwheelID <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "waterwheel_id is required"})
		return
	}

	if r.params.ValidateBounds && !r.validateBounds(&data) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "telemetry values out of configured bounds"})
		return
	}

	wheel, err := r.db.GetWaterwheelByID(c.Request.Context(), data.WaterwheelID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid waterwheel_id"})
		return
	}

	if data.Time.IsZero() {
		data.Time = time.Now()
	}

	msg := pipeline.RawTelemetryMsg{
		Wheel: wheel,
		Data:  &data,
	}

	select {
	case r.chans.RawCh <- msg:
		metrics.IncTelemetryReceived()
	default:
		log.Printf("[DTU Receiver] Warning: RawCh full, dropping telemetry for wheel %d", wheel.ID)
	}

	c.JSON(http.StatusAccepted, gin.H{
		"status":   "accepted",
		"wheel_id": wheel.ID,
		"ts":       data.Time,
	})
}

func (r *DTUReceiver) validateBounds(d *models.TelemetryData) bool {
	p := r.params
	valid := true

	if d.RotationSpeed < p.MinRotationSpeed || d.RotationSpeed > p.MaxRotationSpeed {
		valid = false
	}
	if d.WaterLift < p.MinWaterLift || d.WaterLift > p.MaxWaterLift {
		valid = false
	}
	if d.WaterLevelDrop < p.MinDrop || d.WaterLevelDrop > p.MaxDrop {
		valid = false
	}
	if d.FlowVelocity < p.MinFlow || d.FlowVelocity > p.MaxFlow {
		valid = false
	}
	return valid
}

func (r *DTUReceiver) HandleHealth(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"status":      "ok",
		"time":        time.Now().UTC(),
		"queue_depth": len(r.chans.RawCh),
		"workers":     r.workers,
	})
}
