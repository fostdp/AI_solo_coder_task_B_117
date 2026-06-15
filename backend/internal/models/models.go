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

// ============================================================
// 新增 Feature: 灌溉田块与协同调度
// ============================================================

type IrrigationField struct {
	ID                 int64     `json:"id"`
	Name               string    `json:"name"`
	Location           string    `json:"location"`
	AreaHectare        float64   `json:"area_hectare"`
	CropType           string    `json:"crop_type"`
	DailyWaterReqM3    float64   `json:"daily_water_req_m3"`
	Priority           int       `json:"priority"`
	AssignedWaterwheel []int64   `json:"assigned_waterwheels,omitempty"`
	CurrentFilledM3    float64   `json:"current_filled_m3,omitempty"`
	CreatedAt          time.Time `json:"created_at"`
}

type ScheduleRequest struct {
	FieldID           int64  `json:"field_id"`
	TargetWaterM3     float64 `json:"target_water_m3"`
	DeadlineHours     int     `json:"deadline_hours"`
	UseWaterwheelIDs  []int64 `json:"use_waterwheel_ids,omitempty"`
	AllowElectricPump bool   `json:"allow_electric_pump"`
	ElectricityCost   float64 `json:"electricity_cost_per_kwh"`
}

type ScheduleSolution struct {
	ID                 int64            `json:"id"`
	FieldID            int64            `json:"field_id"`
	FieldName          string           `json:"field_name,omitempty"`
	Time               time.Time        `json:"time"`
	TotalWaterM3       float64          `json:"total_water_m3"`
	TotalDurationHours float64          `json:"total_duration_hours"`
	TotalCostYuan      float64          `json:"total_cost_yuan"`
	TotalEnergyKWh     float64          `json:"total_energy_kwh"`
	RenewableRatio     float64          `json:"renewable_ratio"`
	WaterwheelPlans    []WheelPlan      `json:"waterwheel_plans"`
	PumpPlan           *PumpPlan        `json:"pump_plan,omitempty"`
	Status             string           `json:"status"`
}

type WheelPlan struct {
	WaterwheelID   int64   `json:"waterwheel_id"`
	WaterwheelName string  `json:"waterwheel_name,omitempty"`
	RunHours       float64 `json:"run_hours"`
	WaterM3        float64 `json:"water_m3"`
	EnergySavedKWh float64 `json:"energy_saved_kwh"`
	CostSavedYuan  float64 `json:"cost_saved_yuan"`
	StartHour      int     `json:"start_hour"`
}

type PumpPlan struct {
	PumpType      string  `json:"pump_type"`
	RunHours      float64 `json:"run_hours"`
	WaterM3       float64 `json:"water_m3"`
	EnergyKWh     float64 `json:"energy_kwh"`
	CostYuan      float64 `json:"cost_yuan"`
	FlowRateM3H   float64 `json:"flow_rate_m3h"`
	PowerKW       float64 `json:"power_kw"`
}

// ============================================================
// 新增 Feature: 季节性水位预测与高度调节
// ============================================================

type WaterLevelForecast struct {
	ID            int64     `json:"id"`
	WaterwheelID  int64     `json:"waterwheel_id"`
	ForecastDate  time.Time `json:"forecast_date"`
	HorizonDays   int       `json:"horizon_days"`
	PredictedDrop float64   `json:"predicted_drop_m"`
	PredictedFlow float64   `json:"predicted_flow_mps"`
	LowerBound    float64   `json:"lower_bound"`
	UpperBound    float64   `json:"upper_bound"`
	Season        string    `json:"season"`
	Confidence    float64   `json:"confidence"`
	CreatedAt     time.Time `json:"created_at"`
}

type HeightAdjustment struct {
	ID                   int64     `json:"id"`
	WaterwheelID         int64     `json:"waterwheel_id"`
	ForecastID           int64     `json:"forecast_id,omitempty"`
	CurrentHeight        float64   `json:"current_height_m"`
	RecommendedHeight    float64   `json:"recommended_height_m"`
	AdjustmentCm         float64   `json:"adjustment_cm"`
	ExpectedLiftGain     float64   `json:"expected_lift_gain_percent"`
	ExpectedEffGain      float64   `json:"expected_eff_gain_percent"`
	SubmergenceBefore    float64   `json:"submergence_before"`
	SubmergenceAfter     float64   `json:"submergence_after"`
	Reason               string    `json:"reason"`
	Status               string    `json:"status"`
	ImplementedAt        *time.Time `json:"implemented_at,omitempty"`
	CreatedAt            time.Time `json:"created_at"`
}

