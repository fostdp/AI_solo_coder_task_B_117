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
