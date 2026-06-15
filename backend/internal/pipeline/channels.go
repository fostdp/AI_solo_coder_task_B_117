package pipeline

import (
	"waterwheel-monitor/internal/models"
)

type MsgType int

const (
	MsgRawTelemetry MsgType = iota
	MsgEnrichedTelemetry
	MsgAlert
	MsgOptimizeRequest
	MsgOptimizeResult
)

type RawTelemetryMsg struct {
	Wheel *models.Waterwheel
	Data  *models.TelemetryData
}

type EnrichedTelemetryMsg struct {
	Wheel    *models.Waterwheel
	Data     *models.TelemetryData
	Analysis *models.EfficiencyAnalysis
}

type AlertMsg struct {
	Wheel       *models.Waterwheel
	CurrentEff  float64
	HistoricalAvg float64
	Threshold   float64
	Data        *models.TelemetryData
}

type OptimizeRequest struct {
	Wheel    *models.Waterwheel
	Data     *models.TelemetryData
	ResultCh chan *models.OptimizationResult
}

type Channels struct {
	RawCh         chan RawTelemetryMsg
	EnrichedCh    chan EnrichedTelemetryMsg
	AlertCh       chan AlertMsg
	OptimizeReqCh chan OptimizeRequest
}

func NewChannels(bufferSize int) *Channels {
	return &Channels{
		RawCh:         make(chan RawTelemetryMsg, bufferSize),
		EnrichedCh:    make(chan EnrichedTelemetryMsg, bufferSize),
		AlertCh:       make(chan AlertMsg, bufferSize),
		OptimizeReqCh: make(chan OptimizeRequest, 32),
	}
}

func (ch *Channels) Close() {
	close(ch.RawCh)
	close(ch.EnrichedCh)
	close(ch.AlertCh)
	close(ch.OptimizeReqCh)
}