type HistoricalHydrology struct {
	WaterwheelID int64     `json:"waterwheel_id"`
	Date         time.Time `json:"date"`
	AvgDrop      float64   `json:"avg_drop_m"`
	AvgFlow      float64   `json:"avg_flow_mps"`
	RainfallMm   float64   `json:"rainfall_mm"`
	Month        int       `json:"month"`
}

// ============================================================
// 新增 Feature: 古代筒车 vs 现代水泵 能效对比
// ============================================================

type EfficiencyComparison struct {
	ID                int64     `json:"id"`
	WaterwheelID      int64     `json:"waterwheel_id"`
	Time              time.Time `json:"time"`
	PeriodDays        int       `json:"period_days"`
	WaterwheelMetrics PumpMetrics `json:"waterwheel_metrics"`
	ModernPumpMetrics PumpMetrics `json:"modern_pump_metrics"`
	AncientAdvantage  AncientEdge `json:"ancient_advantage"`
	Scenario          string    `json:"scenario"`
}

type PumpMetrics struct {
	TotalWaterM3      float64 `json:"total_water_m3"`
	TotalEnergyKWh    float64 `json:"total_energy_kwh"`
	EnergyCostYuan    float64 `json:"energy_cost_yuan"`
	CO2EmissionKg     float64 `json:"co2_emission_kg"`
	AvgEfficiency     float64 `json:"avg_efficiency"`
	LiftHeightM       float64 `json:"lift_height_m"`
	MaintenanceCost   float64 `json:"maintenance_cost_yuan"`
	TotalCostYuan     float64 `json:"total_cost_yuan"`
	EnergySource      string  `json:"energy_source"`
}

type AncientEdge struct {
	CostSavedYuan     float64 `json:"cost_saved_yuan_per_year"`
	EnergySavedKWh    float64 `json:"energy_saved_kwh_per_year"`
	CO2SavedKg        float64 `json:"co2_saved_kg_per_year"`
	CostRatio         float64 `json:"cost_ratio_ancient_vs_modern"`
	EnergyRatio       float64 `json:"energy_ratio_ancient_vs_modern"`
	PaybackYears      float64 `json:"waterwheel_payback_years"`
	BreakEvenM3       float64 `json:"break_even_water_m3"`
}

// ============================================================
// 新增 Feature: 公众虚拟建造筒车
// ============================================================

type VirtualBuild struct {
	ID              int64     `json:"id"`
	UserID          string    `json:"user_id"`
	BuildName       string    `json:"build_name"`
	Diameter        float64   `json:"diameter_m"`
	BucketCount     int       `json:"bucket_count"`
	BucketCapacity  float64   `json:"bucket_capacity_m3"`
	SpokeCount      int       `json:"spoke_count"`
	Material        string    `json:"material"`
	WheelAngle      float64   `json:"wheel_angle_deg"`
	InstallHeight   float64   `json:"install_height_m"`
	PartsUsed       []BuildPart `json:"parts_used"`
	PredictedLift   float64   `json:"predicted_lift_m3h"`
	PredictedEff    float64   `json:"predicted_efficiency"`
	Blueprint       string    `json:"blueprint_svg,omitempty"`
	IsPublic        bool      `json:"is_public"`
	Likes           int       `json:"likes"`
	CreatedAt       time.Time `json:"created_at"`
}

type BuildPart struct {
	PartType   string  `json:"part_type"`
	Name       string  `json:"name"`
	Material   string  `json:"material"`
	Quantity   int     `json:"quantity"`
	SizeParam1 float64 `json:"size_1"`
	SizeParam2 float64 `json:"size_2,omitempty"`
	SizeParam3 float64 `json:"size_3,omitempty"`
	PosX       float64 `json:"pos_x,omitempty"`
	PosY       float64 `json:"pos_y,omitempty"`
	Rotation   float64 `json:"rotation_deg,omitempty"`
}

type BuildPreset struct {
	ID          string      `json:"id"`
	Name        string      `json:"name"`
	Culture     string      `json:"culture"`
	Era         string      `json:"era"`
	Description string      `json:"description"`
	Params      VirtualBuild `json:"params"`
	Unlocked    bool        `json:"unlocked"`
}

type BuildSimulation struct {
	FlowVelocity     float64 `json:"flow_velocity_mps"`
	WaterDrop        float64 `json:"water_drop_m"`
	Rpm              float64 `json:"rpm"`
	LiftRate         float64 `json:"lift_rate_m3h"`
	Torque           float64 `json:"torque_nm"`
	BucketFillEff    float64 `json:"bucket_fill_efficiency"`
	SubmergedBuckets int     `json:"submerged_buckets"`
	StressLevel      float64 `json:"stress_level"`
	Stable           bool    `json:"stable"`
	Warning          string  `json:"warning,omitempty"`
}
