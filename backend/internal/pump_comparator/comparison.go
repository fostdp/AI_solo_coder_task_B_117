package pump_comparator

import (
	"context"
	"fmt"
	"math"
	"time"

	"waterwheel-monitor/internal/config"
	"waterwheel-monitor/internal/database"
	"waterwheel-monitor/internal/models"
)

type AncientsVsModern struct {
	db     *database.Database
	params *config.ComparisonParams
}

func NewAncientsVsModern(db *database.Database, params *config.ComparisonParams) *AncientsVsModern {
	return &AncientsVsModern{db: db, params: params}
}

func (c *AncientsVsModern) CompareWaterwheelPump(ctx context.Context, wheelID int64, periodDays int, scenario string) (*models.EfficiencyComparison, error) {
	if periodDays <= 0 {
		periodDays = c.params.DefaultCompareDays
	}
	if scenario == "" {
		scenario = "standard"
	}
	wheel, err := c.db.GetWaterwheelByID(ctx, wheelID)
	if err != nil {
		return nil, fmt.Errorf("筒车不存在: %w", err)
	}

	telRange, err := c.db.GetTelemetryRange(ctx, wheelID, time.Now().AddDate(0, 0, -periodDays), time.Now())
	if err != nil || len(telRange) == 0 {
		return c.generateEstimatedComparison(ctx, wheel, periodDays, scenario)
	}

	avgFlow := 0.0
	avgSpeed := 0.0
	avgMech := 0.0
	avgHydro := 0.0
	avgDrop := 0.0
	count := 0.0
	for _, t := range telRange {
		avgFlow += t.WaterLift
		avgSpeed += t.RotationSpeed
		avgDrop += t.WaterLevelDrop
		if t.MechanicalEfficiency != nil {
			avgMech += *t.MechanicalEfficiency
			count++
		}
		if t.HydraulicEfficiency != nil {
			avgHydro += *t.HydraulicEfficiency
		}
	}
	n := float64(len(telRange))
	avgFlow /= n
	avgSpeed /= n
	avgDrop /= n
	if count > 0 {
		avgMech /= count
		avgHydro /= count
	} else {
		avgMech = 0.55
		avgHydro = 0.70
	}

	return c.buildComparison(ctx, wheel, avgFlow, avgDrop, avgMech, avgHydro, avgSpeed, periodDays, scenario)
}

func (c *AncientsVsModern) generateEstimatedComparison(ctx context.Context, wheel *models.Waterwheel, periodDays int, scenario string) (*models.EfficiencyComparison, error) {
	estFlow := wheel.MaxFlowRate * 0.7
	estDrop := wheel.Diameter * 0.35
	estMech := 0.55
	estHydro := 0.70
	estSpeed := 6.0
	return c.buildComparison(ctx, wheel, estFlow, estDrop, estMech, estHydro, estSpeed, periodDays, scenario+"_estimated")
}

