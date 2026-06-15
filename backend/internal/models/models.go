package models

import "time"

type Waterwheel struct {
	ID             int64     `json:"id"`
	Name           string    `json:"name"`
	Location       string    `json:"location"`
	Diameter       float64   `json:"diameter"`
	BucketCount    int       `json:"bucket_count"`
	BucketCapacity float64   `json:"bucket_capacity"`
	MaxFlowRate    float64   `json:"max_flow_rate"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

type TelemetryData struct {
	Time                 time.Time `json:"time"`
	WaterwheelID         int64     `json:"waterwheel_id"`
	RotationSpeed        float64   `json:"rotation_speed"`
	WaterLift            float64   `json:"water_lift"`
	WaterLevelDrop       float64   `json:"water_level_drop"`
	FlowVelocity         float64   `json:"flow_velocity"`
	MechanicalEfficiency *float64  `json:"mechanical_efficiency,omitempty"`
	HydraulicEfficiency  *float64  `json:"hydraulic_efficiency,omitempty"`
	Torque               *float64  `json:"torque,omitempty"`
	PowerOutput          *float64  `json:"power_output,omitempty"`
}

const (
	AlertTypeLowEfficiency = "low_efficiency"

	SeverityWarning  AlertSeverity = "warning"
	SeverityMajor    AlertSeverity = "major"
	SeverityCritical AlertSeverity = "critical"
)

type AlertSeverity string

type Alert struct {
	ID                   int64         `json:"id"`
	WaterwheelID         int64         `json:"waterwheel_id"`
	WaterwheelName       string        `json:"waterwheel_name,omitempty"`
	Time                 time.Time     `json:"time"`
	Type                 string        `json:"type"`
	Severity             AlertSeverity `json:"severity"`
	Message              string        `json:"message"`
	CurrentEfficiency    float64       `json:"current_efficiency"`
	HistoricalEfficiency float64       `json:"historical_efficiency"`
	Threshold            float64       `json:"threshold"`
	RotationSpeed        float64       `json:"rotation_speed"`
	WaterLift            float64       `json:"water_lift"`
	WaterLevelDrop       float64       `json:"water_level_drop"`
	FlowVelocity         float64       `json:"flow_velocity"`
	Acknowledged         bool          `json:"acknowledged"`
}

type OptimizationResult struct {
	ID                   int       `json:"id"`
	WaterwheelID         int64     `json:"waterwheel_id"`
	Time                 time.Time `json:"time"`
	OptimalBucketAngle   float64   `json:"optimal_bucket_angle"`
	OptimalDepthRatio    float64   `json:"optimal_depth_ratio"`
	OptimalWidthRatio    float64   `json:"optimal_width_ratio"`
	PredictedLift        float64   `json:"predicted_lift_lph"`
	PredictedImprovement float64   `json:"predicted_improvement_percent"`
	Fitness              float64   `json:"fitness"`
	Generations          int       `json:"generations"`
}

type MQTTConfig struct {
	BrokerURL   string
	ClientID    string
	Username    string
	Password    string
	TopicPrefix string
}

type EfficiencyAnalysis struct {
	WaterwheelID        int64     `json:"waterwheel_id"`
	Time                time.Time `json:"time"`
	RotationSpeed       float64   `json:"rotation_speed"`
	InputPower          float64   `json:"input_power"`
	OutputPower         float64   `json:"output_power"`
	TorqueInput         float64   `json:"torque_input"`
	TorqueOutput        float64   `json:"torque_output"`
	LiftResistance      float64   `json:"lift_resistance"`
	MechanicalEfficiency float64  `json:"mechanical_efficiency"`
	HydraulicEfficiency  float64  `json:"hydraulic_efficiency"`
	OverallEfficiency    float64  `json:"overall_efficiency"`
}

type BucketParams struct {
	Width        float64 `json:"width"`
	Depth        float64 `json:"depth"`
	Height       float64 `json:"height"`
	Angle        float64 `json:"angle"`
	Curvature    float64 `json:"curvature"`
}
