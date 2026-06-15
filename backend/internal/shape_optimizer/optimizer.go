package shape_optimizer

import (
	"context"
	"log"
	"math"
	"math/rand"
	"sort"
	"time"

	"waterwheel-monitor/internal/config"
	"waterwheel-monitor/internal/database"
	"waterwheel-monitor/internal/metrics"
	"waterwheel-monitor/internal/models"
	"waterwheel-monitor/internal/pipeline"
)

const (
	paramBucketAngle       = iota
	paramBucketDepthRatio
	paramBucketWidthRatio
	paramActiveAngle
	paramBackSweepAngle
	ParamCount
)

var paramBounds = [ParamCount][2]float64{
	{10.0, 60.0},
	{0.3, 0.9},
	{0.5, 1.5},
	{30.0, 90.0},
	{5.0, 45.0},
}

type Individual struct {
	Params       [ParamCount]float64
	Fitness      float64
	SurrogateFit float64
	RealEval     bool
}

type surrogateSample struct {
	vec     [ParamCount]float64
	fitness float64
}

type ShapeOptimizer struct {
	db               *database.Database
	chans            *pipeline.Channels
	params           *config.OptimizerParams
	hydraulicParams  *config.HydraulicParams
	surrogateSamples []surrogateSample
	rand             *rand.Rand
	workers          int
}

func New(db *database.Database, chans *pipeline.Channels,
	params *config.OptimizerParams, hp *config.HydraulicParams) *ShapeOptimizer {
	return &ShapeOptimizer{
		db:               db,
		chans:            chans,
		params:           params,
		hydraulicParams:  hp,
		surrogateSamples: make([]surrogateSample, 0, 1024),
		rand:             rand.New(rand.NewSource(time.Now().UnixNano())),
		workers:          1,
	}
}

func (so *ShapeOptimizer) Start(ctx context.Context) {
	for i := 0; i < so.workers; i++ {
		go so.worker(ctx, i)
	}
	log.Printf("[Shape Optimizer] Started with %d worker", so.workers)
}

func (so *ShapeOptimizer) worker(ctx context.Context, id int) {
	for {
		select {
		case <-ctx.Done():
			log.Printf("[Shape Optimizer] Worker %d stopped", id)
			return
		case req, ok := <-so.chans.OptimizeReqCh:
			if !ok {
				log.Printf("[Shape Optimizer] Worker %d: OptimizeReqCh closed", id)
				return
			}
			start := time.Now()
			result := so.Optimize(req.Wheel, req.Data)
			metrics.ObserveOptimizationDuration(time.Since(start))
			metrics.IncOptimization()
			if err := so.db.InsertOptimizationResult(context.Background(), result); err != nil {
				log.Printf("[Shape Optimizer] DB insert error (wheel=%d): %v", req.Wheel.ID, err)
			}
			select {
			case req.ResultCh <- result:
			default:
			}
		}
	}
}

func (so *ShapeOptimizer) Optimize(wheel *models.Waterwheel, data *models.TelemetryData) *models.OptimizationResult {
	ctx := context.Background()
	p := so.params

	population := make([]Individual, p.PopulationSize)
	bestFitness := 0.0
	var bestParams [ParamCount]float64

	for i := range population {
		so.randomIndividual(&population[i])
		so.surrogateEvaluate(&population[i])
	}

	so.initSurrogate(wheel, data)
	bestFitness, bestParams = so.evaluateTopN(ctx, wheel, data, population, p.ElitismCount, bestFitness, bestParams)

	for gen := 0; gen < p.Generations; gen++ {
		sort.SliceStable(population, func(i, j int) bool {
			return population[i].effectiveFitness() > population[j].effectiveFitness()
		})

		if gen%10 == 0 {
			bestFitness, bestParams = so.updateBest(population, bestFitness, bestParams)
		}

		so.surrogateEvaluateAll(population)

		sort.SliceStable(population, func(i, j int) bool {
			if population[i].SurrogateFit != population[j].SurrogateFit {
				return population[i].SurrogateFit > population[j].SurrogateFit
			}
			return i < j
		})

		realCount := int(float64(p.PopulationSize) * p.RealEvalRatio)
		exploreStart := int(float64(p.PopulationSize) * p.ExploreRatio)
		bestFitness, bestParams = so.evaluateTopN(ctx, wheel, data, population, p.ElitismCount, bestFitness, bestParams)

		newPop := make([]Individual, p.PopulationSize)
		for i := 0; i < p.ElitismCount && i < len(population); i++ {
			newPop[i] = population[i]
		}

		for i := p.ElitismCount; i < p.PopulationSize; i++ {
			p1 := so.tournamentSelect(population)
			p2 := so.tournamentSelect(population)
			child := so.blxAlphaCrossover(p1, p2)
			so.gaussianMutate(&child)
			newPop[i] = child
		}

		_ = realCount
		_ = exploreStart

		for i := p.ElitismCount; i < p.PopulationSize; i++ {
			so.surrogateEvaluate(&newPop[i])
		}
		population = newPop
	}

	sort.SliceStable(population, func(i, j int) bool {
		return population[i].effectiveFitness() > population[j].effectiveFitness()
	})

	bestFitness, bestParams = so.updateBest(population, bestFitness, bestParams)
	return so.buildResult(wheel, data, bestFitness, bestParams)
}

