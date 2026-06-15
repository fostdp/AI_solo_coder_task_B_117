package database

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"waterwheel-monitor/internal/config"
	"waterwheel-monitor/internal/models"
)

type Database struct {
	pool *pgxpool.Pool
}

func New(cfg *config.Config) (*Database, error) {
	poolConfig, err := pgxpool.ParseConfig(cfg.DSN())
	if err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}

	pool, err := pgxpool.NewWithConfig(context.Background(), poolConfig)
	if err != nil {
		return nil, fmt.Errorf("create pool: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := pool.Ping(ctx); err != nil {
		return nil, fmt.Errorf("ping database: %w", err)
	}

	return &Database{pool: pool}, nil
}

func (db *Database) Close() {
	db.pool.Close()
}

func (db *Database) GetWaterwheels(ctx context.Context) ([]models.Waterwheel, error) {
	rows, err := db.pool.Query(ctx, `
		SELECT id, name, location, diameter, bucket_count, bucket_capacity, max_flow_rate, created_at, updated_at
		FROM waterwheels ORDER BY id
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var wheels []models.Waterwheel
	for rows.Next() {
		var w models.Waterwheel
		if err := rows.Scan(&w.ID, &w.Name, &w.Location, &w.Diameter, &w.BucketCount,
			&w.BucketCapacity, &w.MaxFlowRate, &w.CreatedAt, &w.UpdatedAt); err != nil {
			return nil, err
		}
		wheels = append(wheels, w)
	}
	return wheels, rows.Err()
}

func (db *Database) GetWaterwheelByID(ctx context.Context, id int64) (*models.Waterwheel, error) {
	var w models.Waterwheel
	err := db.pool.QueryRow(ctx, `
		SELECT id, name, location, diameter, bucket_count, bucket_capacity, max_flow_rate, created_at, updated_at
		FROM waterwheels WHERE id = $1
	`, id).Scan(&w.ID, &w.Name, &w.Location, &w.Diameter, &w.BucketCount,
		&w.BucketCapacity, &w.MaxFlowRate, &w.CreatedAt, &w.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return &w, nil
}

func (db *Database) InsertTelemetry(ctx context.Context, data *models.TelemetryData) error {
	_, err := db.pool.Exec(ctx, `
		INSERT INTO telemetry_data (time, waterwheel_id, rotation_speed, water_lift, water_level_drop,
			flow_velocity, mechanical_efficiency, hydraulic_efficiency, torque, power_output)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
	`, data.Time, data.WaterwheelID, data.RotationSpeed, data.WaterLift, data.WaterLevelDrop,
		data.FlowVelocity, data.MechanicalEfficiency, data.HydraulicEfficiency, data.Torque, data.PowerOutput)
	return err
}

func (db *Database) GetLatestTelemetry(ctx context.Context, waterwheelID int64, limit int) ([]models.TelemetryData, error) {
	rows, err := db.pool.Query(ctx, `
		SELECT time, waterwheel_id, rotation_speed, water_lift, water_level_drop,
			flow_velocity, mechanical_efficiency, hydraulic_efficiency, torque, power_output
		FROM telemetry_data WHERE waterwheel_id = $1 ORDER BY time DESC LIMIT $2
	`, waterwheelID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var data []models.TelemetryData
	for rows.Next() {
		var d models.TelemetryData
		if err := rows.Scan(&d.Time, &d.WaterwheelID, &d.RotationSpeed, &d.WaterLift, &d.WaterLevelDrop,
			&d.FlowVelocity, &d.MechanicalEfficiency, &d.HydraulicEfficiency, &d.Torque, &d.PowerOutput); err != nil {
			return nil, err
		}
		data = append(data, d)
	}
	return data, rows.Err()
}

func (db *Database) GetTelemetryRange(ctx context.Context, waterwheelID int64, start, end time.Time) ([]models.TelemetryData, error) {
	rows, err := db.pool.Query(ctx, `
		SELECT time, waterwheel_id, rotation_speed, water_lift, water_level_drop,
			flow_velocity, mechanical_efficiency, hydraulic_efficiency, torque, power_output
		FROM telemetry_data WHERE waterwheel_id = $1 AND time BETWEEN $2 AND $3 ORDER BY time ASC
	`, waterwheelID, start, end)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var data []models.TelemetryData
	for rows.Next() {
		var d models.TelemetryData
		if err := rows.Scan(&d.Time, &d.WaterwheelID, &d.RotationSpeed, &d.WaterLift, &d.WaterLevelDrop,
			&d.FlowVelocity, &d.MechanicalEfficiency, &d.HydraulicEfficiency, &d.Torque, &d.PowerOutput); err != nil {
			return nil, err
		}
		data = append(data, d)
	}
	return data, rows.Err()
}

func (db *Database) GetHistoricalAvgEfficiency(ctx context.Context, waterwheelID int64, hours int) (float64, error) {
	since := time.Now().Add(-time.Duration(hours) * time.Hour)
	var avg float64
	err := db.pool.QueryRow(ctx, `
		SELECT COALESCE(AVG(COALESCE(mechanical_efficiency, 0) * COALESCE(hydraulic_efficiency, 0)), 0)
		FROM telemetry_data WHERE waterwheel_id = $1 AND time > $2
	`, waterwheelID, since).Scan(&avg)
	return avg, err
}

func (db *Database) InsertAlert(ctx context.Context, alert *models.Alert) error {
	if alert.Type == "" {
		alert.Type = models.AlertTypeLowEfficiency
	}
	if alert.Severity == "" {
		alert.Severity = models.SeverityWarning
	}

	err := db.pool.QueryRow(ctx, `
		INSERT INTO alerts (waterwheel_id, waterwheel_name, time, type, severity, message,
			current_efficiency, historical_efficiency, threshold, rotation_speed, water_lift,
			water_level_drop, flow_velocity, acknowledged)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)
		ON CONFLICT DO NOTHING
		RETURNING id
	`, alert.WaterwheelID, alert.WaterwheelName, alert.Time, alert.Type,
		alert.Severity, alert.Message, alert.CurrentEfficiency, alert.HistoricalEfficiency,
		alert.Threshold, alert.RotationSpeed, alert.WaterLift, alert.WaterLevelDrop,
		alert.FlowVelocity, alert.Acknowledged).Scan(&alert.ID)
	if err == nil {
		return nil
	}

	return db.insertAlertLegacy(ctx, alert)
}

func (db *Database) insertAlertLegacy(ctx context.Context, alert *models.Alert) error {
	oldType := alert.Type
	if oldType == "" {
		oldType = string(models.AlertTypeLowEfficiency)
	}
	oldSev := string(alert.Severity)
	if oldSev == "" {
		oldSev = string(models.SeverityWarning)
	}
	err := db.pool.QueryRow(ctx, `
		INSERT INTO alerts (waterwheel_id, alert_type, message, severity,
			efficiency_value, historical_avg, time, acknowledged)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		ON CONFLICT DO NOTHING
		RETURNING id
	`, alert.WaterwheelID, oldType, alert.Message, oldSev,
		alert.CurrentEfficiency, alert.HistoricalEfficiency, alert.Time, alert.Acknowledged).Scan(&alert.ID)
	if err != nil {
		alert.ID = 0
	}
	return nil
}

func (db *Database) GetAlerts(ctx context.Context, waterwheelID int64, limit int) ([]models.Alert, error) {
	rows, err := db.pool.Query(ctx, `
		SELECT id, waterwheel_id, COALESCE(waterwheel_name,''), COALESCE(time, NOW()),
			COALESCE(type,'low_efficiency'), COALESCE(severity,'warning'),
			COALESCE(message,''),
			COALESCE(current_efficiency, efficiency_value, 0),
			COALESCE(historical_efficiency, historical_avg, 0),
			COALESCE(threshold, 0),
			COALESCE(rotation_speed, 0),
			COALESCE(water_lift, 0),
			COALESCE(water_level_drop, 0),
			COALESCE(flow_velocity, 0),
			COALESCE(acknowledged, false)
		FROM alerts WHERE waterwheel_id = $1 ORDER BY time DESC LIMIT $2
	`, waterwheelID, limit)
	if err != nil {
		return db.getAlertsLegacy(ctx, waterwheelID, limit)
	}
	defer rows.Close()

	var alerts []models.Alert
	for rows.Next() {
		var a models.Alert
		if err := rows.Scan(&a.ID, &a.WaterwheelID, &a.WaterwheelName, &a.Time,
			&a.Type, &a.Severity, &a.Message,
			&a.CurrentEfficiency, &a.HistoricalEfficiency, &a.Threshold,
			&a.RotationSpeed, &a.WaterLift, &a.WaterLevelDrop, &a.FlowVelocity,
			&a.Acknowledged); err != nil {
			return nil, err
		}
		alerts = append(alerts, a)
	}
	return alerts, rows.Err()
}

func (db *Database) getAlertsLegacy(ctx context.Context, waterwheelID int64, limit int) ([]models.Alert, error) {
	rows, err := db.pool.Query(ctx, `
		SELECT id, waterwheel_id, alert_type, message, severity,
			efficiency_value, historical_avg, time, acknowledged
		FROM alerts WHERE waterwheel_id = $1 ORDER BY time DESC LIMIT $2
	`, waterwheelID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var alerts []models.Alert
	for rows.Next() {
		var a models.Alert
		var atype, severity string
		var eff, hist float64
		var ack bool
		if err := rows.Scan(&a.ID, &a.WaterwheelID, &atype, &a.Message, &severity,
			&eff, &hist, &a.Time, &ack); err != nil {
			return nil, err
		}
		a.Type = atype
		a.Severity = models.AlertSeverity(severity)
		a.CurrentEfficiency = eff
		a.HistoricalEfficiency = hist
		a.Acknowledged = ack
		alerts = append(alerts, a)
	}
	return alerts, rows.Err()
}

func (db *Database) InsertOptimizationResult(ctx context.Context, result *models.OptimizationResult) error {
	if result.Time.IsZero() {
		result.Time = time.Now()
	}

	err := db.pool.QueryRow(ctx, `
		INSERT INTO optimization_results (waterwheel_id, time, optimal_bucket_angle,
			optimal_depth_ratio, optimal_width_ratio, predicted_lift,
			predicted_improvement_percent, fitness, generations)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		ON CONFLICT DO NOTHING
		RETURNING id
	`, result.WaterwheelID, result.Time, result.OptimalBucketAngle,
		result.OptimalDepthRatio, result.OptimalWidthRatio, result.PredictedLift,
		result.PredictedImprovement, result.Fitness, result.Generations).Scan(&result.ID)
	if err == nil {
		return nil
	}

	return db.insertOptimizationLegacy(ctx, result)
}

func (db *Database) insertOptimizationLegacy(ctx context.Context, result *models.OptimizationResult) error {
	params := map[string]float64{
		"bucket_angle":   result.OptimalBucketAngle,
		"depth_ratio":    result.OptimalDepthRatio,
		"width_ratio":    result.OptimalWidthRatio,
	}
	improved := result.PredictedLift
	if result.PredictedImprovement != 0 {
		improved = result.PredictedLift / (1 + result.PredictedImprovement/100.0)
	}
	original := improved
	err := db.pool.QueryRow(ctx, `
		INSERT INTO optimization_results (waterwheel_id, bucket_shape_params, bucket_angle,
			optimized_lift_rate, original_lift_rate, improvement_percent, generation_count, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		ON CONFLICT DO NOTHING
		RETURNING id
	`, result.WaterwheelID, params, result.OptimalBucketAngle,
		result.PredictedLift, original, result.PredictedImprovement,
		result.Generations, result.Time).Scan(&result.ID)
	if err != nil {
		result.ID = 0
	}
	return nil
}

func (db *Database) GetOptimizationResults(ctx context.Context, waterwheelID int64, limit int) ([]models.OptimizationResult, error) {
	rows, err := db.pool.Query(ctx, `
		SELECT id, waterwheel_id, time, optimal_bucket_angle, optimal_depth_ratio,
			optimal_width_ratio, predicted_lift, predicted_improvement_percent,
			fitness, generations
		FROM optimization_results WHERE waterwheel_id = $1 ORDER BY time DESC LIMIT $2
	`, waterwheelID, limit)
	if err != nil {
		return db.getOptimizationLegacy(ctx, waterwheelID, limit)
	}
	defer rows.Close()

	var results []models.OptimizationResult
	for rows.Next() {
		var r models.OptimizationResult
		if err := rows.Scan(&r.ID, &r.WaterwheelID, &r.Time, &r.OptimalBucketAngle,
			&r.OptimalDepthRatio, &r.OptimalWidthRatio, &r.PredictedLift,
			&r.PredictedImprovement, &r.Fitness, &r.Generations); err != nil {
			return nil, err
		}
		results = append(results, r)
	}
	return results, rows.Err()
}

func (db *Database) getOptimizationLegacy(ctx context.Context, waterwheelID int64, limit int) ([]models.OptimizationResult, error) {
	rows, err := db.pool.Query(ctx, `
		SELECT id, waterwheel_id, bucket_angle, optimized_lift_rate,
			improvement_percent, generation_count, COALESCE(created_at, NOW())
		FROM optimization_results WHERE waterwheel_id = $1 ORDER BY created_at DESC LIMIT $2
	`, waterwheelID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []models.OptimizationResult
	for rows.Next() {
		var r models.OptimizationResult
		var ba, oLr, ip float64
		var gc int
		if err := rows.Scan(&r.ID, &r.WaterwheelID, &ba, &oLr, &ip, &gc, &r.Time); err != nil {
			return nil, err
		}
		r.OptimalBucketAngle = ba
		r.PredictedLift = oLr
		r.PredictedImprovement = ip
		r.Generations = gc
		results = append(results, r)
	}
	return results, rows.Err()
}

// ============================================================
// Feature V2: 灌溉田块与协同调度
// ============================================================

func (db *Database) ListIrrigationFields(ctx context.Context) ([]models.IrrigationField, error) {
	rows, err := db.pool.Query(ctx, `
		SELECT id, name, location, area_hectare, crop_type,
		daily_water_req_m3, priority, COALESCE(assigned_waterwheels, '{}'),
		COALESCE(current_filled_m3, 0), created_at
		FROM irrigation_fields ORDER BY priority ASC, id ASC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var fields []models.IrrigationField
	for rows.Next() {
		var f models.IrrigationField
		if err := rows.Scan(&f.ID, &f.Name, &f.Location, &f.AreaHectare,
			&f.CropType, &f.DailyWaterReqM3, &f.Priority, &f.AssignedWaterwheel,
			&f.CurrentFilledM3, &f.CreatedAt); err != nil {
			return nil, err
		}
		fields = append(fields, f)
	}
	return fields, rows.Err()
}

func (db *Database) GetIrrigationField(ctx context.Context, id int64) (*models.IrrigationField, error) {
	row := db.pool.QueryRow(ctx, `
		SELECT id, name, location, area_hectare, crop_type,
		daily_water_req_m3, priority, COALESCE(assigned_waterwheels, '{}'),
		COALESCE(current_filled_m3, 0), created_at
		FROM irrigation_fields WHERE id = $1
	`, id)
	var f models.IrrigationField
	if err := row.Scan(&f.ID, &f.Name, &f.Location, &f.AreaHectare,
		&f.CropType, &f.DailyWaterReqM3, &f.Priority, &f.AssignedWaterwheel,
		&f.CurrentFilledM3, &f.CreatedAt); err != nil {
		return nil, err
	}
	return &f, nil
}

func (db *Database) SaveScheduleSolution(ctx context.Context, sol *models.ScheduleSolution) (int64, error) {
	wpJSON, _ := json.Marshal(sol.WaterwheelPlans)
	var ppJSON []byte
	if sol.PumpPlan != nil {
		ppJSON, _ = json.Marshal(sol.PumpPlan)
	}
	var deadline int64
	err := db.pool.QueryRow(ctx, `
		INSERT INTO schedule_solutions (field_id, time, total_water_m3, total_duration_hours,
			total_cost_yuan, total_energy_kwh, renewable_ratio, waterwheel_plans, pump_plan, status, deadline_hours)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8::jsonb,$9::jsonb,$10,$11)
		RETURNING id
	`, sol.FieldID, sol.Time, sol.TotalWaterM3, sol.TotalDurationHours,
		sol.TotalCostYuan, sol.TotalEnergyKWh, sol.RenewableRatio,
		wpJSON, ppJSON, sol.Status, 24).Scan(&deadline)
	return deadline, err
}

func (db *Database) ListScheduleSolutions(ctx context.Context, fieldID int64, limit int) ([]models.ScheduleSolution, error) {
	q := `SELECT id, field_id, time, total_water_m3, total_duration_hours,
		total_cost_yuan, total_energy_kwh, renewable_ratio,
		waterwheel_plans, pump_plan, status FROM schedule_solutions `
	var args []interface{}
	if fieldID > 0 {
		q += `WHERE field_id = $1 `
		args = append(args, fieldID)
		args = append(args, limit)
		q += `ORDER BY time DESC LIMIT $2`
	} else {
		args = append(args, limit)
		q += `ORDER BY time DESC LIMIT $1`
	}
	rows, err := db.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var sols []models.ScheduleSolution
	for rows.Next() {
		var s models.ScheduleSolution
		var wpB, ppB []byte
		if err := rows.Scan(&s.ID, &s.FieldID, &s.Time, &s.TotalWaterM3, &s.TotalDurationHours,
			&s.TotalCostYuan, &s.TotalEnergyKWh, &s.RenewableRatio,
			&wpB, &ppB, &s.Status); err != nil {
			return nil, err
		}
		_ = json.Unmarshal(wpB, &s.WaterwheelPlans)
		if ppB != nil {
			var pp models.PumpPlan
			_ = json.Unmarshal(ppB, &pp)
			s.PumpPlan = &pp
		}
		sols = append(sols, s)
	}
	return sols, rows.Err()
}

// ============================================================
// Feature V2: 季节性水位预测与高度调节
// ============================================================

func (db *Database) ListHistoricalHydrology(ctx context.Context, waterwheelID int64, daysBack int) ([]models.HistoricalHydrology, error) {
	rows, err := db.pool.Query(ctx, `
		SELECT date, waterwheel_id, avg_drop, avg_flow, rainfall_mm, month
		FROM historical_hydrology WHERE waterwheel_id = $1 AND date >= NOW() - $2::interval
		ORDER BY date ASC
	`, waterwheelID, fmt.Sprintf("%d days", daysBack))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []models.HistoricalHydrology
	for rows.Next() {
		var h models.HistoricalHydrology
		if err := rows.Scan(&h.Date, &h.WaterwheelID, &h.AvgDrop, &h.AvgFlow, &h.RainfallMm, &h.Month); err != nil {
			return nil, err
		}
		list = append(list, h)
	}
	return list, rows.Err()
}

func (db *Database) SaveWaterLevelForecast(ctx context.Context, f *models.WaterLevelForecast) (int64, error) {
	var id int64
	err := db.pool.QueryRow(ctx, `
		INSERT INTO water_level_forecasts (waterwheel_id, forecast_date, horizon_days,
			predicted_drop, predicted_flow, lower_bound, upper_bound, season, confidence, created_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10) RETURNING id
	`, f.WaterwheelID, f.ForecastDate, f.HorizonDays,
		f.PredictedDrop, f.PredictedFlow, f.LowerBound, f.UpperBound,
		f.Season, f.Confidence, f.CreatedAt).Scan(&id)
	return id, err
}

func (db *Database) GetForecastByID(ctx context.Context, id int64) (*models.WaterLevelForecast, error) {
	row := db.pool.QueryRow(ctx, `
		SELECT id, waterwheel_id, forecast_date, horizon_days, predicted_drop,
			predicted_flow, lower_bound, upper_bound, season, confidence, created_at
		FROM water_level_forecasts WHERE id = $1
	`, id)
	var f models.WaterLevelForecast
	if err := row.Scan(&f.ID, &f.WaterwheelID, &f.ForecastDate, &f.HorizonDays,
		&f.PredictedDrop, &f.PredictedFlow, &f.LowerBound, &f.UpperBound,
		&f.Season, &f.Confidence, &f.CreatedAt); err != nil {
		return nil, err
	}
	return &f, nil
}

func (db *Database) ListForecasts(ctx context.Context, waterwheelID int64, limit int) ([]models.WaterLevelForecast, error) {
	rows, err := db.pool.Query(ctx, `
		SELECT id, waterwheel_id, forecast_date, horizon_days, predicted_drop,
			predicted_flow, lower_bound, upper_bound, season, confidence, created_at
		FROM water_level_forecasts WHERE waterwheel_id = $1 ORDER BY forecast_date DESC LIMIT $2
	`, waterwheelID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []models.WaterLevelForecast
	for rows.Next() {
		var f models.WaterLevelForecast
		if err := rows.Scan(&f.ID, &f.WaterwheelID, &f.ForecastDate, &f.HorizonDays,
			&f.PredictedDrop, &f.PredictedFlow, &f.LowerBound, &f.UpperBound,
			&f.Season, &f.Confidence, &f.CreatedAt); err != nil {
			return nil, err
		}
		list = append(list, f)
	}
	return list, rows.Err()
}

func (db *Database) SaveHeightAdjustment(ctx context.Context, a *models.HeightAdjustment) (int64, error) {
	var id int64
	fid := a.ForecastID
	if fid == 0 {
		fid = 0
	}
	err := db.pool.QueryRow(ctx, `
		INSERT INTO height_adjustments (waterwheel_id, forecast_id, current_height, recommended_height,
			adjustment_cm, expected_lift_gain_percent, expected_eff_gain_percent,
			submergence_before, submergence_after, reason, status, created_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12) RETURNING id
	`, a.WaterwheelID, fid, a.CurrentHeight, a.RecommendedHeight,
		a.AdjustmentCm, a.ExpectedLiftGain, a.ExpectedEffGain,
		a.SubmergenceBefore, a.SubmergenceAfter, a.Reason, a.Status, a.CreatedAt).Scan(&id)
	return id, err
}

func (db *Database) ListHeightAdjustments(ctx context.Context, waterwheelID int64, limit int) ([]models.HeightAdjustment, error) {
	rows, err := db.pool.Query(ctx, `
		SELECT id, waterwheel_id, COALESCE(forecast_id,0), current_height, recommended_height,
			adjustment_cm, expected_lift_gain_percent, expected_eff_gain_percent,
			submergence_before, submergence_after, reason, status, implemented_at, created_at
		FROM height_adjustments WHERE waterwheel_id = $1 ORDER BY created_at DESC LIMIT $2
	`, waterwheelID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []models.HeightAdjustment
	for rows.Next() {
		var a models.HeightAdjustment
		if err := rows.Scan(&a.ID, &a.WaterwheelID, &a.ForecastID, &a.CurrentHeight, &a.RecommendedHeight,
			&a.AdjustmentCm, &a.ExpectedLiftGain, &a.ExpectedEffGain,
			&a.SubmergenceBefore, &a.SubmergenceAfter, &a.Reason, &a.Status, &a.ImplementedAt, &a.CreatedAt); err != nil {
			return nil, err
		}
		list = append(list, a)
	}
	return list, rows.Err()
}

func (db *Database) MarkAdjustmentImplemented(ctx context.Context, adjID int64) error {
	_, err := db.pool.Exec(ctx, `
		UPDATE height_adjustments SET status='implemented', implemented_at = NOW() WHERE id = $1
	`, adjID)
	return err
}

// ============================================================
// Feature V2: 古今能效对比
// ============================================================

func (db *Database) SaveEfficiencyComparison(ctx context.Context, c *models.EfficiencyComparison) (int64, error) {
	wmB, _ := json.Marshal(c.WaterwheelMetrics)
	mmB, _ := json.Marshal(c.ModernPumpMetrics)
	advB, _ := json.Marshal(c.AncientAdvantage)
	var id int64
	err := db.pool.QueryRow(ctx, `
		INSERT INTO efficiency_comparisons (waterwheel_id, time, period_days, waterwheel_metrics, modern_pump_metrics, ancient_advantage, scenario)
		VALUES ($1,$2,$3,$4::jsonb,$5::jsonb,$6::jsonb,$7) RETURNING id
	`, c.WaterwheelID, c.Time, c.PeriodDays, wmB, mmB, advB, c.Scenario).Scan(&id)
	return id, err
}

func (db *Database) ListEfficiencyComparisons(ctx context.Context, waterwheelID int64, limit int) ([]models.EfficiencyComparison, error) {
	q := `SELECT id, waterwheel_id, time, period_days, waterwheel_metrics, modern_pump_metrics, ancient_advantage, scenario FROM efficiency_comparisons `
	var args []interface{}
	if waterwheelID > 0 {
		q += `WHERE waterwheel_id = $1 `
		args = append(args, waterwheelID)
		args = append(args, limit)
		q += `ORDER BY time DESC LIMIT $2`
	} else {
		args = append(args, limit)
		q += `ORDER BY time DESC LIMIT $1`
	}
	rows, err := db.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []models.EfficiencyComparison
	for rows.Next() {
		var c models.EfficiencyComparison
		var wB, mB, aB []byte
		if err := rows.Scan(&c.ID, &c.WaterwheelID, &c.Time, &c.PeriodDays, &wB, &mB, &aB, &c.Scenario); err != nil {
			return nil, err
		}
		_ = json.Unmarshal(wB, &c.WaterwheelMetrics)
		_ = json.Unmarshal(mB, &c.ModernPumpMetrics)
		_ = json.Unmarshal(aB, &c.AncientAdvantage)
		list = append(list, c)
	}
	return list, rows.Err()
}

// ============================================================
// Feature V2: 公众虚拟建造筒车
// ============================================================

func (db *Database) SaveVirtualBuild(ctx context.Context, b *models.VirtualBuild) (int64, error) {
	pB, _ := json.Marshal(b.PartsUsed)
	var id int64
	err := db.pool.QueryRow(ctx, `
		INSERT INTO virtual_builds (user_id, build_name, diameter_m, bucket_count, bucket_capacity_m3,
			spoke_count, material, wheel_angle_deg, install_height_m, parts_used,
			predicted_lift_m3h, predicted_efficiency, blueprint_svg, is_public, likes, created_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10::jsonb,$11,$12,$13,$14,$15,$16) RETURNING id
	`, b.UserID, b.BuildName, b.Diameter, b.BucketCount, b.BucketCapacity,
		b.SpokeCount, b.Material, b.WheelAngle, b.InstallHeight, pB,
		b.PredictedLift, b.PredictedEff, b.Blueprint, b.IsPublic, b.Likes, b.CreatedAt).Scan(&id)
	return id, err
}

func (db *Database) ListVirtualBuilds(ctx context.Context, userID string, onlyPublic bool, limit int) ([]models.VirtualBuild, error) {
	q := `SELECT id, user_id, build_name, diameter_m, bucket_count, bucket_capacity_m3,
		spoke_count, material, wheel_angle_deg, install_height_m, parts_used,
		predicted_lift_m3h, predicted_efficiency, likes, is_public, created_at
		FROM virtual_builds WHERE `
	var args []interface{}
	conds := []string{}
	if userID != "" && !onlyPublic {
		args = append(args, userID)
		conds = append(conds, fmt.Sprintf("(is_public = true OR user_id = $%d", len(args)))
	} else if onlyPublic {
		conds = append(conds, "is_public = true")
	}
	if len(conds) > 0 {
		q += strings.Join(conds, " AND ") + " "
	}
	args = append(args, limit)
	q += fmt.Sprintf("ORDER BY likes DESC, created_at DESC LIMIT $%d", len(args))

	rows, err := db.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []models.VirtualBuild
	for rows.Next() {
		var b models.VirtualBuild
		var pB []byte
		if err := rows.Scan(&b.ID, &b.UserID, &b.BuildName, &b.Diameter, &b.BucketCount, &b.BucketCapacity,
			&b.SpokeCount, &b.Material, &b.WheelAngle, &b.InstallHeight, &pB,
			&b.PredictedLift, &b.PredictedEff, &b.Likes, &b.IsPublic, &b.CreatedAt); err != nil {
			return nil, err
		}
		_ = json.Unmarshal(pB, &b.PartsUsed)
		list = append(list, b)
	}
	return list, rows.Err()
}

func (db *Database) IncrementBuildLikes(ctx context.Context, buildID int64) error {
	_, err := db.pool.Exec(ctx, `UPDATE virtual_builds SET likes = likes + 1 WHERE id = $1`, buildID)
	return err
}
