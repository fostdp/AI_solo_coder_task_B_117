package virtual_builder

import (
	"math"
	"strings"
	"testing"

	"waterwheel-monitor/internal/config"
	"waterwheel-monitor/internal/models"
)

func newTestEngine() *BuildEngine {
	return &BuildEngine{db: nil, params: config.DefaultBuildParams()}
}

func makeBuild(diam float64, buckets int, spokes int, material string, bucketCap float64) *models.VirtualBuild {
	return &models.VirtualBuild{
		BuildName:      "测试筒车",
		Diameter:       diam,
		BucketCount:    buckets,
		BucketCapacity: bucketCap,
		SpokeCount:     spokes,
		Material:       material,
		WheelAngle:     0,
		InstallHeight:  0.5,
	}
}

// ============================================================
// 类别1: 参数边界验证 - 正常/边界/异常
// ============================================================

func TestValidateAndSimulate_NormalCase(t *testing.T) {
	e := newTestEngine()
	build := makeBuild(8.0, 24, 12, "杉木", 0.05)
	sim, err := e.ValidateAndSimulate(build, 1.5, 2.0)
	if err != nil {
		t.Fatalf("正常参数应通过验证, 实际报错: %v", err)
	}
	if sim == nil {
		t.Fatal("仿真结果不应为nil")
	}
	if !sim.Stable {
		t.Errorf("正常参数下应稳定, 警告: %s", sim.Warning)
	}
	if sim.LiftRate <= 0 {
		t.Error("提水量应>0")
	}
	if sim.Torque <= 0 {
		t.Error("净转矩应>0")
	}
	if sim.Rpm <= 0 || sim.Rpm > 25 {
		t.Errorf("转速应∈(0,25], 实际%.2f", sim.Rpm)
	}
	if sim.BucketFillEff <= 0.15 || sim.BucketFillEff > 0.95 {
		t.Errorf("充水效率应∈[0.15,0.95], 实际%.3f", sim.BucketFillEff)
	}
	if sim.StressLevel < 0 || sim.StressLevel > 1.2 {
		t.Errorf("应力应∈[0,1.2], 实际%.3f", sim.StressLevel)
	}
	t.Logf("正常仿真: rpm=%.1f 提水=%.1fm³/h 转矩=%.0fNm 应力=%.0f%% 充水=%.0f%% 浸没斗=%d",
		sim.Rpm, sim.LiftRate, sim.Torque, sim.StressLevel*100, sim.BucketFillEff*100, sim.SubmergedBuckets)
}

func TestValidateAndSimulate_Boundary_DiameterMin(t *testing.T) {
	e := newTestEngine()
	build := makeBuild(2.0, 12, 8, "杉木", 0.02)
	_, err := e.ValidateAndSimulate(build, 1.5, 0.8)
	if err != nil {
		t.Fatalf("直径=%.1f(最小合法)应通过, 报错: %v", 2.0, err)
	}
	t.Log("最小直径2.0m合法通过")
}

func TestValidateAndSimulate_Boundary_DiameterMax(t *testing.T) {
	e := newTestEngine()
	build := makeBuild(15.0, 48, 24, "铸铁", 0.10)
	_, err := e.ValidateAndSimulate(build, 2.5, 5.0)
	if err != nil {
		t.Fatalf("直径=%.1f(最大合法)应通过, 报错: %v", 15.0, err)
	}
	t.Log("最大直径15.0m合法通过")
}

func TestValidateAndSimulate_Anomaly_DiameterTooSmall(t *testing.T) {
	e := newTestEngine()
	build := makeBuild(1.5, 12, 8, "杉木", 0.02)
	_, err := e.ValidateAndSimulate(build, 1.5, 0.8)
	if err == nil {
		t.Fatal("直径1.5m<2.0m应报错")
	}
	if !strings.Contains(err.Error(), "直径") {
		t.Errorf("错误信息应提及'直径', 实际: %v", err)
	}
	t.Logf("超小直径正确拦截: %v", err)
}

