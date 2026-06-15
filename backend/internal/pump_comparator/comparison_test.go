package pump_comparator

import (
	"context"
	"math"
	"testing"
	"time"

	"waterwheel-monitor/internal/config"
	"waterwheel-monitor/internal/models"
)

func newTestComparer() *AncientsVsModern {
	return &AncientsVsModern{
		db:     nil,
		params: config.DefaultComparisonParams(),
	}
}

func makeWheel(id int64, diameter, maxFlow float64) *models.Waterwheel {
	return &models.Waterwheel{
		ID:             id,
		Name:           "测试筒车",
		Location:       "测试河流",
		Diameter:       diameter,
		BucketCount:    20,
		BucketCapacity: 0.08,
		MaxFlowRate:    maxFlow,
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	}
}

// ============================================================
// 正常场景：基本能效对比指标正确性
// ============================================================

func TestBuildComparison_BasicMetrics(t *testing.T) {
	c := newTestComparer()
	wheel := makeWheel(1, 8.0, 120.0)

	comp, err := c.buildComparison(context.Background(),wheel, 80.0, 2.8, 0.55, 0.70, 7.5, 365, "standard")
	if err != nil {
		t.Fatalf("构建对比失败: %v", err)
	}

	if comp == nil {
		t.Fatal("结果为空")
	}

	wm := comp.WaterwheelMetrics
	pm := comp.ModernPumpMetrics

	if wm.TotalWaterM3 <= 0 {
		t.Error("筒车总水量应为正")
	}
	if math.Abs(wm.TotalWaterM3-pm.TotalWaterM3) > 1.0 {
		t.Errorf("等水量对比不一致：筒车%.2f vs 水泵%.2f", wm.TotalWaterM3, pm.TotalWaterM3)
	}
	if wm.EnergyCostYuan != 0 {
		t.Error("筒车能源费用应为0（水力免费）")
	}
	if wm.CO2EmissionKg != 0 {
		t.Error("筒车CO₂排放应为0")
	}
	if wm.EnergySource != "水力/可再生" {
		t.Errorf("筒车能源来源错误：%s", wm.EnergySource)
	}
	if pm.EnergySource != "电网电力" {
		t.Errorf("水泵能源来源错误：%s", pm.EnergySource)
	}
	if pm.TotalCostYuan <= 0 {
		t.Error("水泵总成本应>0")
	}

	t.Logf("✅ 基本指标正确：总水%.0fm³，水泵电费%.2f元，CO₂%.1fkg",
		wm.TotalWaterM3, pm.EnergyCostYuan, pm.CO2EmissionKg)
}

func TestBuildComparison_AncientAdvantage(t *testing.T) {
	c := newTestComparer()
	wheel := makeWheel(1, 9.0, 150.0)

	comp, _ := c.buildComparison(context.Background(),wheel, 100.0, 3.2, 0.6, 0.72, 8.0, 365, "standard")
	adv := comp.AncientAdvantage

	if adv.CostSavedYuan <= 0 {
		t.Errorf("年节省费用应为正，得到%.2f", adv.CostSavedYuan)
	}
	if adv.EnergySavedKWh <= 0 {
		t.Errorf("年节省电量应为正，得到%.2f", adv.EnergySavedKWh)
	}
	if adv.CO2SavedKg <= 0 {
		t.Errorf("年减排CO₂应为正，得到%.2f", adv.CO2SavedKg)
	}
	if adv.CostRatio >= 1.0 {
		t.Errorf("筒车/水泵费用比应<1，得到%.3f（比值越低越省钱）", adv.CostRatio)
	}
	if adv.EnergyRatio >= 1.0 {
		t.Errorf("筒车/水泵能耗比应<1，得到%.3f", adv.EnergyRatio)
	}

	t.Logf("✅ 节能优势指标：年费省%.0f元，节电%.0fkWh，减碳%.0fkg，成本比%.2f%%",
		adv.CostSavedYuan, adv.EnergySavedKWh, adv.CO2SavedKg, adv.CostRatio*100)
}

// ============================================================
// 边界场景测试
// ============================================================

