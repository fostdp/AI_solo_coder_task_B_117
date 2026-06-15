package config

type HydraulicParams struct {
	WaterDensity      float64
	Gravity           float64
	FrictionCoeff     float64
	BearingFriction   float64
	FillTimeConstant  float64
	MaxFillEfficiency float64
	MinFillEfficiency float64
	LiftHeightRatio   float64
	ActiveBucketRatio float64
	EffectiveArmRatio float64
	ImpulseForceRatio float64
}

type OptimizerParams struct {
	PopulationSize  int
	Generations     int
	MutationRate    float64
	CrossoverRate   float64
	SurrogateK      int
	RealEvalRatio   float64
	ElitismCount    int
	ExploreRatio    float64
}

type AlarmParams struct {
	EfficiencyThreshold float64
	CooldownMinutes     int
	HistoryHours        int
}

type ReceiverParams struct {
	WorkerCount      int
	ValidateBounds   bool
	MinRotationSpeed float64
	MaxRotationSpeed float64
	MinWaterLift     float64
	MaxWaterLift     float64
	MinDrop          float64
	MaxDrop          float64
	MinFlow          float64
	MaxFlow          float64
}

type FrontendViewParams struct {
	WheelCenterXRatio    float64
	WheelCenterYRatio    float64
	WheelRadiusWRatio    float64
	WheelRadiusHRatio    float64
	WaterYRatio          float64
	BucketCount          int
	ParticleCount        int
	WorkerParticleCount  int
	AnimationSpeeds      []float64
	DefaultRangeHours    int
	ChartRanges          []int
}

type PanelParams struct {
	MetricCards          int
	AlertLimit           int
	OptHistoryLimit      int
	ChartHeight          int
	ChartPaddingLeft     int
	ChartPaddingRight    int
	ChartPaddingTop      int
	ChartPaddingBottom   int
}

func DefaultHydraulicParams() *HydraulicParams {
	return &HydraulicParams{
		WaterDensity:      1000.0,
		Gravity:           9.81,
		FrictionCoeff:     0.08,
		BearingFriction:   0.05,
		FillTimeConstant:  0.15,
		MaxFillEfficiency: 0.92,
		MinFillEfficiency: 0.15,
		LiftHeightRatio:   0.9,
		ActiveBucketRatio: 0.38,
		EffectiveArmRatio: 0.85,
		ImpulseForceRatio: 0.5,
	}
}

func DefaultOptimizerParams() *OptimizerParams {
	return &OptimizerParams{
		PopulationSize: 100,
		Generations:    150,
		MutationRate:   0.15,
		CrossoverRate:  0.85,
		SurrogateK:     7,
		RealEvalRatio:  0.3,
		ElitismCount:   5,
		ExploreRatio:   0.3,
	}
}

func DefaultAlarmParams() *AlarmParams {
	return &AlarmParams{
		EfficiencyThreshold: 0.8,
		CooldownMinutes:     60,
		HistoryHours:        168,
	}
}

func DefaultReceiverParams() *ReceiverParams {
	return &ReceiverParams{
		WorkerCount:      4,
		ValidateBounds:   true,
		MinRotationSpeed: 0.1,
		MaxRotationSpeed: 20.0,
		MinWaterLift:     0.0,
		MaxWaterLift:     500.0,
		MinDrop:          0.1,
		MaxDrop:          15.0,
		MinFlow:          0.1,
		MaxFlow:          10.0,
	}
}

func DefaultFrontendViewParams() *FrontendViewParams {
	return &FrontendViewParams{
		WheelCenterXRatio:   0.5,
		WheelCenterYRatio:   0.45,
		WheelRadiusWRatio:   0.32,
		WheelRadiusHRatio:   0.38,
		WaterYRatio:         0.75,
		BucketCount:         20,
		ParticleCount:       80,
		WorkerParticleCount: 120,
		AnimationSpeeds:     []float64{1, 2, 5, 10},
		DefaultRangeHours:   24,
		ChartRanges:         []int{1, 6, 24, 168},
	}
}

func DefaultPanelParams() *PanelParams {
	return &PanelParams{
		MetricCards:        6,
		AlertLimit:         20,
		OptHistoryLimit:    10,
		ChartHeight:        200,
		ChartPaddingLeft:   45,
		ChartPaddingRight:  15,
		ChartPaddingTop:    15,
		ChartPaddingBottom: 30,
	}
}