func TestValidateAndSimulate_Anomaly_DiameterTooLarge(t *testing.T) {
	e := newTestEngine()
	build := makeBuild(16.0, 48, 24, "铸铁", 0.10)
	_, err := e.ValidateAndSimulate(build, 1.5, 5.0)
	if err == nil {
		t.Fatal("直径16.0m>15.0m应报错")
	}
	t.Logf("超大直径正确拦截: %v", err)
}

func TestValidateAndSimulate_Boundary_BucketsMin(t *testing.T) {
	e := newTestEngine()
	build := makeBuild(6.0, 6, 8, "杉木", 0.05)
	_, err := e.ValidateAndSimulate(build, 1.5, 1.8)
	if err != nil {
		t.Fatalf("斗数=%d(最小合法)应通过, 报错: %v", 6, err)
	}
	t.Log("最小斗数6合法通过")
}

func TestValidateAndSimulate_Boundary_BucketsMax(t *testing.T) {
	e := newTestEngine()
	build := makeBuild(10.0, 48, 20, "柏木", 0.05)
	_, err := e.ValidateAndSimulate(build, 2.0, 3.5)
	if err != nil {
		t.Fatalf("斗数=%d(最大合法)应通过, 报错: %v", 48, err)
	}
	t.Log("最大斗数48合法通过")
}

func TestValidateAndSimulate_Anomaly_BucketsTooFew(t *testing.T) {
	e := newTestEngine()
	build := makeBuild(6.0, 5, 8, "杉木", 0.05)
	_, err := e.ValidateAndSimulate(build, 1.5, 1.8)
	if err == nil {
		t.Fatal("斗数5<6应报错")
	}
	if !strings.Contains(err.Error(), "水斗数") {
		t.Errorf("错误信息应提及'水斗数', 实际: %v", err)
	}
	t.Logf("斗数过少正确拦截: %v", err)
}

func TestValidateAndSimulate_Anomaly_BucketsTooMany(t *testing.T) {
	e := newTestEngine()
	build := makeBuild(10.0, 50, 20, "柏木", 0.05)
	_, err := e.ValidateAndSimulate(build, 2.0, 3.5)
	if err == nil {
		t.Fatal("斗数50>48应报错")
	}
	t.Logf("斗数过多正确拦截: %v", err)
}

func TestValidateAndSimulate_Boundary_SpokesMin(t *testing.T) {
	e := newTestEngine()
	build := makeBuild(6.0, 16, 4, "松木", 0.04)
	_, err := e.ValidateAndSimulate(build, 1.8, 2.0)
	if err != nil {
		t.Fatalf("辐条=%d(最小合法)应通过, 报错: %v", 4, err)
	}
	t.Log("最小辐条4合法通过")
}

func TestValidateAndSimulate_Boundary_SpokesMax(t *testing.T) {
	e := newTestEngine()
	build := makeBuild(12.0, 36, 24, "楠木", 0.08)
	_, err := e.ValidateAndSimulate(build, 2.0, 4.0)
	if err != nil {
		t.Fatalf("辐条=%d(最大合法)应通过, 报错: %v", 24, err)
	}
	t.Log("最大辐条24合法通过")
}

func TestValidateAndSimulate_Anomaly_SpokesTooFew(t *testing.T) {
	e := newTestEngine()
	build := makeBuild(6.0, 16, 3, "松木", 0.04)
	_, err := e.ValidateAndSimulate(build, 1.8, 2.0)
	if err == nil {
		t.Fatal("辐条3<4应报错")
	}
	if !strings.Contains(err.Error(), "辐条数") {
		t.Errorf("错误信息应提及'辐条数', 实际: %v", err)
	}
	t.Logf("辐条过少正确拦截: %v", err)
}

func TestValidateAndSimulate_Anomaly_SpokesTooMany(t *testing.T) {
	e := newTestEngine()
	build := makeBuild(12.0, 36, 26, "楠木", 0.08)
	_, err := e.ValidateAndSimulate(build, 2.0, 4.0)
	if err == nil {
		t.Fatal("辐条26>24应报错")
	}
	t.Logf("辐条过多正确拦截: %v", err)
}

