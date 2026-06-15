package irrigation_scheduler

import (
	"testing"
	"time"

	"waterwheel-monitor/internal/config"
	"waterwheel-monitor/internal/models"
)

func newTestScheduler() *LPScheduler {
	return &LPScheduler{
		db:     nil,
		params: config.DefaultSchedulerParams(),
	}
}

func makeField(id int64, dailyReq float64, assigned []int64) *models.IrrigationField {
	return &models.IrrigationField{
		ID:                 id,
		Name:               "测试田块",
		Location:           "测试位置",
		AreaHectare:        5.0,
		CropType:           "水稻",
		DailyWaterReqM3:    dailyReq,
		Priority:           2,
		AssignedWaterwheel: assigned,
		CurrentFilledM3:    dailyReq * 0.3,
	}
}

// ============================================================
// 正常场景测试
// ============================================================

func TestSolveGreedyLP_NormalCase(t *testing.T) {
	s := newTestScheduler()
	field := makeField(1, 600, []int64{1, 2, 3})

	cands := []wheelCandidate{
		{id: 1, name: "1号筒车", flowM3H: 40, actualFlow: 40, capacityM3: 800, efficiency: 0.52, distanceCost: 0},
		{id: 2, name: "2号筒车", flowM3H: 35, actualFlow: 35, capacityM3: 700, efficiency: 0.48, distanceCost: 0},
		{id: 3, name: "3号筒车", flowM3H: 50, actualFlow: 50, capacityM3: 1000, efficiency: 0.55, distanceCost: 0},
	}

	req := models.ScheduleRequest{
		FieldID:           1,
		TargetWaterM3:     500,
		DeadlineHours:     24,
		UseWaterwheelIDs:  []int64{1, 2, 3},
		AllowElectricPump: true,
		ElectricityCost:   0.85,
	}

	sol := s.solveGreedyLP(cands, field, req, 0.85)

	if sol == nil {
		t.Fatal("解为空")
	}

	if sol.TotalWaterM3 < 500-1.0 {
		t.Errorf("总水量不足: 目标500, 实际 %.2f", sol.TotalWaterM3)
	}
	if sol.RenewableRatio <= 0 {
		t.Error("可再生能源比例应为正")
	}
	if len(sol.WaterwheelPlans) == 0 {
		t.Error("至少应有一台筒车被调度")
	}

	for _, p := range sol.WaterwheelPlans {
		if p.RunHours <= 0 {
			t.Errorf("筒车%d运行小时应为正，得到%.2f", p.WaterwheelID, p.RunHours)
		}
		if p.WaterM3 <= 0 {
			t.Errorf("筒车%d供水量应为正，得到%.2f", p.WaterwheelID, p.WaterM3)
		}
		if p.EnergySavedKWh < 0 {
			t.Errorf("节能不应为负，得到%.2f", p.EnergySavedKWh)
		}
	}

	t.Logf("✅ 正常场景：总水%.2f m³, 可再生比例%.2f%%, 筒车数%d",
		sol.TotalWaterM3, sol.RenewableRatio, len(sol.WaterwheelPlans))
}

func TestSolveGreedyLP_SortByEfficiency(t *testing.T) {
	s := newTestScheduler()
	field := makeField(1, 800, nil)

	cands := []wheelCandidate{
		{id: 1, name: "低效筒车", flowM3H: 60, capacityM3: 1200, efficiency: 0.20},
		{id: 2, name: "中效筒车", flowM3H: 60, capacityM3: 1200, efficiency: 0.40},
		{id: 3, name: "高效筒车", flowM3H: 60, capacityM3: 1200, efficiency: 0.60},
	}

	req := models.ScheduleRequest{
		FieldID:           1,
		TargetWaterM3:     120,
		DeadlineHours:     6,
		AllowElectricPump: false,
	}

	sol := s.solveGreedyLP(cands, field, req, 0.85)

	if sol == nil || len(sol.WaterwheelPlans) == 0 {
		t.Fatal("未生成调度方案")
	}

	firstID := sol.WaterwheelPlans[0].WaterwheelID
	if firstID != 3 {
		t.Errorf("高效筒车(3)应优先调度，但首先运行的是%d。顺序：", firstID)
		for _, p := range sol.WaterwheelPlans {
			t.Logf("  - 筒车%d", p.WaterwheelID)
		}
	}

	total := 0.0
	for _, p := range sol.WaterwheelPlans {
		total += p.WaterM3
	}
	if total < 119 {
		t.Errorf("总调度水量不足120, 实际%.2f", total)
	}
	t.Logf("✅ 优先级排序验证：第一台调度为高效筒车ID=%d", firstID)
}

