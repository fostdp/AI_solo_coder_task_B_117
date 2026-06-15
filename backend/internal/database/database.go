package database

import (
	"context"
	"fmt"
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