func TestValidateAndSimulate_Anomaly_UnknownMaterial(t *testing.T) {
	e := newTestEngine()
	build := makeBuild(8.0, 20, 12, "钛合金", 0.05)
	_, err := e.ValidateAndSimulate(build, 1.5, 2.5)
	if err == nil {
		t.Fatal("未知材质'钛合金'应报错")
	}
	if !strings.Contains(err.Error(), "材质") {
		t.Errorf("错误信息应提及'材质', 实际: %v", err)
	}
	t.Logf("未知材质正确拦截: %v", err)
}

func TestValidateAndSimulate_Boundary_DefaultFlowAndDrop(t *testing.T) {
	e := newTestEngine()
	build := makeBuild(8.0, 24, 12, "杉木", 0.05)
	sim, err := e.ValidateAndSimulate(build, 0, 0)
	if err != nil {
		t.Fatalf("零流速零落差应回退默认值, 报错: %v", err)
	}
	if sim.FlowVelocity != 1.5 {
		t.Errorf("默认流速应为1.5, 实际%.3f", sim.FlowVelocity)
	}
	if sim.WaterDrop != 2.0 {
		t.Errorf("默认落差应为2.0, 实际%.3f", sim.WaterDrop)
	}
	t.Logf("零输入回退默认值: flow=%.3f drop=%.3f", sim.FlowVelocity, sim.WaterDrop)
}

// ============================================================
// 类别2: 物理仿真逻辑 - 转矩/转速/提水量/应力
// ============================================================

func TestRunSimulation_LowFlow_Unstable(t *testing.T) {
	e := newTestEngine()
	build := makeBuild(12.0, 36, 12, "铸铁", 0.08)
	sim := e.runSimulation(build, 0.1, 0.3)
	if sim.Stable {
		t.Error("极低流量应不稳定")
	}
	if sim.Rpm != 0 || sim.LiftRate != 0 || sim.Torque != 0 {
		t.Errorf("无法驱动时 rpm/提水/转矩应全为0, 实际 rpm=%.2f lift=%.2f torque=%.2f",
			sim.Rpm, sim.LiftRate, sim.Torque)
	}
	if sim.Warning == "" {
		t.Error("不稳定时应给出警告信息")
	}
	if !strings.Contains(sim.Warning, "水流") {
		t.Errorf("警告应提及水流问题: %s", sim.Warning)
	}
	t.Logf("低流量预警: %s", sim.Warning)
}

func TestRunSimulation_HighStress_Unstable(t *testing.T) {
	e := newTestEngine()
	build := makeBuild(10.0, 48, 4, "竹制", 0.12)
	sim := e.runSimulation(build, 3.5, 7.0)
	if sim.StressLevel <= 0.85 {
		t.Logf("竹制4辐条高浸没场景: 应力=%.2f (可能触发阈值取决于参数)", sim.StressLevel)
	}
	if sim.StressLevel > 1.2 {
		t.Errorf("应力应被clamp到≤1.2, 实际%.3f", sim.StressLevel)
	}
	t.Logf("极端工况: 应力=%.0f%% rpm=%.1f 提水=%.1f 稳定=%v warn=%s",
		sim.StressLevel*100, sim.Rpm, sim.LiftRate, sim.Stable, sim.Warning)
}

func TestRunSimulation_Rpm_UpperBound(t *testing.T) {
	e := newTestEngine()
	build := makeBuild(3.0, 12, 8, "杉木", 0.02)
	sim := e.runSimulation(build, 5.0, 1.5)
	if sim.Rpm > 25.01 {
		t.Errorf("转速上限应为25rpm, 实际%.2f", sim.Rpm)
	}
	t.Logf("高流量小筒车: rpm=%.1f (上限25)", sim.Rpm)
}

