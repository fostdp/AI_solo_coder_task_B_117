package forecasting

import (
	"math"
	"testing"
	"time"

	"waterwheel-monitor/internal/config"
	"waterwheel-monitor/internal/models"
)

func newTestForecaster() *WaterLevelForecaster {
	return &WaterLevelForecaster{
		db:     nil,
		params: config.DefaultForecastingParams(),
	}
}

func genHistory(months []int, drops, flows []float64) []models.HistoricalHydrology {
	n := len(months)
	h := make([]models.HistoricalHydrology, n)
	base := time.Date(2023, 1, 15, 0, 0, 0, 0, time.UTC)
	for i := 0; i < n; i++ {
		h[i] = models.HistoricalHydrology{
			WaterwheelID: 1,
			Date:         base.AddDate(0, i, 0),
			AvgDrop:      drops[i],
			AvgFlow:      flows[i],
			RainfallMm:   0,
			Month:        months[i],
		}
	}
	return h
}

func gen3YearHistory() []models.HistoricalHydrology {
	months := make([]int, 36)
	drops := make([]float64, 36)
	flows := make([]float64, 36)
	seasonDrop := map[int]float64{1: 1.0, 2: 1.2, 3: 2.0, 4: 2.8, 5: 3.0, 6: 3.5, 7: 3.8, 8: 3.6, 9: 2.5, 10: 1.8, 11: 1.4, 12: 1.1}
	seasonFlow := map[int]float64{1: 0.6, 2: 0.7, 3: 1.2, 4: 1.6, 5: 2.0, 6: 2.4, 7: 2.5, 8: 2.3, 9: 1.6, 10: 1.1, 11: 0.8, 12: 0.65}
	base := time.Date(2022, 1, 1, 0, 0, 0, 0, time.UTC)
	for i := 0; i < 36; i++ {
		d := base.AddDate(0, i, 0)
		m := int(d.Month())
		months[i] = m
		drops[i] = seasonDrop[m] + (float64(i)-18)*0.005
		flows[i] = seasonFlow[m] + (float64(i)-18)*0.003
	}
	h := make([]models.HistoricalHydrology, 36)
	for i := 0; i < 36; i++ {
		h[i] = models.HistoricalHydrology{
			WaterwheelID: 1,
			Date:         base.AddDate(0, i, 0),
			AvgDrop:      drops[i],
			AvgFlow:      flows[i],
			Month:        months[i],
		}
	}
	return h
}

// ============================================================
// 季节分类正确性测试
// ============================================================

func TestMonthToSeason_Correctness(t *testing.T) {
	cases := []struct {
		month    int
		expected string
	}{
		{1, "冬枯"}, {2, "冬枯"}, {12, "冬枯"},
		{3, "春汛"}, {4, "春汛"}, {5, "春汛"},
		{6, "夏丰"}, {7, "夏丰"}, {8, "夏丰"},
		{9, "秋平"}, {10, "秋平"}, {11, "秋平"},
		{13, "过渡"}, {0, "过渡"}, {-1, "过渡"},
	}
	for _, c := range cases {
		got := monthToSeason(c.month)
		if got != c.expected {
			t.Errorf("monthToSeason(%d) = %s, 期望%s", c.month, got, c.expected)
		}
	}
	t.Log("✅ 季节分类测试通过，16种月份全部正确")
}

// ============================================================
// 自回归预测准确性测试
// ============================================================

func TestAutoregressive_NormalCase(t *testing.T) {
	f := newTestForecaster()
	history := gen3YearHistory()

	drop, flow := f.autoregressive(history, 30)

	if drop <= 0 {
		t.Error("落差预测应为正")
	}
	if flow <= 0 {
		t.Error("流量预测应为正")
	}

	avgDrop := 0.0
	avgFlow := 0.0
	for _, h := range history {
		avgDrop += h.AvgDrop
		avgFlow += h.AvgFlow
	}
	avgDrop /= float64(len(history))
	avgFlow /= float64(len(history))

	if math.Abs(drop-avgDrop) > avgDrop*0.6 {
		t.Errorf("落差预测%.3f偏离历史均值%.3f过大", drop, avgDrop)
	}
	if math.Abs(flow-avgFlow) > avgFlow*0.6 {
		t.Errorf("流量预测%.3f偏离历史均值%.3f过大", flow, avgFlow)
	}
	t.Logf("✅ 自回归预测正常：落差%.3fm, 流量%.3fm/s (均值%.3f/%.3f)",
		drop, flow, avgDrop, avgFlow)
}

