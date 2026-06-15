package virtual_builder

import (
	"context"
	"fmt"
	"math"
	"time"

	"waterwheel-monitor/internal/config"
	"waterwheel-monitor/internal/database"
	"waterwheel-monitor/internal/models"
)

type BuildEngine struct {
	db     *database.Database
	params *config.BuildParams
}

func NewBuildEngine(db *database.Database, params *config.BuildParams) *BuildEngine {
	return &BuildEngine{db: db, params: params}
}

type materialDensity struct {
	name     string
	density  float64
	strength float64
	costFactor float64
}

var materialTable = map[string]materialDensity{
	"楠木":  {density: 0.61, strength: 0.92, costFactor: 1.8},
	"杉木":  {density: 0.38, strength: 0.70, costFactor: 1.0},
	"柏木":  {density: 0.58, strength: 0.85, costFactor: 1.5},
	"松木":  {density: 0.45, strength: 0.65, costFactor: 0.9},
	"竹制":  {density: 0.65, strength: 0.55, costFactor: 0.5},
	"铸铁":  {density: 7.20, strength: 1.00, costFactor: 3.2},
}

func (e *BuildEngine) ValidateAndSimulate(build *models.VirtualBuild, flowVelocity, waterDrop float64) (*models.BuildSimulation, error) {
	snapped := e.SnapPosition(build)

	if build.Diameter < e.params.MinDiameterM || build.Diameter > e.params.MaxDiameterM {
		return nil, fmt.Errorf("直径 %.1fm 超出允许范围 (%.1f ~ %.1f)", build.Diameter, e.params.MinDiameterM, e.params.MaxDiameterM)
	}
	if build.BucketCount < e.params.MinBuckets || build.BucketCount > e.params.MaxBuckets {
		return nil, fmt.Errorf("水斗数 %d 超出允许范围 (%d ~ %d)", build.BucketCount, e.params.MinBuckets, e.params.MaxBuckets)
	}
	if build.SpokeCount < e.params.MinSpokes || build.SpokeCount > e.params.MaxSpokes {
		return nil, fmt.Errorf("辐条数 %d 超出允许范围 (%d ~ %d)", build.SpokeCount, e.params.MinSpokes, e.params.MaxSpokes)
	}
	if _, ok := materialTable[build.Material]; !ok {
		return nil, fmt.Errorf("未知材质: %s", build.Material)
	}

	if flowVelocity <= 0 {
		flowVelocity = e.params.DefaultFlowVelocity
	}
	if waterDrop <= 0 {
		waterDrop = e.params.DefaultWaterDrop
	}

	sim := e.runSimulation(build, flowVelocity, waterDrop)
	sim.SnappedParams = snapped
	sim.Guidance = e.generateGuidance(build, sim)

	build.PredictedLift = sim.LiftRate
	build.PredictedEff = sim.BucketFillEff * sim.StressLevel / 2.0
	if build.PredictedEff > 0.7 {
		build.PredictedEff = 0.7
	}

	return sim, nil
}