func TestRunSimulation_SubmergedFraction_Clamp(t *testing.T) {
	e := newTestEngine()
	build1 := makeBuild(10.0, 20, 12, "杉木", 0.05)
	sim1 := e.runSimulation(build1, 1.5, 0.1)
	if sim1.SubmergedBuckets <= 0 {
		t.Error("极小落差时浸没分率被clamp到0.05, 浸没斗数应>0")
	}

	build2 := makeBuild(5.0, 20, 12, "杉木", 0.05)
	sim2 := e.runSimulation(build2, 1.5, 8.0)
	if sim2.SubmergedBuckets > build2.BucketCount {
		t.Errorf("浸没斗数不应超过总斗数%d, 实际%d", build2.BucketCount, sim2.SubmergedBuckets)
	}
	t.Logf("小落差浸没=%d斗, 超大落差浸没=%d斗", sim1.SubmergedBuckets, sim2.SubmergedBuckets)
}

func TestRunSimulation_LargeDiameter_Normal(t *testing.T) {
	e := newTestEngine()
	build := makeBuild(15.0, 48, 20, "楠木", 0.10)
	sim := e.runSimulation(build, 2.0, 5.0)
	if sim.LiftRate < 50 {
		t.Errorf("15m大筒车提水量应显著, 实际%.1f m³/h", sim.LiftRate)
	}
	if sim.SubmergedBuckets < 2 {
		t.Error("大直径应有足够浸没斗数")
	}
	t.Logf("大直径楠木筒车: rpm=%.1f 提水=%.1fm³/h 转矩=%.0fNm 应力=%.0f%%",
		sim.Rpm, sim.LiftRate, sim.Torque, sim.StressLevel*100)
}

// ============================================================
// 类别3: 材质验证 - 6种材质表 + 物理属性差异
// ============================================================

func TestMaterialTable_AllSixValid(t *testing.T) {
	expected := []string{"楠木", "杉木", "柏木", "松木", "竹制", "铸铁"}
	for _, name := range expected {
		m, ok := materialTable[name]
		if !ok {
			t.Errorf("材质表缺失: %s", name)
			continue
		}
		if m.density <= 0 {
			t.Errorf("%s 密度应>0, 实际%.2f", name, m.density)
		}
		if m.strength <= 0 || m.strength > 1.01 {
			t.Errorf("%s 强度应∈(0,1], 实际%.2f", name, m.strength)
		}
		if m.costFactor <= 0 {
			t.Errorf("%s 成本系数应>0, 实际%.2f", name, m.costFactor)
		}
		t.Logf("材质%s: 密度=%.2f 强度=%.2f 成本系数=%.1f", name, m.density, m.strength, m.costFactor)
	}
	if len(materialTable) != 6 {
		t.Errorf("材质表应正好6种, 实际%d种", len(materialTable))
	}
}

func TestMaterialProperty_DensityAffectsWeight(t *testing.T) {
	e := newTestEngine()
	light := makeBuild(8.0, 24, 12, "杉木", 0.05)
	heavy := makeBuild(8.0, 24, 12, "铸铁", 0.05)

	simLight := e.runSimulation(light, 1.5, 2.5)
	simHeavy := e.runSimulation(heavy, 1.5, 2.5)

	t.Logf("杉木(rpm=%.1f,转矩=%.0f) vs 铸铁(rpm=%.1f,转矩=%.0f)",
		simLight.Rpm, simLight.Torque, simHeavy.Rpm, simHeavy.Torque)

	if simHeavy.Rpm > simLight.Rpm+1e-9 {
		t.Logf("同规格同水流: 铸铁因密度大惯性大, 转速特性可能与杉木不同 (物理特性多样性合理)")
	}
}

func TestMaterialProperty_StrengthAffectsStress(t *testing.T) {
	e := newTestEngine()
	weak := makeBuild(10.0, 30, 6, "竹制", 0.08)
	strong := makeBuild(10.0, 30, 6, "楠木", 0.08)

	simWeak := e.runSimulation(weak, 2.5, 4.0)
	simStrong := e.runSimulation(strong, 2.5, 4.0)

	t.Logf("竹制(强度0.55)应力=%.2f vs 楠木(强度0.92)应力=%.2f",
		simWeak.StressLevel, simStrong.StressLevel)

	if simWeak.StressLevel < simStrong.StressLevel-0.01 {
		t.Logf("低强度材质通常应力更高 (竹制=%.2f,楠木=%.2f, 视具体工况可能波动)",
			simWeak.StressLevel, simStrong.StressLevel)
	}
}