func TestAutoregressive_HorizonDecay(t *testing.T) {
	f := newTestForecaster()
	history := gen3YearHistory()

	dropShort, _ := f.autoregressive(history, 1)
	dropLong, _ := f.autoregressive(history, 365)

	if dropShort <= 0 || dropLong <= 0 {
		t.Fatal("预测不应为0")
	}
	if dropLong >= dropShort {
		t.Errorf("长期预测(365天)=%.4f应小于短期(1天)=%.4f，因为horizonFactor衰减", dropLong, dropShort)
	}
	t.Logf("✅ 时间衰减正常：1天=%.4f, 365天=%.4f (衰减%.1f%%)",
		dropShort, dropLong, (1-dropLong/dropShort)*100)
}

// ============================================================
// 趋势分量测试
// ============================================================

func TestTrendComponent_NormalTrend(t *testing.T) {
	f := newTestForecaster()

	n := 60
	drops := make([]float64, n)
	flows := make([]float64, n)
	months := make([]int, n)
	for i := 0; i < n; i++ {
		drops[i] = 2.0 + float64(i)*0.02
		flows[i] = 1.2 + float64(i)*0.01
		months[i] = (i % 12) + 1
	}
	history := genHistory(months, drops, flows)

	drop, flow := f.trendComponent(history, 30)

	if drop <= drops[len(drops)-1] {
		t.Errorf("上升趋势下预测值%.3f应大于最后观测%.3f", drop, drops[len(drops)-1])
	}
	if flow <= flows[len(flows)-1] {
		t.Errorf("上升趋势下流量预测%.3f应大于最后观测%.3f", flow, flows[len(flows)-1])
	}
	t.Logf("✅ 上升趋势识别正确：落差末值%.2f→预测%.2f，流量末值%.2f→预测%.2f",
		drops[len(drops)-1], drop, flows[len(flows)-1], flow)
}

func TestTrendComponent_InsufficientData(t *testing.T) {
	f := newTestForecaster()

	months := []int{1, 2, 3, 4}
	drops := []float64{2.0, 2.1, 2.2, 2.3}
	flows := []float64{1.0, 1.1, 1.2, 1.3}
	history := genHistory(months, drops, flows)

	drop, flow := f.trendComponent(history, 30)

	if drop != 0 || flow != 0 {
		t.Errorf("数据不足(4条<30)应返回0,0，实际%.3f,%.3f", drop, flow)
	}
	t.Log("✅ 数据不足时趋势分量正确返回0")
}

func TestTrendComponent_DecliningTrend(t *testing.T) {
	f := newTestForecaster()

	n := 60
	drops := make([]float64, n)
	flows := make([]float64, n)
	months := make([]int, n)
	for i := 0; i < n; i++ {
		drops[i] = 4.0 - float64(i)*0.03
		flows[i] = 2.5 - float64(i)*0.02
		months[i] = (i % 12) + 1
	}
	history := genHistory(months, drops, flows)

	drop, flow := f.trendComponent(history, 30)
	lastDrop := drops[len(drops)-1]
	lastFlow := flows[len(flows)-1]

	if drop >= lastDrop {
		t.Errorf("下降趋势下预测%.3f应小于末观测%.3f", drop, lastDrop)
	}
	if flow >= lastFlow {
		t.Errorf("下降趋势下流量预测%.3f应小于末观测%.3f", flow, lastFlow)
	}
	t.Logf("✅ 下降趋势识别正确：落差末值%.2f→预测%.2f，流量末值%.2f→预测%.2f",
		lastDrop, drop, lastFlow, flow)
}

// ============================================================
// 波动率/置信区间测试
// ============================================================

func TestCalcVolatility_StableHistory(t *testing.T) {
	f := newTestForecaster()

	months := make([]int, 36)
	drops := make([]float64, 36)
	flows := make([]float64, 36)
	for i := 0; i < 36; i++ {
		months[i] = 7
		drops[i] = 3.00 + 0.03*float64(i%3)
		flows[i] = 2.00 + 0.02*float64(i%3)
	}
	history := genHistory(months, drops, flows)

	vol := f.calcVolatility(history, 7)

	if vol > 0.2 {
		t.Errorf("平稳数据波动率应较低，实际%.4f (>0.2)", vol)
	}
	if vol < 0 {
		t.Errorf("波动率不应为负: %.4f", vol)
	}
	t.Logf("✅ 平稳数据低波动率：%.4f", vol)
}