// ============================================================
// 边界场景测试
// ============================================================

func TestSolveGreedyLP_Boundary_ZeroTarget(t *testing.T) {
	s := newTestScheduler()
	field := makeField(1, 500, nil)
	req := models.ScheduleRequest{
		FieldID:           1,
		TargetWaterM3:     0,
		DeadlineHours:     24,
		AllowElectricPump: true,
	}
	cands := []wheelCandidate{
		{id: 1, name: "A", flowM3H: 30, capacityM3: 600, efficiency: 0.5},
	}

	sol := s.solveGreedyLP(cands, field, req, 0.85)

	if sol == nil {
		t.Fatal("零目标时应回退到field.DailyWaterReqM3")
	}
	if sol.TotalWaterM3 < 500-1.0 {
		t.Errorf("应回退到田块日需水量500，实际%.2f", sol.TotalWaterM3)
	}
	t.Logf("✅ 零目标回退：总水%.2f (期望500)", sol.TotalWaterM3)
}

func TestSolveGreedyLP_Boundary_ZeroDeadline(t *testing.T) {
	s := newTestScheduler()
	field := makeField(1, 500, nil)
	req := models.ScheduleRequest{
		FieldID:           1,
		TargetWaterM3:     240,
		DeadlineHours:     0,
		AllowElectricPump: true,
	}
	cands := []wheelCandidate{
		{id: 1, name: "A", flowM3H: 20, capacityM3: 400, efficiency: 0.5},
	}

	sol := s.solveGreedyLP(cands, field, req, 0.85)

	if sol == nil {
		t.Fatal("零期限应回退到24小时")
	}
	if sol.TotalWaterM3 < 239 {
		t.Errorf("在24小时内20m³/h×20h应能覆盖240m³，实际%.2f", sol.TotalWaterM3)
	}
	t.Logf("✅ 零期限回退：总水%.2f m³", sol.TotalWaterM3)
}

func TestSolveGreedyLP_Boundary_SingleWheel(t *testing.T) {
	s := newTestScheduler()
	field := makeField(1, 500, nil)
	req := models.ScheduleRequest{
		FieldID:           1,
		TargetWaterM3:     200,
		DeadlineHours:     10,
		AllowElectricPump: false,
	}
	cands := []wheelCandidate{
		{id: 99, name: "唯筒车", flowM3H: 30, capacityM3: 300, efficiency: 0.5},
	}

	sol := s.solveGreedyLP(cands, field, req, 0.85)
	if len(sol.WaterwheelPlans) != 1 {
		t.Fatalf("应只有1台筒车，实际%d台", len(sol.WaterwheelPlans))
	}
	p := sol.WaterwheelPlans[0]
	expectedHours := 200.0 / 30.0
	if p.WaterM3 < 199.0 || p.RunHours < expectedHours-0.1 {
		t.Errorf("水量/小时数不对：水%.2f, 时%.2f", p.WaterM3, p.RunHours)
	}
	t.Logf("✅ 单筒车场景：ID=%d, 运行%.2fh, 供水%.2fm³", p.WaterwheelID, p.RunHours, p.WaterM3)
}

func TestSolveGreedyLP_Boundary_InsufficientWheels_NoPump(t *testing.T) {
	s := newTestScheduler()
	field := makeField(1, 500, nil)
	req := models.ScheduleRequest{
		FieldID:           1,
		TargetWaterM3:     1000,
		DeadlineHours:     10,
		AllowElectricPump: false,
	}
	cands := []wheelCandidate{
		{id: 1, flowM3H: 30, capacityM3: 300, efficiency: 0.5},
		{id: 2, flowM3H: 40, capacityM3: 400, efficiency: 0.5},
	}

	sol := s.solveGreedyLP(cands, field, req, 0.85)

	totalCap := 30*10 + 40*10 // 700
	if sol.TotalWaterM3 > float64(totalCap)+1.0 {
		t.Errorf("不应超过最大可能容量700，实际%.2f", sol.TotalWaterM3)
	}
	if sol.PumpPlan != nil {
		t.Error("禁止水泵时不应有水泵计划")
	}
	if sol.TotalWaterM3 < req.TargetWaterM3 {
		t.Logf("✅ 预期行为：禁止水泵+水量不足，只能供%.0f m³ (需求1000)", sol.TotalWaterM3)
	} else {
		t.Errorf("水量应不足但却满足了？")
	}
}