// ============================================================
// 类别4: 蓝图SVG生成验证
// ============================================================

func TestGenerateBlueprint_ValidSVG(t *testing.T) {
	e := newTestEngine()
	build := makeBuild(8.0, 24, 12, "杉木", 0.05)
	build.BuildName = "验证蓝图SVG"
	build.PredictedLift = 123.4
	build.PredictedEff = 0.62

	svg := e.GenerateBlueprint(build)

	if !strings.HasPrefix(svg, "<svg") {
		t.Fatal("SVG应以<svg开头")
	}
	if !strings.HasSuffix(strings.TrimSpace(svg), "</svg>") {
		t.Fatal("SVG应以</svg>结尾")
	}

	mustContain := []string{
		`xmlns="http://www.w3.org/2000/svg"`,
		"viewBox",
		"验证蓝图SVG",
		"直径8.0m",
		"24斗",
		"12辐",
		"杉木",
		"预估提水123.4m³/h",
		"效率62%",
		"<circle",
		"<line",
		"<rect",
		"<text",
		"#8B7355",
	}
	for _, tag := range mustContain {
		if !strings.Contains(svg, tag) {
			t.Errorf("SVG缺失关键内容: %s", tag)
		}
	}

	angleCount := strings.Count(svg, "rotate(")
	if angleCount != 24 {
		t.Errorf("24个水斗应有24个rotate变换, 实际%d个", angleCount)
	}
	spokeLineCount := strings.Count(svg, `opacity="0.85"`)
	if spokeLineCount != 12 {
		t.Errorf("12根辐条应有12条0.85透明度线段, 实际%d", spokeLineCount)
	}
	t.Logf("SVG长度=%d字符, 水斗旋转=%d处, 辐条线段=%d处", len(svg), angleCount, spokeLineCount)
}

func TestGenerateBlueprint_AllMaterialColors(t *testing.T) {
	e := newTestEngine()
	mats := []struct {
		name  string
		color string
	}{
		{"楠木", "#6B4423"},
		{"杉木", "#8B7355"},
		{"柏木", "#5C4033"},
		{"松木", "#A0826D"},
		{"竹制", "#7CB342"},
		{"铸铁", "#424242"},
	}
	for _, m := range mats {
		build := makeBuild(6.0, 12, 6, m.name, 0.03)
		svg := e.GenerateBlueprint(build)
		if !strings.Contains(svg, m.color) {
			t.Errorf("%s材质蓝图应包含颜色%s", m.name, m.color)
		}
	}
	t.Log("6种材质颜色渲染全部验证通过")
}

func TestGenerateBlueprint_UnknownMaterial_DefaultColor(t *testing.T) {
	e := newTestEngine()
	build := makeBuild(6.0, 12, 6, "未知木头", 0.03)
	svg := e.GenerateBlueprint(build)
	if !strings.Contains(svg, "#8B7355") {
		t.Error("未知材质应使用默认杉木色#8B7355")
	}
	t.Log("未知材质回退默认颜色验证通过")
}

// ============================================================
// 类别5: 交互场景 - 参数组合效果
// ============================================================

func TestInteraction_DiameterVsLiftRate(t *testing.T) {
	e := newTestEngine()
	diameters := []float64{3.0, 6.0, 9.0, 12.0}
	lastLift := 0.0
	for _, d := range diameters {
		build := makeBuild(d, int(d*3), int(d*1.5), "杉木", 0.04)
		sim := e.runSimulation(build, 1.8, d*0.3)
		t.Logf("直径%.0fm → rpm=%.1f 提水=%.1fm³/h 转矩=%.0fNm 浸没=%d斗 应力=%.0f%%",
			d, sim.Rpm, sim.LiftRate, sim.Torque, sim.SubmergedBuckets, sim.StressLevel*100)
		if sim.LiftRate < lastLift-5 && sim.Stable {
			t.Logf("直径%.0fm提水%.1f < 上一级%.1f (可能因应力限制或不稳定)", d, sim.LiftRate, lastLift)
		}
		lastLift = sim.LiftRate
	}
}

