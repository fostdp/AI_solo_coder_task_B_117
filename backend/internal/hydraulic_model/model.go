package hydraulic_model

import (
	"context"
	"fmt"
	"log"
	"math"
	"time"

	"waterwheel-monitor/internal/config"
	"waterwheel-monitor/internal/database"
	"waterwheel-monitor/internal/metrics"
	"waterwheel-monitor/internal/models"
	"waterwheel-monitor/internal/pipeline"
)

type HydraulicModel struct {
	db       *database.Database
	chans    *pipeline.Channels
	params   *config.HydraulicParams
	workers  int
}

func New(db *database.Database, chans *pipeline.Channels, params *config.HydraulicParams) *HydraulicModel {
	return &HydraulicModel{
		db:      db,
		chans:   chans,
		params:  params,
		workers: 2,
	}
}

func (hm *HydraulicModel) Start(ctx context.Context) {
	for i := 0; i < hm.workers; i++ {
		go hm.modelWorker(ctx, i)
	}
	log.Printf("[Hydraulic Model] Started with %d workers", hm.workers)
}

func (hm *HydraulicModel) modelWorker(ctx context.Context, id int) {
	for {
		select {
		case <-ctx.Done():
			log.Printf("[Hydraulic Model] Worker %d stopped", id)
			return
		case msg, ok := <-hm.chans.RawCh:
			if !ok {
				log.Printf("[Hydraulic Model] Worker %d: RawCh closed", id)
				return
			}
			hm.processMessage(&msg)
		}
	}
}

func (hm *HydraulicModel) processMessage(msg *pipeline.RawTelemetryMsg) {
	start := time.Now()
	analysis := hm.Analyze(msg.Wheel, msg.Data)
	metrics.ObserveModelDuration("analyze", time.Since(start))

	wheelID := fmt.Sprintf("%d", msg.Wheel.ID)
	metrics.SetWheelEfficiency(wheelID, "mechanical", analysis.MechanicalEfficiency)
	metrics.SetWheelEfficiency(wheelID, "hydraulic", analysis.HydraulicEfficiency)
	metrics.SetWheelEfficiency(wheelID, "overall", analysis.MechanicalEfficiency*analysis.HydraulicEfficiency)

	dataCopy := *msg.Data
	dataCopy.MechanicalEfficiency = &analysis.MechanicalEfficiency
	dataCopy.HydraulicEfficiency = &analysis.HydraulicEfficiency
	dataCopy.Torque = &analysis.TorqueInput
	dataCopy.PowerOutput = &analysis.OutputPower

	if err := hm.db.InsertTelemetry(context.Background(), &dataCopy); err != nil {
		log.Printf("[Hydraulic Model] DB insert error (wheel=%d): %v", msg.Wheel.ID, err)
	}

	enriched := pipeline.EnrichedTelemetryMsg{
		Wheel:    msg.Wheel,
		Data:     &dataCopy,
		Analysis: analysis,
	}
	select {
	case hm.chans.EnrichedCh <- enriched:
	default:
	}

	currentEff := analysis.MechanicalEfficiency * analysis.HydraulicEfficiency
	histAvg, err := hm.db.GetHistoricalAvgEfficiency(context.Background(), msg.Wheel.ID, 168)
	if err == nil && histAvg > 0 && currentEff < histAvg*0.8 {
		alert := pipeline.AlertMsg{
			Wheel:         msg.Wheel,
			CurrentEff:    currentEff,
			HistoricalAvg: histAvg,
			Threshold:     0.8,
			Data:          &dataCopy,
		}
		select {
		case hm.chans.AlertCh <- alert:
		default:
			log.Printf("[Hydraulic Model] AlertCh full, dropping alert for wheel %d", msg.Wheel.ID)
		}
	}
}

func (hm *HydraulicModel) Analyze(wheel *models.Waterwheel, data *models.TelemetryData) *models.EfficiencyAnalysis {
	p := hm.params
	radius := wheel.Diameter / 2.0
	angularVelocity := data.RotationSpeed * 2 * math.Pi / 60.0

	torqueInput := hm.calcHydraulicTorque(wheel, data, radius)
	torqueOutput := hm.calcOutputTorque(wheel, data, radius, angularVelocity)
	liftResistance := hm.calcLiftResistance(wheel, data, radius)

	netTorque := torqueInput - torqueOutput - liftResistance -
		p.BearingFriction*torqueInput - p.FrictionCoeff*math.Abs(angularVelocity)

	inputPower := torqueInput * angularVelocity
	outputPower := math.Max(0, netTorque) * angularVelocity

	mechEff := 0.0
	if inputPower > 0 {
		mechEff = math.Max(0, math.Min(1, outputPower/inputPower))
	}

	theoreticalLift := hm.calcTheoreticalLift(wheel, data)
	hydEff := 0.0
	if theoreticalLift > 0 {
		hydEff = math.Max(0, math.Min(1, data.WaterLift/theoreticalLift))
	}

	return &models.EfficiencyAnalysis{
		WaterwheelID:         data.WaterwheelID,
		Time:                 data.Time,
		RotationSpeed:        data.RotationSpeed,
		InputPower:           inputPower,
		OutputPower:          outputPower,
		TorqueInput:          torqueInput,
		TorqueOutput:         torqueOutput,
		LiftResistance:       liftResistance,
		MechanicalEfficiency: mechEff,
		HydraulicEfficiency:  hydEff,
		OverallEfficiency:    mechEff * hydEff,
	}
}