type SchedulerParams struct {
	PumpFlowRateM3H      float64
	PumpPowerKW          float64
	PumpEfficiency       float64
	DefaultElecCostYuan  float64
	MaxRunHoursPerWheel  float64
	MinWaterwheelRatio   float64
	CO2PerKWhKg          float64
	LPSolveIterations    int
	PenaltyPumpUsage     float64
	CanalLossRate        float64
	EnsembleMembers      int
	NormalizedEffBaseKW  float64
	SnapGridSize         float64
}

type ForecastingParams struct {
	DefaultHorizonDays    int
	SeasonalWindowYears   int
	ARWeight              float64
	SeasonWeight          float64
	TrendWeight           float64
	MinConfidence         float64
	HeightStepCm          float64
	TargetSubmergence     float64
	MaxAdjustmentCm       float64
	WarningDropRatio      float64
	EnsembleSize          int
	EnsembleNoiseScale    float64
}

type ComparisonParams struct {
	ModernPumpEfficiency    float64
	ModernPumpCostPerKW     float64
	WaterwheelBuildCostYuan float64
	WaterwheelMaintainYuan  float64
	CO2GridFactorKgPerKWh   float64
	DiscountRate            float64
	ProjectLifetimeYears    int
	DefaultCompareDays      int
	LaborCostPerDayYuan     float64
	NormalizedEffBaseKW     float64
	PumpLoadFactor          float64
}

type BuildParams struct {
	MinDiameterM        float64
	MaxDiameterM        float64
	MinBuckets          int
	MaxBuckets          int
	MinSpokes           int
	MaxSpokes           int
	MaxPartCount        int
	Materials           []string
	DefaultFlowVelocity float64
	DefaultWaterDrop    float64
	StressLimit         float64
	SnapGridSize        float64
	SnapThreshold       float64
}

func DefaultSchedulerParams() *SchedulerParams {
	return &SchedulerParams{
		PumpFlowRateM3H:     120.0,
		PumpPowerKW:         15.0,
		PumpEfficiency:      0.72,
		DefaultElecCostYuan: 0.85,
		MaxRunHoursPerWheel: 20.0,
		MinWaterwheelRatio:  0.4,
		CO2PerKWhKg:         0.785,
		LPSolveIterations:   200,
		PenaltyPumpUsage:    0.15,
		CanalLossRate:       0.12,
		EnsembleMembers:     5,
		NormalizedEffBaseKW: 10.0,
		SnapGridSize:        0.5,
	}
}

func DefaultForecastingParams() *ForecastingParams {
	return &ForecastingParams{
		DefaultHorizonDays:  30,
		SeasonalWindowYears: 3,
		ARWeight:            0.35,
		SeasonWeight:        0.45,
		TrendWeight:         0.20,
		MinConfidence:       0.65,
		HeightStepCm:        5.0,
		TargetSubmergence:   0.35,
		MaxAdjustmentCm:     80.0,
		WarningDropRatio:    0.55,
		EnsembleSize:        50,
		EnsembleNoiseScale:  0.1,
	}
}

func DefaultComparisonParams() *ComparisonParams {
	return &ComparisonParams{
		ModernPumpEfficiency:    0.72,
		ModernPumpCostPerKW:     1800.0,
		WaterwheelBuildCostYuan: 45000.0,
		WaterwheelMaintainYuan:  1200.0,
		CO2GridFactorKgPerKWh:   0.785,
		DiscountRate:            0.04,
		ProjectLifetimeYears:    30,
		DefaultCompareDays:      365,
		LaborCostPerDayYuan:     260.0,
		NormalizedEffBaseKW:     10.0,
		PumpLoadFactor:          0.75,
	}
}

func DefaultBuildParams() *BuildParams {
	return &BuildParams{
		MinDiameterM:        2.0,
		MaxDiameterM:        15.0,
		MinBuckets:          6,
		MaxBuckets:          48,
		MinSpokes:           4,
		MaxSpokes:           24,
		MaxPartCount:        200,
		Materials:           []string{"楠木", "杉木", "柏木", "松木", "竹制", "铸铁"},
		DefaultFlowVelocity: 1.5,
		DefaultWaterDrop:    2.0,
		StressLimit:         0.85,
		SnapGridSize:        0.5,
		SnapThreshold:       0.3,
	}
}
