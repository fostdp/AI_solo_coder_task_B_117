package forecasting

import (
	"context"
	"fmt"
	"math"
	"time"

	"waterwheel-monitor/internal/config"
	"waterwheel-monitor/internal/database"
	"waterwheel-monitor/internal/models"
)

type WaterLevelForecaster struct {
	db     *database.Database
	params *config.ForecastingParams
}

func NewWaterLevelForecaster(db *database.Database, params *config.ForecastingParams) *WaterLevelForecaster {
	return &WaterLevelForecaster{db: db, params: params}
}

var seasonNames = map[int]string{
	1: "冬枯", 2: "冬枯", 12: "冬枯",
	3: "春汛", 4: "春汛", 5: "春汛",
	6: "夏丰", 7: "夏丰", 8: "夏丰",
	9: "秋平", 10: "秋平", 11: "秋平",
}

func monthToSeason(m int) string {
	if s, ok := seasonNames[m]; ok {
		return s
	}
	return "过渡"
}

type seasonalBaseline struct {
	avgDrop float64
	avgFlow float64
	count   int
}

func (f *WaterLevelForecaster) GenerateForecast(ctx context.Context, wheelID int64, horizonDays int) (*models.WaterLevelForecast, error) {
	if horizonDays <= 0 {
		horizonDays = f.params.DefaultHorizonDays
	}
	wheel, err := f.db.GetWaterwheelByID(ctx, wheelID)
	if err != nil {
		return nil, fmt.Errorf("筒车不存在: %w", err)
	}
	_ = wheel

	history, err := f.db.ListHistoricalHydrology(ctx, wheelID, 3*365)
	if err != nil {
		return nil, err
	}
	if len(history) == 0 {
		return nil, fmt.Errorf("无历史水文数据")
	}

	monthly := make(map[int]*seasonalBaseline)
	for _, h := range history {
		key := h.Month
		if _, ok := monthly[key]; !ok {
			monthly[key] = &seasonalBaseline{}
		}
		b := monthly[key]
		b.avgDrop += h.AvgDrop
		b.avgFlow += h.AvgFlow
		b.count++
	}
	for _, b := range monthly {
		if b.count > 0 {
			b.avgDrop /= float64(b.count)
			b.avgFlow /= float64(b.count)
		}
	}

	now := time.Now()
	forecastDate := now.AddDate(0, 0, horizonDays)
	futureMonth := int(forecastDate.Month())
	curMonth := int(now.Month())

	var predDrop, predFlow float64
	var seasonFactor float64

	if b, ok := monthly[futureMonth]; ok && b.count > 0 {
		seasonPredD := b.avgDrop * f.params.SeasonWeight
		seasonPredF := b.avgFlow * f.params.SeasonWeight

		arPredD, arPredF := f.autoregressive(history, horizonDays)
		trendD, trendF := f.trendComponent(history, horizonDays)

		predDrop = seasonPredD + arPredD*f.params.ARWeight + trendD*f.params.TrendWeight
		predFlow = seasonPredF + arPredF*f.params.ARWeight + trendF*f.params.TrendWeight

		if cb, ok := monthly[curMonth]; ok && cb.count > 0 {
			if b.avgDrop > 0 {
				seasonFactor = cb.avgDrop / b.avgDrop
			}
		}
	} else {
		predDrop, predFlow = f.autoregressive(history, horizonDays)
		predDrop *= f.params.SeasonWeight
		predFlow *= f.params.SeasonWeight
	}

	if seasonFactor <= 0 {
		seasonFactor = 1.0
	}
	_ = seasonFactor

	volatility := f.calcVolatility(history, futureMonth)
	lowerDrop := predDrop * (1 - volatility)
	upperDrop := predDrop * (1 + volatility)
	lowerFlow := predFlow * (1 - volatility)
	upperFlow := predFlow * (1 + volatility)

	confidence := 1.0 - volatility
	if confidence < f.params.MinConfidence {
		confidence = f.params.MinConfidence
	}

	forecast := &models.WaterLevelForecast{
		WaterwheelID:  wheelID,
		ForecastDate:  forecastDate,
		HorizonDays:   horizonDays,
		PredictedDrop: round3(predDrop),
		PredictedFlow: round3(predFlow),
		LowerBound:    round3(math.Min(lowerDrop, lowerFlow)),
		UpperBound:    round3(math.Max(upperDrop, upperFlow)),
		Season:        monthToSeason(futureMonth),
		Confidence:    round3(confidence),
		CreatedAt:     time.Now(),
	}

	id, err := f.db.SaveWaterLevelForecast(ctx, forecast)
	if err == nil {
		forecast.ID = id
	}
	return forecast, nil
}

