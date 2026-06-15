package scheduler

import (
	"context"
	"fmt"
	"math"
	"sort"
	"time"

	"waterwheel-monitor/internal/config"
	"waterwheel-monitor/internal/database"
	"waterwheel-monitor/internal/models"
)

type LPScheduler struct {
	db     *database.Database
	params *config.SchedulerParams
}

func NewLPScheduler(db *database.Database, params *config.SchedulerParams) *LPScheduler {
	return &LPScheduler{db: db, params: params}
}

type wheelCandidate struct {
	id           int64
	name         string
	flowM3H      float64
	actualFlow   float64
	capacityM3   float64
	efficiency   float64
	distanceCost float64
}

func (s *LPScheduler) ScheduleIrrigation(ctx context.Context, req models.ScheduleRequest) (*models.ScheduleSolution, error) {
	field, err := s.db.GetIrrigationField(ctx, req.FieldID)
	if err != nil {
		return nil, fmt.Errorf("田块不存在: %w", err)
	}

	useIDs := req.UseWaterwheelIDs
	if len(useIDs) == 0 && len(field.AssignedWaterwheel) > 0 {
		useIDs = field.AssignedWaterwheel
	}
	if len(useIDs) == 0 {
		return nil, fmt.Errorf("未指定可用筒车")
	}

	elecCost := req.ElectricityCost
	if elecCost <= 0 {
		elecCost = s.params.DefaultElecCostYuan
	}

	candidates, err := s.buildCandidates(ctx, useIDs, req.DeadlineHours)
	if err != nil {
		return nil, err
	}
	if len(candidates) == 0 {
		return nil, fmt.Errorf("无可用筒车")
	}

	solution := s.solveGreedyLP(candidates, field, req, elecCost)
	solution.FieldName = field.Name
	solution.Time = time.Now()

	solID, err := s.db.SaveScheduleSolution(ctx, solution)
	if err == nil {
		solution.ID = solID
	}

	return solution, nil
}

func (s *LPScheduler) buildCandidates(ctx context.Context, ids []int64, deadlineHours int) ([]wheelCandidate, error) {
	candidates := make([]wheelCandidate, 0, len(ids))
	for _, wid := range ids {
		wheel, err := s.db.GetWaterwheelByID(ctx, wid)
		if err != nil {
			continue
		}
		tel, err := s.db.GetLatestTelemetry(ctx, wid)
		flow := wheel.MaxFlowRate * 0.75
		eff := 0.40
		if err == nil && tel != nil {
			flow = tel.WaterLift
			if tel.MechanicalEfficiency != nil && tel.HydraulicEfficiency != nil {
				eff = *tel.MechanicalEfficiency * *tel.HydraulicEfficiency
			}
		}
		if flow <= 0 {
			flow = wheel.MaxFlowRate * 0.75
		}
		maxRun := s.params.MaxRunHoursPerWheel
		if float64(deadlineHours) < maxRun {
			maxRun = float64(deadlineHours)
		}
		candidates = append(candidates, wheelCandidate{
			id:           wheel.ID,
			name:         wheel.Name,
			flowM3H:      flow,
			actualFlow:   flow,
			capacityM3:   flow * maxRun,
			efficiency:   eff,
			distanceCost: 0,
		})
	}
	return candidates, nil
}

type lpDecision struct {
	wheelID   int64
	hours     float64
	waterM3   float64
	usePump   bool
	pumpHours float64
	pumpM3    float64
}