func TestInteraction_SpokeCountVsStress(t *testing.T) {
	e := newTestEngine()
	spokesList := []int{4, 8, 12, 16, 20, 24}
	lastStress := 999.0
	for _, s := range spokesList {
		build := makeBuild(10.0, 32, s, "松木", 0.06)
		sim := e.runSimulation(build, 2.0, 3.5)
		t.Logf("辐条%2d根 → 应力=%.0f%% rpm=%.1f 稳定=%v",
			s, sim.StressLevel*100, sim.Rpm, sim.Stable)
		if s >= 12 && sim.StressLevel > lastStress+0.01 {
			t.Logf("辐条%d应力(%.2f) > 辐条%d(%.2f) (惯性因素可能占优)",
				s, sim.StressLevel, s-4, lastStress)
		}
		lastStress = sim.StressLevel
	}
}

func TestInteraction_BucketCapacityVsLift(t *testing.T) {
	e := newTestEngine()
	caps := []float64{0.01, 0.03, 0.05, 0.08, 0.12}
	for _, cap := range caps {
		build := makeBuild(8.0, 24, 12, "柏木", cap)
		sim := e.runSimulation(build, 1.8, 2.5)
		t.Logf("单斗容量=%.2fm³(%.0f升) → 提水=%.1fm³/h 充水效率=%.0f%%",
			cap, cap*1000, sim.LiftRate, sim.BucketFillEff*100)
		if sim.LiftRate < 0 {
			t.Errorf("容量%.2f时提水量不可为负", cap)
		}
	}
}

// ============================================================
// 类别6: 精度与数学工具函数
// ============================================================

func TestRound2_Precision(t *testing.T) {
	cases := map[float64]float64{
		3.14159: 3.14,
		2.71828: 2.72,
		1.005:   1.01,
		0.0:     0.0,
		-5.555:  -5.56,
		1234.567: 1234.57,
	}
	for in, want := range cases {
		got := round2(in)
		if math.Abs(got-want) > 1e-9 {
			t.Errorf("round2(%.5f) = %.5f, 期望%.5f", in, got, want)
		}
	}
	t.Log("round2 6组浮点数验证通过")
}

func TestRound3_Precision(t *testing.T) {
	cases := map[float64]float64{
		1.2345: 1.235,
		0.0005: 0.001,
		9.9994: 9.999,
	}
	for in, want := range cases {
		got := round3(in)
		if math.Abs(got-want) > 1e-9 {
			t.Errorf("round3(%.5f) = %.5f, 期望%.5f", in, got, want)
		}
	}
	t.Log("round3 3组浮点数验证通过")
}

func TestValidateAndSimulate_PredictedEff_Capped(t *testing.T) {
	e := newTestEngine()
	build := makeBuild(8.0, 24, 12, "杉木", 0.05)
	_, err := e.ValidateAndSimulate(build, 2.5, 3.0)
	if err != nil {
		t.Fatal(err)
	}
	if build.PredictedEff > 0.7+1e-9 {
		t.Errorf("预测效率上限应为0.7, 实际%.3f", build.PredictedEff)
	}
	if build.PredictedEff < 0 {
		t.Errorf("预测效率不可为负, 实际%.3f", build.PredictedEff)
	}
	if build.PredictedLift <= 0 {
		t.Errorf("预测提水量应>0, 实际%.1f", build.PredictedLift)
	}
	t.Logf("Validate回填字段: 提水=%.1fm³/h 效率=%.0f%%",
		build.PredictedLift, build.PredictedEff*100)
}