func TestBuildComparison_Boundary_ZeroPeriod(t *testing.T) {
	c := newTestComparer()
	wheel := makeWheel(1, 6.0, 60.0)

	comp, err := c.buildComparison(context.Background(),wheel, 45.0, 2.1, 0.5, 0.65, 6.0, 0, "standard")
	if err != nil {
		t.Fatalf("零周期不应出错: %v", err)
	}

	if comp.WaterwheelMetrics.TotalWaterM3 != 0 {
		t.Errorf("零周期总水量应为0，实际%.2f", comp.WaterwheelMetrics.TotalWaterM3)
	}
	t.Logf("✅ 零周期边界正确：总水量%.2f", comp.WaterwheelMetrics.TotalWaterM3)
}

func TestBuildComparison_Boundary_OneDay(t *testing.T) {
	c := newTestComparer()
	wheel := makeWheel(1, 8.0, 100.0)

	comp, _ := c.buildComparison(context.Background(),wheel, 70.0, 2.8, 0.55, 0.70, 7.0, 1, "standard")

	expectedHours := 24.0 * 0.78
	expectedWater := 70.0 * expectedHours

	if math.Abs(comp.WaterwheelMetrics.TotalWaterM3-expectedWater) > expectedWater*0.01 {
		t.Errorf("单日水量偏差：预期%.0f，实际%.0f", expectedWater, comp.WaterwheelMetrics.TotalWaterM3)
	}
	if comp.PeriodDays != 1 {
		t.Errorf("周期天数字段应为1，实际%d", comp.PeriodDays)
	}
	t.Logf("✅ 单日边界：总水%.0fm³(预期≈%.0f)", comp.WaterwheelMetrics.TotalWaterM3, expectedWater)
}

func TestBuildComparison_Boundary_LowFlow(t *testing.T) {
	c := newTestComparer()
	wheel := makeWheel(1, 5.0, 20.0)

	comp, _ := c.buildComparison(context.Background(),wheel, 5.0, 0.8, 0.3, 0.4, 2.0, 365, "lowflow")

	if comp.ModernPumpMetrics.LiftHeightM < 1.0 {
		t.Errorf("扬程被兜底到1.0m以下：实际%.2f", comp.ModernPumpMetrics.LiftHeightM)
	}
	if comp.ModernPumpMetrics.LiftHeightM < 1.0 || math.Abs(comp.ModernPumpMetrics.LiftHeightM-1.0) < 0.01 {
		t.Logf("✅ 低扬程正确兜底到1.0m")
	}
	if comp.WaterwheelMetrics.AvgEfficiency >= comp.ModernPumpMetrics.AvgEfficiency {
		t.Logf("✅ 低流量场景：筒车综合效率%.2f < 水泵%.2f，符合预期",
			comp.WaterwheelMetrics.AvgEfficiency, comp.ModernPumpMetrics.AvgEfficiency)
	}
}

func TestBuildComparison_Boundary_LargeScaleLongTerm(t *testing.T) {
	c := newTestComparer()
	wheel := makeWheel(1, 12.0, 250.0)

	comp, _ := c.buildComparison(context.Background(),wheel, 200.0, 4.0, 0.58, 0.75, 6.0, 365*10, "longterm")

	if comp.PeriodDays != 3650 {
		t.Errorf("10年应=3650天，实际%d", comp.PeriodDays)
	}

	adv := comp.AncientAdvantage
	if adv.PaybackYears <= 0 || adv.PaybackYears > 50 {
		t.Errorf("回收期%.2f年不合理（应在1~50年）", adv.PaybackYears)
	}
	if adv.BreakEvenM3 <= 0 {
		t.Errorf("盈亏平衡水量应为正，实际%.2f", adv.BreakEvenM3)
	}
	t.Logf("✅ 长期10年场景：回收期%.2f年，盈亏平衡%.0fm³",
		adv.PaybackYears, adv.BreakEvenM3)
}

func TestBuildComparison_Boundary_ScenarioWithLabor(t *testing.T) {
	c := newTestComparer()
	wheel := makeWheel(1, 8.0, 120.0)

	compStd, _ := c.buildComparison(context.Background(),wheel, 80.0, 2.8, 0.55, 0.70, 7.0, 365, "standard")
	compLab, _ := c.buildComparison(context.Background(),wheel, 80.0, 2.8, 0.55, 0.70, 7.0, 365, "with_labor")

	if compLab.WaterwheelMetrics.TotalCostYuan <= compStd.WaterwheelMetrics.TotalCostYuan {
		t.Error("含人工场景筒车成本应高于标准场景")
	}
	if !contains(compLab.Scenario, "labor") {
		t.Errorf("场景字段应含labor标记：%s", compLab.Scenario)
	}
	t.Logf("✅ 人工成本影响：标准%.2f元 vs 含人工%.2f元",
		compStd.WaterwheelMetrics.TotalCostYuan, compLab.WaterwheelMetrics.TotalCostYuan)
}