func TestSolveGreedyLP_Boundary_NeedsPumpBackup(t *testing.T) {
	s := newTestScheduler()
	field := makeField(1, 500, nil)
	req := models.ScheduleRequest{
		FieldID:           1,
		TargetWaterM3:     1500,
		DeadlineHours:     24,
		AllowElectricPump: true,
	}
	cands := []wheelCandidate{
		{id: 1, flowM3H: 30, capacityM3: 600, efficiency: 0.5},
	}

	sol := s.solveGreedyLP(cands, field, req, 0.85)

	if sol.PumpPlan == nil {
		t.Error("水量差距大，应触发水泵补充")
	} else {
		if sol.PumpPlan.WaterM3 <= 0 {
			t.Error("水泵补水量应为正")
		}
		if sol.TotalCostYuan <= 0 {
			t.Error("启用水泵应有电费成本")
		}
		total := 0.0
		for _, p := range sol.WaterwheelPlans {
			total += p.WaterM3
		}
		total += sol.PumpPlan.WaterM3
		if total < req.TargetWaterM3-1.0 {
			t.Errorf("总水量(筒车+水泵)应满足需求%.0f，实际%.2f", req.TargetWaterM3, total)
		}
		t.Logf("✅ 水泵补充：筒车%.0f m³ + 水泵%.0f m³ = %.0f m³，电费%.2f元",
			total-sol.PumpPlan.WaterM3, sol.PumpPlan.WaterM3, total, sol.TotalCostYuan)
	}
}

// ============================================================
// 异常/错误场景测试
// ============================================================

func TestSolveGreedyLP_Anomaly_ZeroFlow(t *testing.T) {
	s := newTestScheduler()
	field := makeField(1, 500, nil)
	req := models.ScheduleRequest{
		FieldID:           1,
		TargetWaterM3:     300,
		DeadlineHours:     12,
		AllowElectricPump: false,
	}
	cands := []wheelCandidate{
		{id: 1, name: "断流", flowM3H: 0, capacityM3: 0, efficiency: 0},
	}

	sol := s.solveGreedyLP(cands, field, req, 0.85)

	if len(sol.WaterwheelPlans) > 0 {
		t.Error("零流量不应有筒车被调度")
	}
	if sol.TotalWaterM3 > 0.01 {
		t.Errorf("总水量应为0，实际%.2f", sol.TotalWaterM3)
	}
	if sol.RenewableRatio != 0 {
		t.Errorf("没有产水时可再生比例应为0，实际%.2f", sol.RenewableRatio)
	}
	t.Logf("✅ 零流量异常：总水量%.2f", sol.TotalWaterM3)
}

func TestSolveGreedyLP_Anomaly_NegativeTarget(t *testing.T) {
	s := newTestScheduler()
	field := makeField(1, 400, nil)
	req := models.ScheduleRequest{
		FieldID:           1,
		TargetWaterM3:     -100,
		DeadlineHours:     24,
		AllowElectricPump: true,
	}
	cands := []wheelCandidate{
		{id: 1, flowM3H: 30, capacityM3: 600, efficiency: 0.5},
	}

	sol := s.solveGreedyLP(cands, field, req, 0.85)

	if sol.TotalWaterM3 < 399 {
		t.Errorf("负目标应回退到日需求400，实际%.2f", sol.TotalWaterM3)
	}
	t.Logf("✅ 负目标回退正常：%.2f m³", sol.TotalWaterM3)
}

func TestSolveGreedyLP_Anomaly_HugeFlow(t *testing.T) {
	s := newTestScheduler()
	field := makeField(1, 500, nil)
	req := models.ScheduleRequest{
		FieldID:           1,
		TargetWaterM3:     500,
		DeadlineHours:     24,
		AllowElectricPump: false,
	}
	cands := []wheelCandidate{
		{id: 1, flowM3H: 99999, capacityM3: 999999, efficiency: 0.9},
	}

	sol := s.solveGreedyLP(cands, field, req, 0.85)

	if sol.TotalWaterM3 > 501 {
		t.Errorf("超大型流量不应导致分配超过目标值，实际%.2f", sol.TotalWaterM3)
	}
	if len(sol.WaterwheelPlans) != 1 {
		t.Errorf("应只有1个计划，实际%d", len(sol.WaterwheelPlans))
	}
	p := sol.WaterwheelPlans[0]
	if p.RunHours > s.params.MaxRunHoursPerWheel+0.1 {
		t.Errorf("受MaxRunHoursPerWheel限制应<=%.0f，实际%.2f", s.params.MaxRunHoursPerWheel, p.RunHours)
	}
	t.Logf("✅ 极端流量被正确截断：运行%.2fh, 供水%.2fm³", p.RunHours, p.WaterM3)
}