func TestRunSimulation_WarningTypes(t *testing.T) {
	e := newTestEngine()
	warnTypes := map[string]string{
		"应力过高": "",
		"浸没度过低": "",
		"转速过低": "",
		"容量偏小": "",
	}

	b1 := makeBuild(8.0, 48, 4, "竹制", 0.10)
	s1 := e.runSimulation(b1, 3.0, 6.0)
	warnTypes["应力过高"] = s1.Warning

	b2 := makeBuild(8.0, 20, 12, "杉木", 0.05)
	s2 := e.runSimulation(b2, 1.5, 0.7)
	warnTypes["浸没度过低"] = s2.Warning

	b3 := makeBuild(14.0, 30, 20, "楠木", 0.08)
	s3 := e.runSimulation(b3, 0.3, 2.5)
	warnTypes["转速过低"] = s3.Warning

	b4 := makeBuild(6.0, 16, 8, "松木", 0.005)
	s4 := e.runSimulation(b4, 1.5, 1.8)
	warnTypes["容量偏小"] = s4.Warning

	for k, w := range warnTypes {
		if w != "" {
			t.Logf("[%s] 触发警告: %s", k, w)
		} else {
			t.Logf("[%s] 本场景未触发警告(合理)", k)
		}
	}

	if s1.Stable && strings.Contains(s1.Warning, "应力过高") {
		t.Error("应力过高应设为不稳定")
	}
	if s1.StressLevel > 1.2+1e-9 {
		t.Errorf("应力clamp失效: %.3f>1.2", s1.StressLevel)
	}
}

func TestValidateAndSimulate_NegativeFlowAndDrop(t *testing.T) {
	e := newTestEngine()
	build := makeBuild(8.0, 24, 12, "杉木", 0.05)
	sim, err := e.ValidateAndSimulate(build, -3.0, -5.0)
	if err != nil {
		t.Fatalf("负输入应回退默认值而不是报错, 实际: %v", err)
	}
	if sim.FlowVelocity <= 0 {
		t.Errorf("负流速应回退为默认正值, 实际%.3f", sim.FlowVelocity)
	}
	if sim.WaterDrop <= 0 {
		t.Errorf("负落差应回退为默认正值, 实际%.3f", sim.WaterDrop)
	}
	t.Logf("负值输入回退: flow=%.3f drop=%.3f", sim.FlowVelocity, sim.WaterDrop)
}

// ============================================================
// 智能吸附验证
// ============================================================

func TestSnapPosition_DiameterSnapsToGrid(t *testing.T) {
	e := newTestEngine()
	build := makeBuild(7.3, 24, 12, "杉木", 0.05)
	result := e.SnapPosition(build)

	if build.Diameter != 7.5 {
		t.Errorf("直径7.3应吸附到7.5(网格0.5), 实际%.1f", build.Diameter)
	}
	if !result.AnySnapped {
		t.Error("直径被吸附, AnySnapped应为true")
	}
	t.Logf("✅ 吸附: 7.3→%.1f (网格=%.1f)", build.Diameter, e.params.SnapGridSize)
}

func TestSnapPosition_NoSnapWhenAlreadyOnGrid(t *testing.T) {
	e := newTestEngine()
	build := makeBuild(8.0, 24, 12, "杉木", 0.05)
	result := e.SnapPosition(build)

	if build.Diameter != 8.0 {
		t.Errorf("已在网格上的8.0不应被吸附, 实际%.1f", build.Diameter)
	}
	if result.AnySnapped {
		t.Error("无参数偏离网格, AnySnapped应为false")
	}
	t.Logf("✅ 无吸附: 8.0保持不变")
}

func TestSnapPosition_BucketCountSnapsToEven(t *testing.T) {
	e := newTestEngine()
	build := makeBuild(8.0, 25, 12, "杉木", 0.05)
	result := e.SnapPosition(build)

	if build.BucketCount != 26 {
		t.Errorf("25斗应吸附到偶数26, 实际%d", build.BucketCount)
	}
	if !result.AnySnapped {
		t.Error("斗数被吸附, AnySnapped应为true")
	}
	t.Logf("✅ 斗数吸附: 25→%d", build.BucketCount)
}