func (c *AncientsVsModern) buildComparison(ctx context.Context, wheel *models.Waterwheel, avgFlowM3H, avgDropM, avgMech, avgHydro, avgRpm float64, periodDays int, scenario string) (*models.EfficiencyComparison, error) {
	totalHours := float64(periodDays) * 24
	utilization := 0.78
	runHours := totalHours * utilization
	_ = avgRpm

	totalWaterM3 := avgFlowM3H * runHours
	overallEff := avgMech * avgHydro

	liftHeightM := avgDropM * 0.9
	if liftHeightM < 1.0 {
		liftHeightM = 1.0
	}

	rho := 1000.0
	g := 9.81
	wheelsUsefulPowerKW := (rho * g * totalWaterM3 * liftHeightM) / (1000 * 3600 * runHours) * 0.001 * 0
	_ = wheelsUsefulPowerKW

	wheelsGravInputJ := rho * g * totalWaterM3 * liftHeightM
	wheelsTotalInputKWh := wheelsGravInputJ / 3.6e6 / overallEff

	wheelsMaintenance := (float64(periodDays) / 365.0) * c.params.WaterwheelMaintainYuan
	wheelsLabor := 0.0
	if scenario == "with_labor" {
		days := float64(periodDays) / 7.0
		wheelsLabor = days * c.params.LaborCostPerDayYuan
	}

	waterwheelMetrics := models.PumpMetrics{
		TotalWaterM3:    round2(totalWaterM3),
		TotalEnergyKWh:  round2(wheelsTotalInputKWh),
		EnergyCostYuan:  0,
		CO2EmissionKg:   0,
		AvgEfficiency:   round3(overallEff),
		LiftHeightM:     round2(liftHeightM),
		MaintenanceCost: round2(wheelsMaintenance),
		TotalCostYuan:   round2(wheelsMaintenance + wheelsLabor),
		EnergySource:    "水力/可再生",
	}

	pumpEff := c.params.ModernPumpEfficiency
	pumpLoadFactor := c.params.PumpLoadFactor
	if pumpLoadFactor <= 0 {
		pumpLoadFactor = 0.75
	}
	effectivePumpEff := pumpEff * (0.3 + 0.7*pumpLoadFactor)
	pumpHydraulicPowerKW := (rho * g * avgFlowM3H * liftHeightM) / 3.6e6
	pumpShaftPowerKW := pumpHydraulicPowerKW / effectivePumpEff
	if pumpShaftPowerKW < 1.0 {
		pumpShaftPowerKW = 1.0
	}
	pumpTotalEnergyKWh := pumpShaftPowerKW * runHours * pumpLoadFactor
	elecCostYuan := pumpTotalEnergyKWh * c.params.DefaultElecCostYuan
	pumpCO2Kg := pumpTotalEnergyKWh * c.params.CO2GridFactorKgPerKWh

	pumpBuildCost := pumpShaftPowerKW * c.params.ModernPumpCostPerKW
	pumpMaintenance := pumpBuildCost * (0.03 * float64(periodDays) / 365.0)

	modernPumpMetrics := models.PumpMetrics{
		TotalWaterM3:    round2(totalWaterM3),
		TotalEnergyKWh:  round2(pumpTotalEnergyKWh),
		EnergyCostYuan:  round2(elecCostYuan),
		CO2EmissionKg:   round2(pumpCO2Kg),
		AvgEfficiency:   round3(pumpEff),
		LiftHeightM:     round2(liftHeightM),
		MaintenanceCost: round2(pumpMaintenance),
		TotalCostYuan:   round2(elecCostYuan + pumpMaintenance),
		EnergySource:    "电网电力",
	}

	yearScale := 365.0 / float64(periodDays)
	annualCostSaved := (modernPumpMetrics.TotalCostYuan - waterwheelMetrics.TotalCostYuan) * yearScale
	annualEnergySaved := modernPumpMetrics.TotalEnergyKWh * yearScale
	annualCO2Saved := modernPumpMetrics.CO2EmissionKg * yearScale

	costRatio := 0.0
	if modernPumpMetrics.TotalCostYuan > 0 {
		costRatio = waterwheelMetrics.TotalCostYuan / modernPumpMetrics.TotalCostYuan
	}
	energyRatio := 0.0
	if modernPumpMetrics.TotalEnergyKWh > 0 {
		energyRatio = waterwheelMetrics.TotalEnergyKWh / modernPumpMetrics.TotalEnergyKWh
	}

	paybackYears := 0.0
	if annualCostSaved > 0 {
		paybackYears = c.params.WaterwheelBuildCostYuan / annualCostSaved
	}
	breakEvenM3 := 0.0
	m3CostPump := 0.0
	if totalWaterM3 > 0 && modernPumpMetrics.TotalCostYuan > 0 {
		m3CostPump = modernPumpMetrics.TotalCostYuan / totalWaterM3
	}
	m3CostWheel := 0.0
	if totalWaterM3 > 0 {
		m3CostWheel = (waterwheelMetrics.TotalCostYuan + c.params.WaterwheelBuildCostYuan/float64(c.params.ProjectLifetimeYears)*float64(periodDays)/365.0) / totalWaterM3
	}
	if m3CostPump > m3CostWheel && m3CostPump > 0 {
		perM3Saved := m3CostPump - m3CostWheel
		if perM3Saved > 0 {
			breakEvenM3 = c.params.WaterwheelBuildCostYuan / perM3Saved
		}
	}

	normalizedEffRatio := 0.0
	baseKW := c.params.NormalizedEffBaseKW
	if baseKW <= 0 {
		baseKW = 10.0
	}
	wheelNormEff := 0.0
	pumpNormEff := 0.0
	if waterwheelMetrics.TotalEnergyKWh > 0 && totalWaterM3 > 0 {
		wheelNormEff = (totalWaterM3 * liftHeightM * rho * g / 3.6e6) / waterwheelMetrics.TotalEnergyKWh
	}
	if pumpTotalEnergyKWh > 0 && totalWaterM3 > 0 {
		pumpNormEff = (totalWaterM3 * liftHeightM * rho * g / 3.6e6) / pumpTotalEnergyKWh
	}
	if pumpNormEff > 0 {
		normalizedEffRatio = round3(wheelNormEff / pumpNormEff)
	}

	adv := models.AncientEdge{
		CostSavedYuan:      round2(annualCostSaved),
		EnergySavedKWh:     round2(annualEnergySaved),
		CO2SavedKg:         round2(annualCO2Saved),
		CostRatio:          round3(costRatio),
		EnergyRatio:        round3(energyRatio),
		PaybackYears:       round2(paybackYears),
		BreakEvenM3:        round2(breakEvenM3),
		NormalizedEffRatio: normalizedEffRatio,
		PumpLoadFactor:     round3(pumpLoadFactor),
	}

	comp := &models.EfficiencyComparison{
		WaterwheelID:      wheel.ID,
		Time:              time.Now(),
		PeriodDays:        periodDays,
		WaterwheelMetrics: waterwheelMetrics,
		ModernPumpMetrics: modernPumpMetrics,
		AncientAdvantage:  adv,
		Scenario:          scenario,
	}

	if c.db != nil {
		id, err := c.db.SaveEfficiencyComparison(ctx, comp)
		if err == nil {
			comp.ID = id
		}
	}
	return comp, nil
}