func (so *ShapeOptimizer) initSurrogate(wheel *models.Waterwheel, data *models.TelemetryData) {
	for i := 0; i < 20; i++ {
		var ind Individual
		so.randomIndividual(&ind)
		so.evaluateIndividual(wheel, data, &ind)
		so.addSurrogateSample(&ind)
	}
}

func (so *ShapeOptimizer) evaluateTopN(ctx context.Context, wheel *models.Waterwheel, data *models.TelemetryData,
	population []Individual, n int, bestFit float64, bestParams [ParamCount]float64) (float64, [ParamCount]float64) {
	if n > len(population) {
		n = len(population)
	}
	for i := 0; i < n; i++ {
		if !population[i].RealEval {
			so.evaluateIndividual(wheel, data, &population[i])
			so.addSurrogateSample(&population[i])
		}
		if population[i].Fitness > bestFit {
			bestFit = population[i].Fitness
			copy(bestParams[:], population[i].Params[:])
		}
	}
	_ = ctx
	return bestFit, bestParams
}

func (so *ShapeOptimizer) updateBest(population []Individual, bestFit float64, bestParams [ParamCount]float64) (float64, [ParamCount]float64) {
	for i := range population {
		if population[i].RealEval && population[i].Fitness > bestFit {
			bestFit = population[i].Fitness
			copy(bestParams[:], population[i].Params[:])
		}
	}
	return bestFit, bestParams
}

func (so *ShapeOptimizer) addSurrogateSample(ind *Individual) {
	vec := so.paramsToVector(&ind.Params)
	so.surrogateSamples = append(so.surrogateSamples, surrogateSample{
		vec:     vec,
		fitness: ind.Fitness,
	})
}

func (so *ShapeOptimizer) randomIndividual(ind *Individual) {
	for i := 0; i < ParamCount; i++ {
		ind.Params[i] = paramBounds[i][0] + so.rand.Float64()*(paramBounds[i][1]-paramBounds[i][0])
	}
	ind.RealEval = false
}

func (so *ShapeOptimizer) paramsToVector(p *[ParamCount]float64) (out [ParamCount]float64) {
	for i := 0; i < ParamCount; i++ {
		range_ := paramBounds[i][1] - paramBounds[i][0]
		if range_ > 0 {
			out[i] = (p[i] - paramBounds[i][0]) / range_
		}
	}
	return
}

func (so *ShapeOptimizer) surrogateEvaluateAll(population []Individual) {
	for i := range population {
		if !population[i].RealEval {
			so.surrogateEvaluate(&population[i])
		}
	}
}