func (s *LPScheduler) solveGreedyLP(cands []wheelCandidate, field *models.IrrigationField, req models.ScheduleRequest, elecCost float64) *models.ScheduleSolution {
	target := req.TargetWaterM3
	if target <= 0 {
		target = field.DailyWaterReqM3
	}
	deadline := float64(req.DeadlineHours)
	if deadline <= 0 {
		deadline = 24
	}

	sort.Slice(cands, func(i, j int) bool {
		iScore := cands[i].efficiency*(1-s.params.PenaltyPumpUsage) - cands[i].distanceCost*0.01
		jScore := cands[j].efficiency*(1-s.params.PenaltyPumpUsage) - cands[j].distanceCost*0.01
		return iScore > jScore
	})

	remaining := target
	totalWheelM3 := 0.0
	plans := make([]models.WheelPlan, 0, len(cands))
	startHour := 0

	deliveryRate := 1.0 - s.params.CanalLossRate
	if deliveryRate <= 0 {
		deliveryRate = 0.7
	}

	for _, c := range cands {
		if remaining <= 0 {
			break
		}
		maxHours := math.Min(s.params.MaxRunHoursPerWheel, deadline)
		maxWater := c.flowM3H * maxHours
		assignM3 := math.Min(remaining/deliveryRate, maxWater)
		deliveredM3 := assignM3 * deliveryRate
		assignHours := assignM3 / c.flowM3H
		if assignM3 <= 0 {
			continue
		}

		eqPumpKWh := (deliveredM3 / s.params.PumpFlowRateM3H) * s.params.PumpPowerKW
		wheelPlan := models.WheelPlan{
			WaterwheelID:   c.id,
			WaterwheelName: c.name,
			RunHours:       round2(assignHours),
			WaterM3:        round2(deliveredM3),
			EnergySavedKWh: round2(eqPumpKWh),
			CostSavedYuan:  round2(eqPumpKWh * elecCost),
			StartHour:      startHour,
		}
		plans = append(plans, wheelPlan)
		totalWheelM3 += deliveredM3
		remaining -= deliveredM3
		startHour += int(math.Ceil(assignHours))
	}

	var pumpPlan *models.PumpPlan
	totalPumpM3 := 0.0
	if remaining > 0 && req.AllowElectricPump {
		pumpHours := remaining / s.params.PumpFlowRateM3H
		pumpKWh := pumpHours * s.params.PumpPowerKW
		totalPumpM3 = remaining
		pumpPlan = &models.PumpPlan{
			PumpType:    "离心泵",
			RunHours:    round2(pumpHours),
			WaterM3:     round2(remaining),
			EnergyKWh:   round2(pumpKWh),
			CostYuan:    round2(pumpKWh * elecCost),
			FlowRateM3H: s.params.PumpFlowRateM3H,
			PowerKW:     s.params.PumpPowerKW,
		}
		remaining = 0
	}

	achieved := target - remaining
	renewableRatio := 0.0
	if totalWheelM3+totalPumpM3 > 0 {
		renewableRatio = totalWheelM3 / (totalWheelM3 + totalPumpM3)
	}

	totalDuration := 0.0
	for _, p := range plans {
		if p.StartHour+int(math.Ceil(p.RunHours)) > int(totalDuration) {
			totalDuration = float64(p.StartHour) + p.RunHours
		}
	}
	if pumpPlan != nil {
		pumpDuration := totalDuration + pumpPlan.RunHours
		if pumpDuration > totalDuration {
			totalDuration = pumpDuration
		}
	}
	totalCost := 0.0
	totalEnergy := 0.0
	if pumpPlan != nil {
		totalCost = pumpPlan.CostYuan
		totalEnergy = pumpPlan.EnergyKWh
	}

	return &models.ScheduleSolution{
		FieldID:            field.ID,
		TotalWaterM3:       round2(achieved),
		TotalDurationHours: round2(totalDuration),
		TotalCostYuan:      round2(totalCost),
		TotalEnergyKWh:     round2(totalEnergy),
		RenewableRatio:     round2(renewableRatio * 100),
		WaterwheelPlans:    plans,
		PumpPlan:           pumpPlan,
		Status:             "optimized",
	}
}

func (s *LPScheduler) ListFields(ctx context.Context) ([]models.IrrigationField, error) {
	return s.db.ListIrrigationFields(ctx)
}

func (s *LPScheduler) ListSolutions(ctx context.Context, fieldID int64, limit int) ([]models.ScheduleSolution, error) {
	if limit <= 0 {
		limit = 20
	}
	return s.db.ListScheduleSolutions(ctx, fieldID, limit)
}

func round2(v float64) float64 {
	return math.Round(v*100) / 100
}