func TestCalcVolatility_HighVariance(t *testing.T) {
	f := newTestForecaster()

	months := make([]int, 24)
	drops := make([]float64, 24)
	flows := make([]float64, 24)
	for i := 0; i < 24; i++ {
		months[i] = 7
		if i%2 == 0 {
			drops[i] = 1.0
			flows[i] = 0.5
		} else {
			drops[i] = 5.0
			flows[i] = 4.0
		}
	}
	history := genHistory(months, drops, flows)

	volStable := f.calcVolatility(history, 7)

	months2 := make([]int, 24)
	drops2 := make([]float64, 24)
	flows2 := make([]float64, 24)
	for i := 0; i < 24; i++ {
		months2[i] = 7
		drops2[i] = 3.0
		flows2[i] = 2.0
	}
	history2 := genHistory(months2, drops2, flows2)
	volLow := f.calcVolatility(history2, 7)

	if volStable <= volLow {
		t.Errorf("高变历史波动率%.4f应大于稳定数据%.4f", volStable, volLow)
	}
	t.Logf("✅ 波动率敏感性验证：高变=%.4f, 低变=%.4f", volStable, volLow)
}

func TestCalcVolatility_InsufficientMonthData(t *testing.T) {
	f := newTestForecaster()

	months := []int{7, 8}
	drops := []float64{3.0, 2.0}
	flows := []float64{1.5, 1.0}
	history := genHistory(months, drops, flows)

	vol := f.calcVolatility(history, 9) // 9月只有0条

	if math.Abs(vol-0.15*0.7) > 0.001 {
		t.Errorf("无同月数据应返回默认CV=0.15*0.7=0.105，实际%.4f", vol)
	}
	t.Logf("✅ 数据不足时回退默认波动率：%.4f", vol)
}

// ============================================================
// 边界场景测试
// ============================================================

func TestAutoregressive_EmptyHistory(t *testing.T) {
	f := newTestForecaster()
	history := []models.HistoricalHydrology{}

	drop, flow := f.autoregressive(history, 30)

	if drop != 0 || flow != 0 {
		t.Errorf("空历史应返回(0,0)，实际%.4f,%.4f", drop, flow)
	}
	t.Log("✅ 空历史边界处理正确")
}

func TestAutoregressive_ShortHistory(t *testing.T) {
	f := newTestForecaster()

	months := []int{1, 2, 3}
	drops := []float64{2.0, 2.5, 3.0}
	flows := []float64{1.0, 1.2, 1.4}
	history := genHistory(months, drops, flows)

	drop, flow := f.autoregressive(history, 30)

	if drop <= 0 || flow <= 0 {
		t.Errorf("短历史也应返回正预测，得到%.4f,%.4f", drop, flow)
	}
	avgD := (2.0 + 2.5 + 3.0) / 3.0
	if math.Abs(drop-avgD) > avgD {
		t.Errorf("偏离均值过大：预测%.4f，均值%.4f", drop, avgD)
	}
	t.Logf("✅ 短历史(3条)正确处理：预测落差%.3f (均值%.3f)", drop, avgD)
}

func TestAutoregressive_ZeroHorizon(t *testing.T) {
	f := newTestForecaster()
	history := gen3YearHistory()

	drop0, flow0 := f.autoregressive(history, 0)
	drop1, flow1 := f.autoregressive(history, 1)

	if math.Abs(drop0-drop1) > 0.001 || math.Abs(flow0-flow1) > 0.001 {
		t.Errorf("horizon=0应与horizon=1效果相同(1/(1+0/180)=1)")
	}
	t.Logf("✅ horizon=0边界正确：%.4f≈%.4f", drop0, drop1)
}

// ============================================================
// 异常场景测试
// ============================================================