func (e *BuildEngine) runSimulation(build *models.VirtualBuild, flowVelocity, waterDrop float64) *models.BuildSimulation {
	radius := build.Diameter / 2.0
	mat := materialTable[build.Material]

	wheelWeight := math.Pi * 2 * radius * mat.density * 0.08
	totalBucketWeight := float64(build.BucketCount) * build.BucketCapacity * 1000.0 * 0.15
	structWeight := wheelWeight + totalBucketWeight*0.05
	_ = structWeight

	submergedFraction := waterDrop / build.Diameter
	if submergedFraction < 0.05 {
		submergedFraction = 0.05
	}
	if submergedFraction > 0.8 {
		submergedFraction = 0.8
	}
	submergedBuckets := int(math.Ceil(float64(build.BucketCount) * submergedFraction * 0.5))

	bucketAngleRad := 2 * math.Pi / float64(build.BucketCount)
	immerseTime := 0.0
	if flowVelocity > 0 {
		immerseTime = (bucketAngleRad * radius) / (flowVelocity * 0.7)
	}
	fillEff := 0.15 + 0.77*(1.0-math.Exp(-immerseTime/0.15))
	if fillEff > 0.95 {
		fillEff = 0.95
	}

	effectiveFlowFactor := 0.65 + flowVelocity*0.15
	if effectiveFlowFactor > 1.2 {
		effectiveFlowFactor = 1.2
	}

	gravityTorque := 0.0
	impactTorque := 0.0
	for i := 0; i < submergedBuckets; i++ {
		angle := float64(i) * bucketAngleRad
		if angle < math.Pi/2.0 {
			leverFactor := math.Sin(angle + 0.3)
		} else {
			leverFactor := 1.0 - (angle-math.Pi/2.0)/math.Pi
		}
		if leverFactor < 0 {
			leverFactor = 0
		}
		perBucketWater := build.BucketCapacity * fillEff * (float64(submergedBuckets-i) / float64(submergedBuckets))
		gravityTorque += perBucketWater * 9.81 * 1000.0 * radius * 0.85 * leverFactor
		impactTorque += flowVelocity * flowVelocity * 0.5 * 1000.0 * build.BucketCapacity * 0.2 * leverFactor
	}

	totalTorque := gravityTorque + impactTorque

	inertia := 0.5 * wheelWeight * radius * radius
	inertia += float64(build.SpokeCount) * (wheelWeight / float64(build.SpokeCount+build.BucketCount)) * radius * radius / 3.0

	frictionFactor := 0.08 + (1.0-mat.strength)*0.05 + float64(build.BucketCount)*0.0005
	frictionTorque := frictionFactor * structWeight * 9.81 * radius * 0.02

	netTorque := totalTorque - frictionTorque
	if netTorque <= 0 {
		return &models.BuildSimulation{
			FlowVelocity:     flowVelocity,
			WaterDrop:        waterDrop,
			Rpm:              0,
			LiftRate:         0,
			Torque:           0,
			BucketFillEff:    fillEff,
			SubmergedBuckets: submergedBuckets,
			StressLevel:      1.0,
			Stable:           false,
			Warning:          "水流能量不足，无法驱动筒车，请加大直径或增加浸没度",
		}
	}

	angularAccel := netTorque / inertia
	maxAngVel := math.Sqrt(2 * angularAccel * (2 * math.Pi))
	if maxAngVel*radius > flowVelocity*2.0 {
		maxAngVel = (flowVelocity * 2.0) / radius
	}

	rpm := maxAngVel * 60 / (2 * math.Pi)
	if rpm > 25 {
		rpm = 25
	}

	liftPerRevolution := float64(build.BucketCount) * build.BucketCapacity * fillEff * 0.6
	liftRate := liftPerRevolution * rpm
	_ = liftPerRevolution

	stressLevel := 0.0
	stressLevel += netTorque / (float64(build.SpokeCount) * mat.strength * 5000.0)
	stressLevel += (rpm / 25.0) * 0.3
	if submergedFraction > 0.55 {
		stressLevel += (submergedFraction - 0.55) * 0.8
	}
	if stressLevel > 1.2 {
		stressLevel = 1.2
	}

	warn := ""
	stable := true
	if stressLevel > e.params.StressLimit {
		warn = fmt.Sprintf("结构应力过高 (%.0f%%)，建议增强辐条数或改用高强度材质", stressLevel*100)
		stable = false
	} else if submergedFraction < 0.12 {
		warn = fmt.Sprintf("浸没度过低 (%.0f%%)，水斗充水不足，建议降低安装高度", submergedFraction*100)
	} else if rpm < 2.0 {
		warn = "转速过低 ( < 2 rpm)，提水效率受限"
	} else if build.BucketCapacity*1000.0 < 10.0 {
		warn = "单斗容量偏小，建议增大水斗容积"
	}

	return &models.BuildSimulation{
		FlowVelocity:     round3(flowVelocity),
		WaterDrop:        round3(waterDrop),
		Rpm:              round2(rpm),
		LiftRate:         round2(liftRate),
		Torque:           round2(netTorque),
		BucketFillEff:    round2(fillEff),
		SubmergedBuckets: submergedBuckets,
		StressLevel:      round2(stressLevel),
		Stable:           stable,
		Warning:          warn,
	}
}

func (e *BuildEngine) SaveBuild(ctx context.Context, build *models.VirtualBuild) (int64, error) {
	if build.UserID == "" {
		build.UserID = "anonymous"
	}
	if build.BuildName == "" {
		build.BuildName = fmt.Sprintf("我的筒车_%d", time.Now().Unix()%10000)
	}
	build.CreatedAt = time.Now()
	return e.db.SaveVirtualBuild(ctx, build)
}

func (e *BuildEngine) ListBuilds(ctx context.Context, userID string, onlyPublic bool, limit int) ([]models.VirtualBuild, error) {
	if limit <= 0 {
		limit = 50
	}
	return e.db.ListVirtualBuilds(ctx, userID, onlyPublic, limit)
}

func (e *BuildEngine) LikeBuild(ctx context.Context, buildID int64) error {
	return e.db.IncrementBuildLikes(ctx, buildID)
}