func (c *AncientsVsModern) ListComparisons(ctx context.Context, wheelID int64, limit int) ([]models.EfficiencyComparison, error) {
	if limit <= 0 {
		limit = 20
	}
	return c.db.ListEfficiencyComparisons(ctx, wheelID, limit)
}

func (c *AncientsVsModern) GetBuildPresets() []models.BuildPreset {
	return []models.BuildPreset{
		{
			ID:          "dujiangyan",
			Name:        "都江堰经典24斗",
			Culture:     "川西古堰文明",
			Era:         "宋代定型",
			Description: "都江堰灌溉区标准形制，楠木打造，24个水斗对应当地24节气，直径8.5米，适合大中型灌溉。",
			Params: models.VirtualBuild{
				BuildName:      "都江堰经典24斗筒车",
				Diameter:       8.5,
				BucketCount:    24,
				BucketCapacity: 0.08,
				SpokeCount:     12,
				Material:       "楠木",
				WheelAngle:     0,
				InstallHeight:  3.8,
				PredictedLift:  120.0,
				PredictedEff:   0.42,
			},
			Unlocked: true,
		},
		{
			ID:          "fenghuang",
			Name:        "凤凰沱江巨型28斗",
			Culture:     "湘西土家族水文化",
			Era:         "清代扩建",
			Description: "湘西凤凰古城沱江边的大型筒车，杉木榫卯结构，直径超9米，为土家族标志性水利设施。",
			Params: models.VirtualBuild{
				BuildName:      "凤凰沱江巨型28斗",
				Diameter:       9.1,
				BucketCount:    28,
				BucketCapacity: 0.10,
				SpokeCount:     16,
				Material:       "杉木",
				WheelAngle:     0,
				InstallHeight:  4.1,
				PredictedLift:  142.0,
				PredictedEff:   0.38,
			},
			Unlocked: true,
		},
		{
			ID:          "lijiang",
			Name:        "丽江小型18斗轻便型",
			Culture:     "纳西族东巴文化",
			Era:         "明代",
			Description: "云南丽江黑龙潭旁的小型筒车，柏木精制，便于拆装迁移，适合村落级小规模灌溉。",
			Params: models.VirtualBuild{
				BuildName:      "丽江小型18斗轻便型",
				Diameter:       6.8,
				BucketCount:    18,
				BucketCapacity: 0.05,
				SpokeCount:     10,
				Material:       "柏木",
				WheelAngle:     0,
				InstallHeight:  3.0,
				PredictedLift:  82.0,
				PredictedEff:   0.45,
			},
			Unlocked: true,
		},
		{
			ID:          "zhuzhi",
			Name:        "竹制民俗12斗",
			Culture:     "岭南民俗文化",
			Era:         "近现代传承",
			Description: "广西、广东乡村常见的竹制民俗筒车，以竹篾编织斗体，造价低廉，兼具景观与实用价值。",
			Params: models.VirtualBuild{
				BuildName:      "竹制民俗12斗筒车",
				Diameter:       5.5,
				BucketCount:    12,
				BucketCapacity: 0.04,
				SpokeCount:     8,
				Material:       "竹制",
				WheelAngle:     0,
				InstallHeight:  2.5,
				PredictedLift:  45.0,
				PredictedEff:   0.36,
			},
			Unlocked: true,
		},
		{
			ID:          "zhutie",
			Name:        "铸铁工业型32斗",
			Culture:     "近代工业改良",
			Era:         "民国时期",
			Description: "受西方机械影响出现的铸铁筒车，坚固耐用，斗数多达32个，是近代水利改良的尝试。",
			Params: models.VirtualBuild{
				BuildName:      "铸铁工业型32斗",
				Diameter:       10.2,
				BucketCount:    32,
				BucketCapacity: 0.12,
				SpokeCount:     16,
				Material:       "铸铁",
				WheelAngle:     0,
				InstallHeight:  4.6,
				PredictedLift:  175.0,
				PredictedEff:   0.50,
			},
			Unlocked: true,
		},
	}
}

func round2(v float64) float64 { return math.Round(v*100) / 100 }
func round3(v float64) float64 { return math.Round(v*1000) / 1000 }