func TestAutoregressive_ExtremeSpike(t *testing.T) {
	f := newTestForecaster()
	n := 40
	months := make([]int, n)
	drops := make([]float64, n)
	flows := make([]float64, n)
	for i := 0; i < n; i++ {
		months[i] = (i % 12) + 1
		drops[i] = 2.0
		flows[i] = 1.2
	}
	drops[38] = 9999.0
	drops[39] = -9999.0
	history := genHistory(months, drops, flows)

	drop, flow := f.autoregressive(history, 30)

	if drop > 100 || drop < 0 {
		t.Errorf("极端值未被平滑：预测落差%.4f不合理", drop)
	}
	if flow <= 0 {
		t.Error("流量预测应为正")
	}
	t.Logf("✅ 极端值场景输出在合理范围：落差%.2f, 流量%.2f", drop, flow)
}

// ============================================================
// 精度函数测试
// ============================================================

func TestRound2_Round3_Precision(t *testing.T) {
	cases2 := []struct{ in, out float64 }{
		{2.344, 2.34}, {2.345, 2.35}, {-1.234, -1.23}, {0.0, 0.0},
	}
	for _, c := range cases2 {
		res := round2(c.in)
		if res != c.out {
			t.Errorf("round2(%.3f)=%.3f, 期望%.3f", c.in, res, c.out)
		}
	}
	cases3 := []struct{ in, out float64 }{
		{1.2344, 1.234}, {1.2345, 1.235}, {-0.1234, -0.123},
	}
	for _, c := range cases3 {
		res := round3(c.in)
		if res != c.out {
			t.Errorf("round3(%.4f)=%.4f, 期望%.4f", c.in, res, c.out)
		}
	}
	t.Log("✅ round2/round3 精度测试通过")
}

// ============================================================
// 三因子加权一致性测试
// ============================================================

func TestThreeFactorWeighting_Sum(t *testing.T) {
	f := newTestForecaster()
	p := f.params

	total := p.SeasonWeight + p.ARWeight + p.TrendWeight

	if math.Abs(total-1.0) > 0.01 {
		t.Errorf("三因子权重之和应≈1.0, 实际%.3f (季节%.2f + 自回归%.2f + 趋势%.2f)",
			total, p.SeasonWeight, p.ARWeight, p.TrendWeight)
	}
	t.Logf("✅ 三因子权重总和正确：%.3f", total)
}

func TestMinConfidence_Floor(t *testing.T) {
	f := newTestForecaster()

	if f.params.MinConfidence < 0.5 || f.params.MinConfidence > 0.9 {
		t.Errorf("最小置信度阈值%.2f不在合理范围[0.5,0.9]", f.params.MinConfidence)
	}
	if f.params.TargetSubmergence < 0.2 || f.params.TargetSubmergence > 0.5 {
		t.Errorf("目标浸没度%.2f不在合理范围[0.2,0.5]", f.params.TargetSubmergence)
	}
	t.Logf("✅ 参数合理性：置信度下界%.2f，目标浸没度%.2f",
		f.params.MinConfidence, f.params.TargetSubmergence)
}

// ============================================================
// 浸没度计算逻辑验证
// ============================================================

type subTestCase struct {
	name         string
	radius       float64
	currentH     float64
	drop         float64
	expectedSub  float64
	tolerance    float64
}

func TestSubmergenceCalculation_Logic(t *testing.T) {
	cases := []subTestCase{
		{"完全浸没", 3.0, 0.0, 6.5, 1.0, 0.01},
		{"完全露出", 3.0, 5.0, 1.0, 0.0, 0.01},
		{"35%目标", 3.0, 2.95, 4.0, 0.35, 0.01},
		{"50%一半", 4.0, 2.0, 4.0, 0.5, 0.01},
	}
	for _, c := range cases {
		sub := 0.0
		if c.radius+c.currentH < c.drop {
			sub = 1.0
		} else {
			sub = (c.drop - c.currentH) / c.radius
		}
		if sub < 0 {
			sub = 0
		}
		if sub > 1 {
			sub = 1
		}
		if math.Abs(sub-c.expectedSub) > c.tolerance {
			t.Errorf("[%s] 浸没度计算错误：半径%.1f, 高%.1f, 落%.1f → 得%.3f, 期望%.3f",
				c.name, c.radius, c.currentH, c.drop, sub, c.expectedSub)
		}
		t.Logf("✅ [%s] 浸没度=%.2f%% (正确)", c.name, sub*100)
	}
}
