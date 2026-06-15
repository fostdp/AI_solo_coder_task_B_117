package optimizer

import (
	"math"
	"math/rand"
	"sort"
	"time"

	"waterwheel-monitor/internal/models"
)

type Individual struct {
	Params        models.BucketParams
	Fitness       float64
	IsRealEvaluated bool
}

type surrogateSample struct {
	params  [5]float64
	fitness float64
}

type GAOptimizer struct {
	populationSize   int
	generations      int
	mutationRate     float64
	crossoverRate    float64
	rand             *rand.Rand
	surrogateSamples []surrogateSample
	surrogateK       int
	realEvalRatio    float64
}

func NewGAOptimizer() *GAOptimizer {
	return &GAOptimizer{
		populationSize: 100,
		generations:    150,
		mutationRate:   0.15,
		crossoverRate:  0.85,
		rand:           rand.New(rand.NewSource(time.Now().UnixNano())),
		surrogateK:     7,
		realEvalRatio:  0.3,
	}
}

func (ga *GAOptimizer) Optimize(wheel *models.Waterwheel, currentData *models.TelemetryData) *models.OptimizationResult {
	ga.surrogateSamples = make([]surrogateSample, 0, 500)

	baseline := ga.realEvaluateAndStore(wheel, currentData, models.BucketParams{
		Width:     wheel.BucketCapacity * 10,
		Depth:     wheel.BucketCapacity * 5,
		Height:    wheel.BucketCapacity * 8,
		Angle:     15.0,
		Curvature: 0.3,
	})

	population := ga.initializePopulation()
	fitnessHistory := make([]float64, 0, ga.generations)

	best := population[0]
	best.Fitness = ga.realEvaluateAndStore(wheel, currentData, best.Params)
	best.IsRealEvaluated = true

	for gen := 0; gen < ga.generations; gen++ {
		ga.surrogateEvaluateAll(population)

		sort.Slice(population, func(i, j int) bool {
			return population[i].Fitness > population[j].Fitness
		})

		realEvalCount := int(float64(ga.populationSize) * ga.realEvalRatio)
		eliteCount := 5
		randomCount := int(float64(realEvalCount) * 0.3)
		topCount := realEvalCount - eliteCount - randomCount
		if topCount < 0 {
			topCount = 0
		}

		for i := 0; i < eliteCount && i < len(population); i++ {
			if !population[i].IsRealEvaluated {
				population[i].Fitness = ga.realEvaluateAndStore(wheel, currentData, population[i].Params)
				population[i].IsRealEvaluated = true
			}
		}

		for i := eliteCount; i < eliteCount+topCount && i < len(population); i++ {
			if !population[i].IsRealEvaluated {
				population[i].Fitness = ga.realEvaluateAndStore(wheel, currentData, population[i].Params)
				population[i].IsRealEvaluated = true
			}
		}

		for i := 0; i < randomCount; i++ {
			idx := eliteCount + topCount + ga.rand.Intn(len(population)-eliteCount-topCount)
			if !population[idx].IsRealEvaluated {
				population[idx].Fitness = ga.realEvaluateAndStore(wheel, currentData, population[idx].Params)
				population[idx].IsRealEvaluated = true
			}
		}

		sort.Slice(population, func(i, j int) bool {
			scoreI := population[i].Fitness
			scoreJ := population[j].Fitness
			if population[i].IsRealEvaluated {
				scoreI += 0.001
			}
			if population[j].IsRealEvaluated {
				scoreJ += 0.001
			}
			return scoreI > scoreJ
		})

		if population[0].Fitness > best.Fitness {
			best = population[0]
		}
		fitnessHistory = append(fitnessHistory, best.Fitness)

		if gen < ga.generations-1 {
			population = ga.nextGeneration(population)
		}
	}

	if !best.IsRealEvaluated {
		best.Fitness = ga.realEvaluateAndStore(wheel, currentData, best.Params)
	}

	improvement := 0.0
	if baseline > 0 {
		improvement = (best.Fitness - baseline) / baseline * 100.0
	}

	return &models.OptimizationResult{
		WaterwheelID: wheel.ID,
		BucketShapeParams: map[string]float64{
			"width":     best.Params.Width,
			"depth":     best.Params.Depth,
			"height":    best.Params.Height,
			"curvature": best.Params.Curvature,
		},
		BucketAngle:        best.Params.Angle,
		OptimizedLiftRate:  best.Fitness,
		OriginalLiftRate:   baseline,
		ImprovementPercent: improvement,
		GenerationCount:    ga.generations,
		FitnessHistory:     fitnessHistory,
	}
}