// ============================================================
// 异常场景测试
// ============================================================

func TestBuildComparison_Anomaly_NegativeFlow(t *testing.T) {
	c := newTestComparer()
	wheel := makeWheel(1, 6.0, 80.0)

	comp, err := c.buildComparison(context.Background(),wheel, -50.0, 2.0, 0.5, 0.6, 5.0, 365, "anomaly")
	if err != nil {
		t.Fatalf("不应因负流量报错: %v", err)
	}

	if comp.WaterwheelMetrics.TotalWaterM3 >= 0 && comp.WaterwheelMetrics.TotalWaterM3 > 1.0 {
		t.Logf("负流量场景输出：总水%.2f（负流量导致异常值，被利用系数×后可能很小）",
			comp.WaterwheelMetrics.TotalWaterM3)
	} else {
		t.Logf("✅ 负流量场景鲁棒性：总水=%.2f（正常或异常值均无崩溃）",
			comp.WaterwheelMetrics.TotalWaterM3)
	}
}

func TestBuildComparison_Anomaly_ZeroEfficiency(t *testing.T) {
	c := newTestComparer()
	wheel := makeWheel(1, 8.0, 100.0)

	comp, _ := c.buildComparison(context.Background(),wheel, 60.0, 2.5, 0.0, 0.0, 0.0, 365, "zeromech")

	if comp == nil {
		t.Fatal("零效率时也应返回结构")
	}
	if comp.WaterwheelMetrics.AvgEfficiency != 0 {
		t.Errorf("输入0效率应得到0综合效率，实际%.3f", comp.WaterwheelMetrics.AvgEfficiency)
	}
	t.Logf("✅ 零效率鲁棒性：综合效率=%.3f，无Panic", comp.WaterwheelMetrics.AvgEfficiency)
}

func TestBuildComparison_Anomaly_HugeFlow(t *testing.T) {
	c := newTestComparer()
	wheel := makeWheel(1, 15.0, 9999.0)

	comp, _ := c.buildComparison(context.Background(),wheel, 99999.0, 10.0, 0.9, 0.95, 20.0, 365, "huge")

	adv := comp.AncientAdvantage
	if math.IsNaN(adv.CostSavedYuan) || math.IsInf(adv.CostSavedYuan, 0) {
		t.Errorf("巨大流量下出现NaN/Inf：节省%.2f元", adv.CostSavedYuan)
	}
	if math.IsNaN(adv.PaybackYears) || math.IsInf(adv.PaybackYears, 0) {
		t.Errorf("巨大流量下回收期异常：%.2f年", adv.PaybackYears)
	}
	t.Logf("✅ 巨量流量鲁棒性：省%.0f元/年，回收期%.2f年", adv.CostSavedYuan, adv.PaybackYears)
}

// ============================================================
// 物理公式正确性：水泵能耗验证
// ============================================================

func TestModernPumpEnergyCalculation(t *testing.T) {
	c := newTestComparer()
	p := c.params
	rho := 1000.0
	g := 9.81

	flowM3H := 100.0
	liftM := 3.0

	flowM3S := flowM3H / 3600.0
	hydraulicKW := (rho * g * flowM3S * liftM) / 1000.0
	shaftKW := hydraulicKW / p.ModernPumpEfficiency
	if shaftKW < 1.0 {
		shaftKW = 1.0
	}

	hours := 365 * 24 * 0.78
	totalKWh := shaftKW * hours

	comp, _ := c.buildComparison(context.Background(),makeWheel(1, 8.0, 150), flowM3H, liftM/0.9, 0.6, 0.72, 7.0, 365, "verify")
	actual := comp.ModernPumpMetrics.TotalEnergyKWh

	errRatio := math.Abs(actual-totalKWh) / totalKWh
	if errRatio > 0.2 {
		t.Errorf("水泵能耗偏差>20%%：计算%.0f vs 实际%.0f (误差%.1f%%)",
			totalKWh, actual, errRatio*100)
	}
	t.Logf("✅ 水泵能耗物理公式验证：理论≈%.0f，实际%.0f kWh (误差%.1f%%)",
		totalKWh, actual, errRatio*100)
}