func (f *WaterLevelForecaster) autoregressive(history []models.HistoricalHydrology, horizonDays int) (drop, flow float64) {
	n := len(history)
	if n == 0 {
		return 0, 0
	}
	recentLimit := 30
	if n < recentLimit {
		recentLimit = n
	}
	recent := history[n-recentLimit:]

	var recDrop, recFlow float64
	for _, r := range recent {
		recDrop += r.AvgDrop
		recFlow += r.AvgFlow
	}
	recDrop /= float64(len(recent))
	recFlow /= float64(len(recent))

	longLimit := 90
	if n < longLimit {
		longLimit = n
	}
	long := history[n-longLimit:]
	var longDrop, longFlow float64
	for _, r := range long {
		longDrop += r.AvgDrop
		longFlow += r.AvgFlow
	}
	longDrop /= float64(len(long))
	longFlow /= float64(len(long))

	alpha := 0.7
	horizonFactor := 1.0 / (1.0 + float64(horizonDays)/180.0)
	drop = (alpha*recDrop + (1-alpha)*longDrop) * horizonFactor
	flow = (alpha*recFlow + (1-alpha)*longFlow) * horizonFactor
	return
}

func (f *WaterLevelForecaster) trendComponent(history []models.HistoricalHydrology, horizonDays int) (drop, flow float64) {
	n := len(history)
	if n < 30 {
		return 0, 0
	}
	half := n / 2
	oldHalf := history[:half]
	newHalf := history[half:]

	var oldDrop, oldFlow, newDrop, newFlow float64
	for _, r := range oldHalf {
		oldDrop += r.AvgDrop
		oldFlow += r.AvgFlow
	}
	for _, r := range newHalf {
		newDrop += r.AvgDrop
		newFlow += r.AvgFlow
	}
	oldDrop /= float64(len(oldHalf))
	oldFlow /= float64(len(oldHalf))
	newDrop /= float64(len(newHalf))
	newFlow /= float64(len(newHalf))

	trendPerDayD := (newDrop - oldDrop) / float64(half*30)
	trendPerDayF := (newFlow - oldFlow) / float64(half*30)

	drop = newDrop + trendPerDayD*float64(horizonDays)
	flow = newFlow + trendPerDayF*float64(horizonDays)
	return
}

func (f *WaterLevelForecaster) calcVolatility(history []models.HistoricalHydrology, month int) float64 {
	var valsD, valsF []float64
	for _, h := range history {
		if h.Month == month {
			valsD = append(valsD, h.AvgDrop)
			valsF = append(valsF, h.AvgFlow)
		}
	}
	cv := func(v []float64) float64 {
		if len(v) < 2 {
			return 0.15
		}
		var mean, sd float64
		for _, x := range v {
			mean += x
		}
		mean /= float64(len(v))
		for _, x := range v {
			sd += (x - mean) * (x - mean)
		}
		sd = math.Sqrt(sd / float64(len(v)-1))
		if mean == 0 {
			return 0.15
		}
		return sd / mean
	}
	return math.Max(cv(valsD), cv(valsF)) * 0.7
}