func (ga *GAOptimizer) paramsToVector(p models.BucketParams) [5]float64 {
	return [5]float64{
		p.Width / 2.0,
		p.Depth / 1.0,
		p.Height / 1.5,
		(p.Angle + 10.0) / 60.0,
		p.Curvature / 1.0,
	}
}

func (ga *GAOptimizer) surrogateEvaluate(ind *Individual) {
	if len(ga.surrogateSamples) < ga.surrogateK {
		ind.Fitness = 0
		ind.IsRealEvaluated = false
		return
	}

	vec := ga.paramsToVector(ind.Params)

	distances := make([]float64, len(ga.surrogateSamples))
	for i, s := range ga.surrogateSamples {
		d := 0.0
		for j := 0; j < 5; j++ {
			diff := vec[j] - s.params[j]
			d += diff * diff
		}
		distances[i] = math.Sqrt(d)
	}

	k := ga.surrogateK
	if k > len(ga.surrogateSamples) {
		k = len(ga.surrogateSamples)
	}

	weights := make([]float64, k)
	fitnesses := make([]float64, k)

	for i := 0; i < k; i++ {
		minIdx := i
		minDist := distances[i]
		for j := i + 1; j < len(distances); j++ {
			if distances[j] < minDist {
				minDist = distances[j]
				minIdx = j
			}
		}
		if minIdx != i {
			distances[i], distances[minIdx] = distances[minIdx], distances[i]
			ga.surrogateSamples[i], ga.surrogateSamples[minIdx] = ga.surrogateSamples[minIdx], ga.surrogateSamples[i]
		}
		weights[i] = 1.0 / (distances[i] + 0.0001)
		fitnesses[i] = ga.surrogateSamples[i].fitness
	}

	weightSum := 0.0
	fitSum := 0.0
	for i := 0; i < k; i++ {
		weightSum += weights[i]
		fitSum += weights[i] * fitnesses[i]
	}

	ind.Fitness = fitSum / weightSum
	ind.IsRealEvaluated = false
}

func (ga *GAOptimizer) surrogateEvaluateAll(pop []Individual) {
	for i := range pop {
		if !pop[i].IsRealEvaluated {
			ga.surrogateEvaluate(&pop[i])
		}
	}
}

func (ga *GAOptimizer) realEvaluateAndStore(wheel *models.Waterwheel, data *models.TelemetryData, params models.BucketParams) float64 {
	fitness := ga.evaluateFitness(wheel, data, params)
	ga.surrogateSamples = append(ga.surrogateSamples, surrogateSample{
		params:  ga.paramsToVector(params),
		fitness: fitness,
	})
	return fitness
}

func (ga *GAOptimizer) initializePopulation() []Individual {
	pop := make([]Individual, ga.populationSize)
	for i := range pop {
		pop[i] = Individual{
			Params: models.BucketParams{
				Width:     0.1 + ga.rand.Float64()*0.9,
				Depth:     0.05 + ga.rand.Float64()*0.5,
				Height:    0.1 + ga.rand.Float64()*0.8,
				Angle:     ga.rand.Float64()*45.0 - 5.0,
				Curvature: ga.rand.Float64()*0.8 + 0.1,
			},
		}
	}
	return pop
}

func (ga *GAOptimizer) evaluateFitness(wheel *models.Waterwheel, data *models.TelemetryData, params models.BucketParams) float64 {
	bucketVolume := params.Width * params.Depth * params.Height * params.Curvature * 0.7
	if bucketVolume <= 0 {
		return 0
	}

	radius := wheel.Diameter / 2.0
	omega := data.RotationSpeed * 2 * math.Pi / 60.0

	fillEfficiency := ga.calcFillEfficiency(wheel, data, params, omega)
	fillEfficiency = math.Max(0.15, math.Min(0.92, fillEfficiency))

	angleRad := params.Angle * math.Pi / 180.0
	angleFactor := 0.6 + 0.4*math.Sin(angleRad+math.Pi/6)
	fillEfficiency *= angleFactor
	fillEfficiency = math.Max(0.1, math.Min(0.92, fillEfficiency))

	activeBucketRatio := 0.38
	effectiveCount := float64(wheel.BucketCount) * activeBucketRatio * fillEfficiency
	volumePerRotation := effectiveCount * bucketVolume

	theoreticalLift := volumePerRotation * data.RotationSpeed * 60.0
	theoreticalLift = math.Min(theoreticalLift, wheel.MaxFlowRate*1.2)

	waterWeight := WaterDensity * Gravity * bucketVolume * fillEfficiency
	torquePerBucket := waterWeight * radius * math.Cos(angleRad)

	submergedCount := int(math.Max(2, float64(wheel.BucketCount)*0.25))
	totalTorque := float64(submergedCount)*torquePerBucket*0.7 +
		0.5*WaterDensity*data.FlowVelocity*data.FlowVelocity*params.Width*params.Height*radius*0.3

	dragCoeff := 0.5 + params.Curvature*0.5
	dragLoss := dragCoeff * data.FlowVelocity * data.FlowVelocity * params.Width * params.Height * 0.4

	centrifugalLoss := WaterDensity * bucketVolume * omega * omega * radius * 0.006

	totalTorque = totalTorque - dragLoss - centrifugalLoss
	totalTorque = math.Max(0, totalTorque)

	powerOutput := totalTorque * omega

	liftHeight := wheel.Diameter * 0.9
	actualLift := 0.0
	if liftHeight > 0 && Gravity > 0 {
		actualLift = powerOutput / (WaterDensity * Gravity * liftHeight) * 3600.0
	}

	fitness := actualLift*0.7 + theoreticalLift*0.3
	return math.Max(0, fitness)
}