func (hm *HydraulicModel) calcDynamicFillEfficiency(wheel *models.Waterwheel, data *models.TelemetryData) float64 {
	p := hm.params
	radius := wheel.Diameter / 2.0
	angularVelocity := data.RotationSpeed * 2 * math.Pi / 60.0
	if angularVelocity <= 0 {
		return p.MaxFillEfficiency
	}

	submersionRatio := math.Min(1, data.WaterLevelDrop/wheel.Diameter)
	submergedAngle := 2 * math.Asin(math.Sqrt(submersionRatio))
	submergedAngle = math.Max(0.3, math.Min(math.Pi*0.8, submergedAngle))

	immersionTime := submergedAngle / angularVelocity

	fillEff := 1.0 - math.Exp(-immersionTime/p.FillTimeConstant)
	fillEff = p.MinFillEfficiency + fillEff*(p.MaxFillEfficiency-p.MinFillEfficiency)
	return fillEff
}

func (hm *HydraulicModel) calcHydraulicTorque(wheel *models.Waterwheel, data *models.TelemetryData, radius float64) float64 {
	p := hm.params
	submersionRatio := math.Min(1, data.WaterLevelDrop/wheel.Diameter)
	submergedBuckets := int(math.Max(1, float64(wheel.BucketCount)*submersionRatio*0.4))
	fillEff := hm.calcDynamicFillEfficiency(wheel, data)

	bucketForce := p.WaterDensity * p.Gravity * wheel.BucketCapacity * fillEff
	effectiveRadius := radius * p.EffectiveArmRatio

	impactForce := 0.5 * p.WaterDensity * data.FlowVelocity * data.FlowVelocity *
		wheel.BucketCapacity * fillEff * p.ImpulseForceRatio / radius

	torque := float64(submergedBuckets)*bucketForce*effectiveRadius +
		float64(wheel.BucketCount/4)*impactForce*radius
	return torque
}

func (hm *HydraulicModel) calcOutputTorque(wheel *models.Waterwheel, data *models.TelemetryData, radius, omega float64) float64 {
	p := hm.params
	liftedMassPerSecond := data.WaterLift / 60.0 / 1000.0
	liftHeight := wheel.Diameter * p.LiftHeightRatio
	potentialPower := liftedMassPerSecond * p.Gravity * liftHeight
	if omega > 0 {
		return potentialPower / omega
	}
	return 0
}

func (hm *HydraulicModel) calcLiftResistance(wheel *models.Waterwheel, data *models.TelemetryData, radius float64) float64 {
	p := hm.params
	liftedVolumePerMin := data.WaterLift / 1000.0
	omega := data.RotationSpeed * 2 * math.Pi / 60.0

	liftedVolume := 0.0
	if omega > 0 {
		liftedVolume = liftedVolumePerMin / 60.0 / omega
	} else {
		liftedVolume = liftedVolumePerMin / 60.0 / 0.05
	}

	eccentricTorque := p.WaterDensity * p.Gravity * liftedVolume * radius * 0.3
	centrifugalLoss := p.WaterDensity * liftedVolume * omega * omega * radius * 0.01
	return eccentricTorque + centrifugalLoss
}

func (hm *HydraulicModel) calcTheoreticalLift(wheel *models.Waterwheel, data *models.TelemetryData) float64 {
	p := hm.params
	fillEff := hm.calcDynamicFillEfficiency(wheel, data)
	volumePerRotation := float64(wheel.BucketCount) * p.ActiveBucketRatio * wheel.BucketCapacity * fillEff
	liftPerHour := volumePerRotation * data.RotationSpeed * 60.0
	return math.Min(liftPerHour, wheel.MaxFlowRate)
}

func (hm *HydraulicModel) EnrichTelemetry(wheel *models.Waterwheel, data *models.TelemetryData) {
	analysis := hm.Analyze(wheel, data)
	mech := analysis.MechanicalEfficiency
	hyd := analysis.HydraulicEfficiency
	torque := analysis.TorqueInput
	power := analysis.OutputPower

	data.MechanicalEfficiency = &mech
	data.HydraulicEfficiency = &hyd
	data.Torque = &torque
	data.PowerOutput = &power
}