func (e *BuildEngine) GenerateBlueprint(build *models.VirtualBuild) string {
	w := 800.0
	h := 500.0
	cx := w / 2.0
	cy := h / 2.0
	r := math.Min(cx, cy) * 0.42
	matColor := map[string]string{
		"楠木": "#6B4423",
		"杉木": "#8B7355",
		"柏木": "#5C4033",
		"松木": "#A0826D",
		"竹制": "#7CB342",
		"铸铁": "#424242",
	}
	c := "#8B7355"
	if mc, ok := matColor[build.Material]; ok {
		c = mc
	}

	svg := fmt.Sprintf(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 %.0f %.0f" width="%.0f" height="%.0f">`, w, h, w, h)
	svg += `<rect width="100%" height="100%" fill="#E8F4F8"/>`
	svg += `<defs><linearGradient id="water" x1="0" x2="0" y1="1" y2="0"><stop offset="0%" stop-color="#0D47A1"/><stop offset="100%" stop-color="#64B5F6"/></linearGradient></defs>`

	svg += fmt.Sprintf(`<rect x="0" y="%.0f" width="%.0f" height="%.0f" fill="url(#water)" opacity="0.6"/>`, cy+r*0.7, w, h-cy-r*0.7)

	svg += fmt.Sprintf(`<line x1="%.0f" y1="%.0f" x2="%.0f" y2="%.0f" stroke="#5D4037" stroke-width="10" stroke-linecap="round"/>`, cx-r*0.85, cy+r*0.85, cx, cy)
	svg += fmt.Sprintf(`<line x1="%.0f" y1="%.0f" x2="%.0f" y2="%.0f" stroke="#5D4037" stroke-width="10" stroke-linecap="round"/>`, cx+r*0.85, cy+r*0.85, cx, cy)

	svg += fmt.Sprintf(`<circle cx="%.0f" cy="%.0f" r="%.0f" fill="none" stroke="%s" stroke-width="8"/>`, cx, cy, r, c)
	svg += fmt.Sprintf(`<circle cx="%.0f" cy="%.0f" r="%.0f" fill="none" stroke="%s" stroke-width="4" opacity="0.6"/>`, cx, cy, r*0.5, c)
	svg += fmt.Sprintf(`<circle cx="%.0f" cy="%.0f" r="%.0f" fill="%s"/>`, cx, cy, 12.0, c)

	for i := 0; i < build.SpokeCount; i++ {
		ang := float64(i) * 2 * math.Pi / float64(build.SpokeCount)
		x2 := cx + r*0.5*math.Cos(ang)
		y2 := cy + r*0.5*math.Sin(ang)
		svg += fmt.Sprintf(`<line x1="%.0f" y1="%.0f" x2="%.0f" y2="%.0f" stroke="%s" stroke-width="5" opacity="0.85"/>`, cx, cy, x2, y2, c)
	}

	bucketAng := 2 * math.Pi / float64(build.BucketCount)
	for i := 0; i < build.BucketCount; i++ {
		ang := float64(i)*bucketAng + bucketAng/2.0
		bx := cx + r*math.Cos(ang)
		by := cy + r*math.Sin(ang)
		bw := r * 0.12
		bh := bw * 1.4
		deg := ang * 180 / math.Pi
		svg += fmt.Sprintf(`<rect x="%.0f" y="%.0f" width="%.0f" height="%.0f" fill="%s" stroke="#3E2723" stroke-width="2" transform="rotate(%.0f %.0f %.0f)" opacity="0.92"/>`,
			bx-bw/2, by-bh/3, bw, bh, c, deg, bx, by)
	}

	svg += fmt.Sprintf(`<text x="%.0f" y="%.0f" font-family="Microsoft YaHei" font-size="16" text-anchor="middle" fill="#263238" font-weight="bold">%s</text>`, cx, 35, build.BuildName)
	info := fmt.Sprintf("直径%.1fm | %d斗 | %d辐 | %s | 预估提水%.1fm³/h | 效率%.0f%%",
		build.Diameter, build.BucketCount, build.SpokeCount, build.Material, build.PredictedLift, build.PredictedEff*100)
	svg += fmt.Sprintf(`<text x="%.0f" y="%.0f" font-family="Microsoft YaHei" font-size="14" text-anchor="middle" fill="#37474F">%s</text>`, cx, h-25, info)

	svg += `</svg>`
	return svg
}

func round2(v float64) float64 { return math.Round(v*100) / 100 }
func round3(v float64) float64 { return math.Round(v*1000) / 1000 }

func (e *BuildEngine) SnapPosition(build *models.VirtualBuild) *models.SnappedResult {
	grid := e.params.SnapGridSize
	if grid <= 0 {
		grid = 0.5
	}
	threshold := e.params.SnapThreshold
	if threshold <= 0 {
		threshold = 0.3
	}

	originalDiameter := build.Diameter
	originalBuckets := build.BucketCount
	originalSpokes := build.SpokeCount

	snappedDiameter := math.Round(build.Diameter/grid) * grid
	if math.Abs(build.Diameter-snappedDiameter) < threshold*grid {
		build.Diameter = round2(snappedDiameter)
	}

	snappedBuckets := int(math.Round(float64(build.BucketCount)/2.0) * 2.0)
	if math.Abs(float64(build.BucketCount-snappedBuckets)) <= 1 {
		build.BucketCount = snappedBuckets
	}

	snappedSpokes := int(math.Round(float64(build.SpokeCount)/2.0) * 2.0)
	if math.Abs(float64(build.SpokeCount-snappedSpokes)) <= 1 {
		build.SpokeCount = snappedSpokes
	}

	anySnapped := build.Diameter != originalDiameter ||
		build.BucketCount != originalBuckets ||
		build.SpokeCount != originalSpokes

	return &models.SnappedResult{
		Diameter:    build.Diameter,
		BucketCount: build.BucketCount,
		SpokeCount:  build.SpokeCount,
		AnySnapped:  anySnapped,
	}
}

func (e *BuildEngine) generateGuidance(build *models.VirtualBuild, sim *models.BuildSimulation) []models.BuildGuidance {
	var guides []models.BuildGuidance

	idealBucketPerMeter := 3.0
	idealBuckets := int(math.Round(build.Diameter * idealBucketPerMeter))
	if idealBuckets < e.params.MinBuckets {
		idealBuckets = e.params.MinBuckets
	}
	if idealBuckets > e.params.MaxBuckets {
		idealBuckets = e.params.MaxBuckets
	}
	bucketDev := 0.0
	if idealBuckets > 0 {
		bucketDev = math.Abs(float64(build.BucketCount-idealBuckets)) / float64(idealBuckets) * 100
	}
	if bucketDev > 15 {
		guides = append(guides, models.BuildGuidance{
			Step:         "adjust_buckets",
			Message:      fmt.Sprintf("当前%d斗偏离理想%d斗(%.0f%%)，建议调整水斗数以匹配直径", build.BucketCount, idealBuckets, bucketDev),
			ParamName:    "bucket_count",
			CurrentVal:   float64(build.BucketCount),
			TargetVal:    float64(idealBuckets),
			DeviationPct: round2(bucketDev),
		})
	}

	idealSpokePerMeter := 1.5
	idealSpokes := int(math.Round(build.Diameter * idealSpokePerMeter))
	if idealSpokes < e.params.MinSpokes {
		idealSpokes = e.params.MinSpokes
	}
	if idealSpokes > e.params.MaxSpokes {
		idealSpokes = e.params.MaxSpokes
	}
	spokeDev := 0.0
	if idealSpokes > 0 {
		spokeDev = math.Abs(float64(build.SpokeCount-idealSpokes)) / float64(idealSpokes) * 100
	}
	if spokeDev > 20 {
		guides = append(guides, models.BuildGuidance{
			Step:         "adjust_spokes",
			Message:      fmt.Sprintf("当前%d辐偏离理想%d辐(%.0f%%)，辐条过少结构不稳，过多增加重量", build.SpokeCount, idealSpokes, spokeDev),
			ParamName:    "spoke_count",
			CurrentVal:   float64(build.SpokeCount),
			TargetVal:    float64(idealSpokes),
			DeviationPct: round2(spokeDev),
		})
	}

	if sim.StressLevel > e.params.StressLimit*0.8 {
		guides = append(guides, models.BuildGuidance{
			Step:         "reduce_stress",
			Message:      fmt.Sprintf("应力%.0f%%接近阈值%.0f%%，建议增加辐条或改用高强度材质", sim.StressLevel*100, e.params.StressLimit*100),
			ParamName:    "stress_level",
			CurrentVal:   sim.StressLevel,
			TargetVal:    e.params.StressLimit * 0.7,
			DeviationPct: round2((sim.StressLevel - e.params.StressLimit*0.7) / e.params.StressLimit * 100),
		})
	}

	if sim.Rpm > 0 && sim.Rpm < 3.0 {
		guides = append(guides, models.BuildGuidance{
			Step:         "increase_rpm",
			Message:      "转速过低，建议增大直径或提高浸没度以获取更多水力",
			ParamName:    "rpm",
			CurrentVal:   sim.Rpm,
			TargetVal:    5.0,
			DeviationPct: round2((5.0 - sim.Rpm) / 5.0 * 100),
		})
	}

	if len(guides) == 0 && sim.Stable {
		guides = append(guides, models.BuildGuidance{
			Step:      "complete",
			Message:   "筒车参数合理，仿真稳定，可保存作品或调整细节优化",
			ParamName: "",
		})
	}

	return guides
}