func (so *ShapeOptimizer) surrogateEvaluate(ind *Individual) {
	if len(so.surrogateSamples) < 1 {
		ind.SurrogateFit = 0.0
		return
	}

	vec := so.paramsToVector(&ind.Params)
	K := so.params.SurrogateK
	if K > len(so.surrogateSamples) {
		K = len(so.surrogateSamples)
	}

	type distItem struct {
		dist    float64
		fitness float64
	}
	items := make([]distItem, len(so.surrogateSamples))
	for i, s := range so.surrogateSamples {
		d := 0.0
		for j := 0; j < ParamCount; j++ {
			diff := vec[j] - s.vec[j]
			d += diff * diff
		}
		items[i] = distItem{dist: d, fitness: s.fitness}
	}

	sort.Slice(items, func(i, j int) bool { return items[i].dist < items[j].dist })

	var wSum, fSum float64
	for i := 0; i < K; i++ {
		w := 1.0 / (1e-9 + items[i].dist)
		wSum += w
		fSum += w * items[i].fitness
	}
	if wSum > 0 {
		ind.SurrogateFit = fSum / wSum
	}
}

func (ind *Individual) effectiveFitness() float64 {
	if ind.RealEval {
		return ind.Fitness + 1e-6
	}
	return ind.SurrogateFit
}

func (so *ShapeOptimizer) evaluateIndividual(wheel *models.Waterwheel, data *models.TelemetryData, ind *Individual) {
	hp := so.hydraulicParams

	p := ind.Params[:]
	bucketAngleDeg := p[paramBucketAngle]
	depthRatio := p[paramBucketDepthRatio]
	widthRatio := p[paramBucketWidthRatio]
	activeAngleDeg := p[paramActiveAngle]
	_ = p[paramBackSweepAngle]

	effectiveBucketCap := wheel.BucketCapacity * depthRatio * widthRatio
	bucketAngle := bucketAngleDeg * math.Pi / 180.0
	activeAngle := activeAngleDeg * math.Pi / 180.0

	radius := wheel.Diameter / 2.0
	omega := data.RotationSpeed * 2 * math.Pi / 60.0

	submersionRatio := math.Min(1, data.WaterLevelDrop/wheel.Diameter)
	fillEff := so.calcDynamicFill(wheel, data)
	submergedBuckets := int(math.Max(1, float64(wheel.BucketCount)*activeAngle/(2*math.Pi)))

	bucketForce := hp.WaterDensity * hp.Gravity * effectiveBucketCap * fillEff
	effectiveRadius := radius * hp.EffectiveArmRatio * math.Sin(bucketAngle*0.5)

	impactForce := 0.5 * hp.WaterDensity * data.FlowVelocity * data.FlowVelocity *
		effectiveBucketCap * fillEff * 0.5 / radius

	torqueIn := float64(submergedBuckets)*bucketForce*effectiveRadius +
		float64(wheel.BucketCount/4)*impactForce*radius

	activeBuckets := int(math.Max(1, float64(wheel.BucketCount)*submersionRatio))
	dragForce := 0.5 * hp.WaterDensity * omega * omega * radius * radius *
		effectiveBucketCap * float64(activeBuckets) * 0.02

	netTorque := torqueIn - dragForce
	theoreticalRate := float64(wheel.BucketCount) * effectiveBucketCap * fillEff *
		data.RotationSpeed * 60.0

	actualRate := theoreticalRate
	if netTorque < 0 {
		actualRate *= 0.3
	} else if torqueIn > 0 {
		eff := netTorque / torqueIn
		actualRate *= math.Min(1, math.Max(0.3, eff))
	}

	beforeImprovement := calculateBaseline(wheel, data, hp)
	fitness := 0.0
	if beforeImprovement > 0 {
		improvement := (actualRate - beforeImprovement) / beforeImprovement
		fitness = math.Min(2.0, math.Max(-0.5, improvement)) * 100.0
	} else {
		fitness = math.Log(1.0 + actualRate*1000.0)
	}

	ind.Fitness = fitness
	ind.RealEval = true
}

func (so *ShapeOptimizer) calcDynamicFill(wheel *models.Waterwheel, data *models.TelemetryData) float64 {
	hp := so.hydraulicParams
	omega := data.RotationSpeed * 2 * math.Pi / 60.0
	if omega <= 0 {
		return hp.MaxFillEfficiency
	}
	submersionRatio := math.Min(1, data.WaterLevelDrop/wheel.Diameter)
	submergedAngle := 2 * math.Asin(math.Sqrt(submersionRatio))
	submergedAngle = math.Max(0.3, math.Min(math.Pi*0.8, submergedAngle))
	immersionTime := submergedAngle / omega
	fillEff := 1.0 - math.Exp(-immersionTime/hp.FillTimeConstant)
	return hp.MinFillEfficiency + fillEff*(hp.MaxFillEfficiency-hp.MinFillEfficiency)
}