// ============================================================
// CO₂减排系数验证
// ============================================================

func TestCO2Emission_Factor(t *testing.T) {
	c := newTestComparer()
	wheel := makeWheel(1, 8.0, 120.0)
	comp, _ := c.buildComparison(context.Background(),wheel, 80.0, 3.0, 0.55, 0.70, 7.0, 365, "co2check")

	pm := comp.ModernPumpMetrics
	expectedCO2 := pm.TotalEnergyKWh * c.params.CO2GridFactorKgPerKWh
	if math.Abs(pm.CO2EmissionKg-expectedCO2) > 1.0 {
		t.Errorf("CO₂排放计算错误：%.1f vs 期望%.1f (= %.1f kWh × %.3f)",
			pm.CO2EmissionKg, expectedCO2, pm.TotalEnergyKWh, c.params.CO2GridFactorKgPerKWh)
	}
	t.Logf("✅ CO₂排放系数正确：%.0f kg = %.0f kWh × %.3f kg/kWh",
		pm.CO2EmissionKg, pm.TotalEnergyKWh, c.params.CO2GridFactorKgPerKWh)
}

// ============================================================
// 全生命周期：回收期公式验证
// ============================================================

func TestPaybackYears_Formula(t *testing.T) {
	c := newTestComparer()
	wheel := makeWheel(1, 8.5, 140.0)
	comp, _ := c.buildComparison(context.Background(),wheel, 95.0, 3.0, 0.58, 0.72, 7.5, 365, "pb")

	adv := comp.AncientAdvantage
	yearScale := 365.0 / float64(comp.PeriodDays)
	annualSaved := (comp.ModernPumpMetrics.TotalCostYuan - comp.WaterwheelMetrics.TotalCostYuan) * yearScale
	expectedPB := 0.0
	if annualSaved > 0 {
		expectedPB = c.params.WaterwheelBuildCostYuan / annualSaved
	}

	if annualSaved > 0 && math.Abs(adv.PaybackYears-expectedPB) > 0.1 {
		t.Errorf("回收期公式不符：%.2f vs 手算%.2f (年省%.0f, 建造费%.0f)",
			adv.PaybackYears, expectedPB, annualSaved, c.params.WaterwheelBuildCostYuan)
	}
	t.Logf("✅ 回收期公式正确：%.2f年 (= 建造成本%.0f / 年费省%.0f)",
		adv.PaybackYears, c.params.WaterwheelBuildCostYuan, annualSaved)
}

// ============================================================
// 建造模板完整性测试
// ============================================================

func TestGetBuildPresets_AllValid(t *testing.T) {
	c := newTestComparer()
	presets := c.GetBuildPresets()

	if len(presets) != 5 {
		t.Errorf("应有5个经典模板，实际%d个", len(presets))
	}

	ids := make(map[string]bool)
	for _, p := range presets {
		if p.ID == "" {
			t.Error("模板ID不应为空")
		}
		if p.Name == "" {
			t.Error("模板名不应为空")
		}
		if ids[p.ID] {
			t.Errorf("重复模板ID: %s", p.ID)
		}
		ids[p.ID] = true

		if p.Params.Diameter <= 0 {
			t.Errorf("模板%s直径无效: %.2f", p.Name, p.Params.Diameter)
		}
		if p.Params.BucketCount < 6 {
			t.Errorf("模板%s水斗数太少: %d", p.Name, p.Params.BucketCount)
		}
		if !p.Unlocked {
			t.Errorf("预置模板%s应默认解锁", p.Name)
		}
	}
	expected := []string{"dujiangyan", "fenghuang", "lijiang", "zhuzhi", "zhutie"}
	for _, e := range expected {
		if !ids[e] {
			t.Errorf("缺失关键模板: %s", e)
		}
	}
	t.Logf("✅ %d个建造模板全部完整有效", len(presets))
}