func (ga *GAOptimizer) calcFillEfficiency(wheel *models.Waterwheel, data *models.TelemetryData,
	params models.BucketParams, omega float64) float64 {
	if omega <= 0 {
		return 0.92
	}

	submersionRatio := math.Min(1, data.WaterLevelDrop/wheel.Diameter)
	submergedAngle := 2 * math.Asin(math.Sqrt(submersionRatio))
	submergedAngle = math.Max(0.3, math.Min(math.Pi*0.8, submergedAngle))

	immersionTime := submergedAngle / omega

	fillTimeConst := 0.12 + params.Curvature*0.08
	fillEff := 1.0 - math.Exp(-immersionTime/fillTimeConst)

	return fillEff
}

func (ga *GAOptimizer) nextGeneration(pop []Individual) []Individual {
	next := make([]Individual, 0, ga.populationSize)

	elitism := 5
	for i := 0; i < elitism && i < len(pop); i++ {
		elite := pop[i]
		elite.IsRealEvaluated = pop[i].IsRealEvaluated
		next = append(next, elite)
	}

	for len(next) < ga.populationSize {
		parent1 := ga.tournamentSelect(pop)
		parent2 := ga.tournamentSelect(pop)

		var child Individual
		if ga.rand.Float64() < ga.crossoverRate {
			child = ga.crossover(parent1, parent2)
		} else {
			child = Individual{Params: parent1.Params}
		}

		if ga.rand.Float64() < ga.mutationRate {
			child = ga.mutate(child)
		}

		child.IsRealEvaluated = false
		next = append(next, child)
	}

	return next
}

func (ga *GAOptimizer) tournamentSelect(pop []Individual) Individual {
	tournamentSize := 5
	bestIdx := ga.rand.Intn(len(pop))
	for i := 1; i < tournamentSize; i++ {
		idx := ga.rand.Intn(len(pop))
		if pop[idx].Fitness > pop[bestIdx].Fitness {
			bestIdx = idx
		}
	}
	return pop[bestIdx]
}

func (ga *GAOptimizer) crossover(p1, p2 Individual) Individual {
	return Individual{
		Params: models.BucketParams{
			Width:     ga.blendCrossover(p1.Params.Width, p2.Params.Width),
			Depth:     ga.blendCrossover(p1.Params.Depth, p2.Params.Depth),
			Height:    ga.blendCrossover(p1.Params.Height, p2.Params.Height),
			Angle:     ga.blendCrossover(p1.Params.Angle, p2.Params.Angle),
			Curvature: ga.blendCrossover(p1.Params.Curvature, p2.Params.Curvature),
		},
	}
}

func (ga *GAOptimizer) blendCrossover(a, b float64) float64 {
	alpha := 0.5
	min := math.Min(a, b)
	max := math.Max(a, b)
	rangeVal := max - min
	return min - alpha*rangeVal + ga.rand.Float64()*(rangeVal*(1+2*alpha))
}

func (ga *GAOptimizer) mutate(ind Individual) Individual {
	mutateGene := func(val, min, max, sigma float64) float64 {
		if ga.rand.Float64() < 0.5 {
			val += ga.rand.NormFloat64() * sigma
		}
		return math.Max(min, math.Min(max, val))
	}

	ind.Params.Width = mutateGene(ind.Params.Width, 0.05, 2.0, 0.1)
	ind.Params.Depth = mutateGene(ind.Params.Depth, 0.02, 1.0, 0.05)
	ind.Params.Height = mutateGene(ind.Params.Height, 0.05, 1.5, 0.08)
	ind.Params.Angle = mutateGene(ind.Params.Angle, -10.0, 50.0, 3.0)
	ind.Params.Curvature = mutateGene(ind.Params.Curvature, 0.1, 1.0, 0.1)

	return ind
}

const (
	WaterDensity = 1000.0
	Gravity      = 9.81
)