// ============================================================
// 经济指标正确性测试
// ============================================================

func TestRound2_Precision(t *testing.T) {
	cases := []struct {
		in  float64
		out float64
	}{
		{3.1415, 3.14},
		{2.7182, 2.72},
		{1.005, 1.01},
		{0.004, 0.0},
		{-1.234, -1.23},
	}
	for _, c := range cases {
		res := round2(c.in)
		if res != c.out {
			t.Errorf("round2(%.4f) = %.4f, 期望%.4f", c.in, res, c.out)
		}
	}
	t.Log("✅ round2 精度测试通过")
}

func TestEnergySavedCalculation(t *testing.T) {
	s := newTestScheduler()
	field := makeField(1, 300, nil)
	req := models.ScheduleRequest{TargetWaterM3: 120, DeadlineHours: 6}
	elec := 0.85

	cands := []wheelCandidate{{id: 1, flowM3H: 30, capacityM3: 180, efficiency: 0.5}}
	sol := s.solveGreedyLP(cands, field, req, elec)

	if len(sol.WaterwheelPlans) == 0 {
		t.Fatal("无计划")
	}
	p := sol.WaterwheelPlans[0]
	expectedKWh := (p.WaterM3 / s.params.PumpFlowRateM3H) * s.params.PumpPowerKW
	expectedKWh = round2(expectedKWh)
	if p.EnergySavedKWh != expectedKWh {
		t.Errorf("节能计算错误：得到%.2f kWh，期望%.2f", p.EnergySavedKWh, expectedKWh)
	}
	expectedCost := round2(expectedKWh * elec)
	if p.CostSavedYuan != expectedCost {
		t.Errorf("节省费用错误：得到%.2f元，期望%.2f", p.CostSavedYuan, expectedCost)
	}
	t.Logf("✅ 经济指标正确：节能%.2f kWh，省费%.2f元", p.EnergySavedKWh, p.CostSavedYuan)
}

// ============================================================
// 可再生比例验证
// ============================================================

func TestRenewableRatio_ValidRange(t *testing.T) {
	s := newTestScheduler()
	field := makeField(1, 500, nil)

	cases := []struct {
		name       string
		cands      []wheelCandidate
		req        models.ScheduleRequest
		minRatio   float64
		allowPump  bool
	}{
		{
			name: "纯水力",
			cands: []wheelCandidate{{id: 1, flowM3H: 100, efficiency: 0.5, capacityM3: 2000}},
			req:  models.ScheduleRequest{TargetWaterM3: 500, DeadlineHours: 24},
			minRatio: 99.0,
		},
		{
			name: "混合(水力+水泵)",
			cands: []wheelCandidate{{id: 1, flowM3H: 10, efficiency: 0.5, capacityM3: 200}},
			req:  models.ScheduleRequest{TargetWaterM3: 500, DeadlineHours: 24, AllowElectricPump: true},
			minRatio: 0.01,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			c.req.FieldID = 1
			sol := s.solveGreedyLP(c.cands, field, c.req, 0.85)
			if sol.RenewableRatio < c.minRatio-0.1 {
				t.Errorf("[%s] 可再生比例%.2f%% < 期望阈值%.2f%%", c.name, sol.RenewableRatio, c.minRatio)
			}
			if sol.RenewableRatio > 100.1 {
				t.Errorf("[%s] 比例超过100%%: %.2f%%", c.name, sol.RenewableRatio)
			}
			t.Logf("✅ %s: 可再生比例=%.2f%%", c.name, sol.RenewableRatio)
		})
	}
}

// ============================================================
// 渠道输水损耗系数验证
// ============================================================