func (f *WaterLevelForecaster) ProposeHeightAdjustment(ctx context.Context, wheelID int64, forecastID int64, currentHeight float64) (*models.HeightAdjustment, error) {
	wheel, err := f.db.GetWaterwheelByID(ctx, wheelID)
	if err != nil {
		return nil, fmt.Errorf("筒车不存在: %w", err)
	}

	var forecast *models.WaterLevelForecast
	if forecastID > 0 {
		forecast, _ = f.db.GetForecastByID(ctx, forecastID)
	}
	if forecast == nil {
		forecast, err = f.GenerateForecast(ctx, wheelID, f.params.DefaultHorizonDays)
		if err != nil {
			return nil, err
		}
	}

	if currentHeight <= 0 {
		currentHeight = wheel.Diameter * 0.45
	}

	drop := forecast.PredictedDrop
	radius := wheel.Diameter / 2.0
	subBefore := 0.0
	if radius+currentHeight < drop {
		subBefore = 1.0
	} else {
		subBefore = (drop - currentHeight) / radius
	}
	if subBefore < 0 {
		subBefore = 0
	}
	if subBefore > 1 {
		subBefore = 1
	}

	target := f.params.TargetSubmergence
	stepM := f.params.HeightStepCm / 100.0

	var recHeight float64
	var reason string

	if subBefore < target-0.05 {
		deltaCm := (target - subBefore) * radius * 100
		adjCm := math.Min(deltaCm, f.params.MaxAdjustmentCm)
		adjCm = math.Round(adjCm/stepM/100) * f.params.HeightStepCm
		recHeight = currentHeight - adjCm/100
		if recHeight < 0 {
			recHeight = 0
		}
		reason = fmt.Sprintf("预测%s季水位下降(%.2fm)，当前浸没度%.0f%%过低，建议下移%.0fcm至推荐浸没度%.0f%%",
			forecast.Season, drop, subBefore*100, adjCm, target*100)
	} else if subBefore > target+0.08 {
		deltaCm := (subBefore - target) * radius * 100
		adjCm := math.Min(deltaCm, f.params.MaxAdjustmentCm)
		adjCm = math.Round(adjCm/stepM/100) * f.params.HeightStepCm
		recHeight = currentHeight + adjCm/100
		reason = fmt.Sprintf("预测%s季水位高(%.2fm)，当前浸没度%.0f%%过高，轴承摩擦加大，建议上移%.0fcm",
			forecast.Season, drop, subBefore*100, adjCm)
	} else if drop < wheel.Diameter*f.params.WarningDropRatio {
		adjCm := math.Min(30.0, f.params.MaxAdjustmentCm)
		adjCm = math.Round(adjCm/stepM/100) * f.params.HeightStepCm
		recHeight = currentHeight - adjCm/100
		if recHeight < 0 {
			recHeight = 0
		}
		reason = fmt.Sprintf("预测水位落差仅%.2fm(筒车直径的%.0f%%)，接近临界值，建议下移%.0fcm以防空转",
			drop, drop/wheel.Diameter*100, adjCm)
	} else {
		recHeight = currentHeight
		reason = fmt.Sprintf("当前浸没度%.0f%%在合理区间(%.0f%%±)，预测水位平稳，暂不建议调节",
			subBefore*100, target*100)
	}

	subAfter := 0.0
	if radius+recHeight < drop {
		subAfter = 1.0
	} else {
		subAfter = (drop - recHeight) / radius
	}
	if subAfter < 0 {
		subAfter = 0
	}
	if subAfter > 1 {
		subAfter = 1
	}

	effGain := math.Max(0, (subAfter/subBefore - 1) * 100)
	if subBefore == 0 {
		effGain = 25
	}
	liftGain := effGain * 0.85

	adj := &models.HeightAdjustment{
		WaterwheelID:      wheelID,
		ForecastID:        forecast.ID,
		CurrentHeight:     round3(currentHeight),
		RecommendedHeight: round3(recHeight),
		AdjustmentCm:      round3((recHeight - currentHeight) * 100),
		ExpectedLiftGain:  round2(liftGain),
		ExpectedEffGain:   round2(effGain),
		SubmergenceBefore: round2(subBefore * 100),
		SubmergenceAfter:  round2(subAfter * 100),
		Reason:            reason,
		Status:            "suggested",
		CreatedAt:         time.Now(),
	}

	id, err := f.db.SaveHeightAdjustment(ctx, adj)
	if err == nil {
		adj.ID = id
	}
	return adj, nil
}

func (f *WaterLevelForecaster) ListForecasts(ctx context.Context, wheelID int64, limit int) ([]models.WaterLevelForecast, error) {
	if limit <= 0 {
		limit = 20
	}
	return f.db.ListForecasts(ctx, wheelID, limit)
}

func (f *WaterLevelForecaster) ListAdjustments(ctx context.Context, wheelID int64, limit int) ([]models.HeightAdjustment, error) {
	if limit <= 0 {
		limit = 20
	}
	return f.db.ListHeightAdjustments(ctx, wheelID, limit)
}

func (f *WaterLevelForecaster) MarkAdjustmentImplemented(ctx context.Context, adjID int64) error {
	return f.db.MarkAdjustmentImplemented(ctx, adjID)
}

func round2(v float64) float64 { return math.Round(v*100) / 100 }
func round3(v float64) float64 { return math.Round(v*1000) / 1000 }