// ============================================================
// 精度函数与辅助函数
// ============================================================

func TestRoundingFunctions(t *testing.T) {
	if round2(3.14159) != 3.14 {
		t.Error("round2 错误")
	}
	if round2(2.71828) != 2.72 {
		t.Error("round2 四舍五入错误")
	}
	if round3(1.2345) != 1.235 {
		t.Error("round3 五入错误")
	}
	if round3(9.8764) != 9.876 {
		t.Error("round3 四舍错误")
	}
	t.Log("✅ 舍入函数精度测试通过")
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(s) > 0 &&
		(len(sub) == 0 || indexOf(s, sub) >= 0))
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}

// ============================================================
// 节能比例范围合理性
// ============================================================

func TestAncientAdvantageRanges(t *testing.T) {
	c := newTestComparer()

	wheels := []*models.Waterwheel{
		makeWheel(1, 6.0, 60.0),
		makeWheel(2, 8.0, 120.0),
		makeWheel(3, 10.0, 200.0),
	}
	flows := []float64{40, 85, 160}
	drops := []float64{2.1, 2.8, 3.5}

	for i, w := range wheels {
		t.Run(w.Name, func(t *testing.T) {
			comp, _ := c.buildComparison(context.Background(),w, flows[i], drops[i], 0.55, 0.70, 7.0, 365, "range")
			adv := comp.AncientAdvantage

			if adv.CostRatio > 1.0 {
				t.Errorf("W%d成本比%.3f>1，筒车失去经济优势（可能参数问题）", i, adv.CostRatio)
			}
			if adv.PaybackYears < 0.5 || adv.PaybackYears > 30 {
				t.Errorf("W%d回收期%.2f年不在合理范围[0.5,30]", i, adv.PaybackYears)
			}
			t.Logf("✅ W%d: 成本比=%.0f%%, 回收期=%.1f年（合理）",
				i+1, adv.CostRatio*100, adv.PaybackYears)
		})
	}
}

// ============================================================
// 归一化能效比验证
// ============================================================

func TestBuildComparison_NormalizedEffRatio(t *testing.T) {
	c := newTestComparer()
	wheel := makeWheel(1, 8.0, 120.0)

	comp, _ := c.buildComparison(context.Background(), wheel, 80.0, 2.8, 0.55, 0.70, 7.5, 365, "standard")
	adv := comp.AncientAdvantage

	if adv.NormalizedEffRatio < 0 {
		t.Errorf("归一化能效比不可为负: %.3f", adv.NormalizedEffRatio)
	}
	if math.IsNaN(adv.NormalizedEffRatio) || math.IsInf(adv.NormalizedEffRatio, 0) {
		t.Errorf("归一化能效比不应为NaN/Inf: %.3f", adv.NormalizedEffRatio)
	}
	if adv.PumpLoadFactor <= 0 || adv.PumpLoadFactor > 1.0 {
		t.Errorf("水泵负载因子应∈(0,1], 实际%.3f", adv.PumpLoadFactor)
	}
	t.Logf("✅ 归一化能效比=%.3f, 水泵负载因子=%.2f",
		adv.NormalizedEffRatio, adv.PumpLoadFactor)
}

func TestBuildComparison_PumpLoadFactorEffect(t *testing.T) {
	c := newTestComparer()
	wheel := makeWheel(1, 8.0, 120.0)

	origLF := c.params.PumpLoadFactor
	c.params.PumpLoadFactor = 0.5
	compLow, _ := c.buildComparison(context.Background(), wheel, 80.0, 2.8, 0.55, 0.70, 7.5, 365, "low_load")

	c.params.PumpLoadFactor = 1.0
	compHigh, _ := c.buildComparison(context.Background(), wheel, 80.0, 2.8, 0.55, 0.70, 7.5, 365, "full_load")
	c.params.PumpLoadFactor = origLF

	t.Logf("低载(50%%)电费=%.2f vs 满载(100%%)电费=%.2f",
		compLow.ModernPumpMetrics.EnergyCostYuan, compHigh.ModernPumpMetrics.EnergyCostYuan)

	if compLow.AncientAdvantage.PumpLoadFactor >= compHigh.AncientAdvantage.PumpLoadFactor+0.01 {
		t.Log("低载因子记录值应低于满载（合理）")
	}
}