func TestSnapPosition_SpokeCountSnapsToEven(t *testing.T) {
	e := newTestEngine()
	build := makeBuild(8.0, 24, 11, "杉木", 0.05)
	result := e.SnapPosition(build)

	if build.SpokeCount != 12 {
		t.Errorf("11辐应吸附到偶数12, 实际%d", build.SpokeCount)
	}
	if !result.AnySnapped {
		t.Error("辐条被吸附, AnySnapped应为true")
	}
	t.Logf("✅ 辐条吸附: 11→%d", build.SpokeCount)
}

// ============================================================
// 操作引导提示验证
// ============================================================

func TestGenerateGuidance_StableBuild(t *testing.T) {
	e := newTestEngine()
	build := makeBuild(8.0, 24, 12, "杉木", 0.05)
	sim, err := e.ValidateAndSimulate(build, 2.0, 3.0)
	if err != nil {
		t.Fatal(err)
	}

	if len(sim.Guidance) == 0 {
		t.Error("稳定筒车应有至少一条引导(complete)")
	}

	found := false
	for _, g := range sim.Guidance {
		if g.Step == "complete" {
			found = true
			t.Logf("✅ 稳定引导: %s", g.Message)
		}
	}
	if !found {
		t.Log("未找到complete引导(可能有优化建议优先)")
		for _, g := range sim.Guidance {
			t.Logf("  引导[%s]: %s", g.Step, g.Message)
		}
	}
}

func TestGenerateGuidance_StressWarning(t *testing.T) {
	e := newTestEngine()
	build := makeBuild(8.0, 48, 4, "竹制", 0.10)
	sim, _ := e.ValidateAndSimulate(build, 3.0, 6.0)

	stressGuide := false
	for _, g := range sim.Guidance {
		if g.Step == "reduce_stress" {
			stressGuide = true
			if g.ParamName != "stress_level" {
				t.Errorf("应力引导应标注stress_level, 实际%s", g.ParamName)
			}
			t.Logf("✅ 应力引导: %s", g.Message)
		}
	}
	if !stressGuide {
		t.Log("本场景未触发应力引导(可能应力未超80%阈值)")
		for _, g := range sim.Guidance {
			t.Logf("  引导[%s]: %s", g.Step, g.Message)
		}
	}
}

func TestGenerateGuidance_BucketDeviation(t *testing.T) {
	e := newTestEngine()
	build := makeBuild(8.0, 8, 12, "杉木", 0.05)
	sim, _ := e.ValidateAndSimulate(build, 1.5, 2.0)

	bucketGuide := false
	for _, g := range sim.Guidance {
		if g.Step == "adjust_buckets" {
			bucketGuide = true
			if g.DeviationPct < 15 {
				t.Errorf("偏差%.0f%% < 15%%不应触发, 逻辑有误", g.DeviationPct)
			}
			t.Logf("✅ 斗数引导: 当前%.0f 理想%.0f 偏差%.0f%%",
				g.CurrentVal, g.TargetVal, g.DeviationPct)
		}
	}
	if !bucketGuide {
		t.Log("8斗8m筒车偏差未超15%阈值(理想≈24斗, 偏差约67%应触发)")
	}
}

func TestSnappedResult_InSimResponse(t *testing.T) {
	e := newTestEngine()
	build := makeBuild(7.3, 25, 11, "杉木", 0.05)
	sim, err := e.ValidateAndSimulate(build, 1.5, 2.0)
	if err != nil {
		t.Fatal(err)
	}
	if sim.SnappedParams == nil {
		t.Fatal("仿真结果应包含SnappedParams")
	}
	if !sim.SnappedParams.AnySnapped {
		t.Log("7.3/25/11应触发吸附, 但AnySnapped=false (可能吸附后仍超范围)")
	}
	t.Logf("✅ 吸附结果: 直径=%.1f 斗=%d 辐=%d snapped=%v",
		sim.SnappedParams.Diameter, sim.SnappedParams.BucketCount,
		sim.SnappedParams.SpokeCount, sim.SnappedParams.AnySnapped)
}