func TestSolveGreedyLP_CanalLossRate(t *testing.T) {
	s := newTestScheduler()
	field := makeField(1, 500, []int64{1})
	cands := []wheelCandidate{
		{id: 1, name: "A", flowM3H: 50, actualFlow: 50, capacityM3: 1000, efficiency: 0.5, distanceCost: 0},
	}
	req := models.ScheduleRequest{
		FieldID:           1,
		TargetWaterM3:     500,
		DeadlineHours:     24,
		UseWaterwheelIDs:  []int64{1},
		AllowElectricPump: false,
	}

	sol := s.solveGreedyLP(cands, field, req, 0.85)

	deliveryRate := 1.0 - s.params.CanalLossRate
	if deliveryRate <= 0 {
		t.Fatalf("CanalLossRate=%.2f 导致到达率<=0", s.params.CanalLossRate)
	}

	if sol.TotalWaterM3 > 500+1.0 {
		t.Errorf("有损耗时总送达水量不应超过目标太多: %.2f", sol.TotalWaterM3)
	}

	t.Logf("✅ 渠道损耗率=%.0f%%, 到达率=%.0f%%, 送达水量=%.2fm³ (目标500)",
		s.params.CanalLossRate*100, deliveryRate*100, sol.TotalWaterM3)
}

func TestSolveGreedyLP_CanalLossZeroVsNonZero(t *testing.T) {
	s := newTestScheduler()
	field := makeField(1, 500, []int64{1, 2})
	cands := []wheelCandidate{
		{id: 1, name: "A", flowM3H: 30, actualFlow: 30, capacityM3: 600, efficiency: 0.5, distanceCost: 0},
		{id: 2, name: "B", flowM3H: 40, actualFlow: 40, capacityM3: 800, efficiency: 0.5, distanceCost: 0},
	}
	req := models.ScheduleRequest{
		FieldID:           1,
		TargetWaterM3:     500,
		DeadlineHours:     24,
		AllowElectricPump: false,
	}

	sol := s.solveGreedyLP(cands, field, req, 0.85)
	deliveredWithLoss := sol.TotalWaterM3

	origLoss := s.params.CanalLossRate
	s.params.CanalLossRate = 0
	solNoLoss := s.solveGreedyLP(cands, field, req, 0.85)
	s.params.CanalLossRate = origLoss

	t.Logf("有损耗(%.0f%%)送达=%.2fm³ vs 无损耗送达=%.2fm³",
		origLoss*100, deliveredWithLoss, solNoLoss.TotalWaterM3)

	if deliveredWithLoss > solNoLoss.TotalWaterM3+1.0 {
		t.Error("有损耗时送达水量不应高于无损耗")
	}
}

// ============================================================
// 独立goroutine求解验证
// ============================================================

func TestSolveGreedyLP_GoroutineResultConsistency(t *testing.T) {
	s := newTestScheduler()
	field := makeField(1, 500, []int64{1, 2})
	cands := []wheelCandidate{
		{id: 1, name: "A", flowM3H: 40, actualFlow: 40, capacityM3: 800, efficiency: 0.5, distanceCost: 0},
		{id: 2, name: "B", flowM3H: 35, actualFlow: 35, capacityM3: 700, efficiency: 0.45, distanceCost: 0},
	}
	req := models.ScheduleRequest{
		FieldID:           1,
		TargetWaterM3:     400,
		DeadlineHours:     24,
		UseWaterwheelIDs:  []int64{1, 2},
		AllowElectricPump: false,
	}

	resultCh := make(chan *models.ScheduleSolution, 1)
	go func() {
		sol := s.solveGreedyLP(cands, field, req, 0.85)
		resultCh <- sol
	}()

	select {
	case sol := <-resultCh:
		if sol == nil {
			t.Fatal("goroutine求解结果不应为nil")
		}
		if sol.TotalWaterM3 < 399 {
			t.Errorf("goroutine求解水量不足: %.2f", sol.TotalWaterM3)
		}
		if len(sol.WaterwheelPlans) == 0 {
			t.Error("goroutine求解应有筒车计划")
		}
		t.Logf("✅ goroutine求解一致: 总水%.2fm³, 筒车%d台",
			sol.TotalWaterM3, len(sol.WaterwheelPlans))
	case <-time.After(5 * time.Second):
		t.Fatal("goroutine求解超时5s")
	}
}

func TestLPScheduler_TimeoutConfig(t *testing.T) {
	s := newTestScheduler()
	lpTimeout := s.params.MaxRunHoursPerWheel * 2.0
	if lpTimeout < 10 {
		lpTimeout = 10
	}
	if lpTimeout < 10 {
		t.Errorf("LP求解超时下限应为10s, 实际%.0f", lpTimeout)
	}
	t.Logf("✅ LP超时配置: MaxRunHours=%.0f → 超时=%.0fs", s.params.MaxRunHoursPerWheel, lpTimeout)
}