func calculateBaseline(wheel *models.Waterwheel, data *models.TelemetryData, hp *config.HydraulicParams) float64 {
	fillEff := 0.38
	radius := wheel.Diameter / 2.0
	omega := data.RotationSpeed * 2 * math.Pi / 60.0
	if omega > 0 {
		submersionRatio := math.Min(1, data.WaterLevelDrop/wheel.Diameter)
		submergedAngle := 2 * math.Asin(math.Sqrt(submersionRatio))
		submergedAngle = math.Max(0.3, math.Min(math.Pi*0.8, submergedAngle))
		immersionTime := submergedAngle / omega
		fillEff = 1.0 - math.Exp(-immersionTime/hp.FillTimeConstant)
		fillEff = hp.MinFillEfficiency + fillEff*(hp.MaxFillEfficiency-hp.MinFillEfficiency)
	}
	volumePerRotation := float64(wheel.BucketCount) * hp.ActiveBucketRatio * wheel.BucketCapacity * fillEff
	return volumePerRotation * data.RotationSpeed * 60.0
}

func (so *ShapeOptimizer) tournamentSelect(population []Individual) *Individual {
	p := so.params
	tournamentSize := 3
	bestIdx := so.rand.Intn(len(population))
	for i := 1; i < tournamentSize; i++ {
		idx := so.rand.Intn(len(population))
		if population[idx].effectiveFitness() > population[bestIdx].effectiveFitness() {
			bestIdx = idx
		}
	}
	_ = p
	return &population[bestIdx]
}

func (so *ShapeOptimizer) blxAlphaCrossover(p1, p2 *Individual) Individual {
	var child Individual
	alpha := 0.5
	for i := 0; i < ParamCount; i++ {
		minVal := math.Min(p1.Params[i], p2.Params[i])
		maxVal := math.Max(p1.Params[i], p2.Params[i])
		diff := maxVal - minVal
		lo := minVal - alpha*diff
		hi := maxVal + alpha*diff
		child.Params[i] = lo + so.rand.Float64()*(hi-lo)
		child.Params[i] = math.Max(paramBounds[i][0], math.Min(paramBounds[i][1], child.Params[i]))
	}
	child.RealEval = false
	return child
}

func (so *ShapeOptimizer) gaussianMutate(ind *Individual) {
	p := so.params
	for i := 0; i < ParamCount; i++ {
		if so.rand.Float64() < p.MutationRate {
			sigma := (paramBounds[i][1] - paramBounds[i][0]) * 0.1
			ind.Params[i] += so.rand.NormFloat64() * sigma
			ind.Params[i] = math.Max(paramBounds[i][0], math.Min(paramBounds[i][1], ind.Params[i]))
			ind.RealEval = false
		}
	}
}

func (so *ShapeOptimizer) buildResult(wheel *models.Waterwheel, data *models.TelemetryData,
	bestFitness float64, bestParams [ParamCount]float64) *models.OptimizationResult {
	p := bestParams[:]
	before := calculateBaseline(wheel, data, so.hydraulicParams)

	effectiveBucketCap := wheel.BucketCapacity * p[paramBucketDepthRatio] * p[paramBucketWidthRatio]
	fillEff := so.calcDynamicFill(wheel, data)
	after := float64(wheel.BucketCount) * so.hydraulicParams.ActiveBucketRatio *
		effectiveBucketCap * fillEff * data.RotationSpeed * 60.0

	improvement := 0.0
	if before > 0 {
		improvement = (after - before) / before
	}

	return &models.OptimizationResult{
		ID:                  0,
		WaterwheelID:        wheel.ID,
		Time:                time.Now(),
		OptimalBucketAngle:  p[paramBucketAngle],
		OptimalDepthRatio:   p[paramBucketDepthRatio],
		OptimalWidthRatio:   p[paramBucketWidthRatio],
		PredictedLift:       after,
		PredictedImprovement: improvement * 100.0,
		Fitness:             bestFitness,
		Generations:         so.params.Generations,
	}
}
