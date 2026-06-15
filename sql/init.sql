CREATE EXTENSION IF NOT EXISTS timescaledb;

CREATE TABLE IF NOT EXISTS waterwheels (
    id SERIAL PRIMARY KEY,
    name VARCHAR(100) NOT NULL,
    location VARCHAR(200) NOT NULL,
    diameter DOUBLE PRECISION NOT NULL,
    bucket_count INTEGER NOT NULL,
    bucket_capacity DOUBLE PRECISION NOT NULL,
    max_flow_rate DOUBLE PRECISION NOT NULL,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS telemetry_data (
    time TIMESTAMPTZ NOT NULL,
    waterwheel_id INTEGER NOT NULL REFERENCES waterwheels(id),
    rotation_speed DOUBLE PRECISION NOT NULL,
    water_lift DOUBLE PRECISION NOT NULL,
    water_level_drop DOUBLE PRECISION NOT NULL,
    flow_velocity DOUBLE PRECISION NOT NULL,
    mechanical_efficiency DOUBLE PRECISION,
    hydraulic_efficiency DOUBLE PRECISION,
    torque DOUBLE PRECISION,
    power_output DOUBLE PRECISION
);

SELECT create_hypertable('telemetry_data', 'time', if_not_exists => TRUE);

CREATE INDEX IF NOT EXISTS idx_telemetry_waterwheel_id ON telemetry_data (waterwheel_id, time DESC);

CREATE TABLE IF NOT EXISTS alerts (
    id SERIAL PRIMARY KEY,
    waterwheel_id INTEGER NOT NULL REFERENCES waterwheels(id),
    alert_type VARCHAR(50) NOT NULL,
    message TEXT NOT NULL,
    severity VARCHAR(20) NOT NULL DEFAULT 'warning',
    efficiency_value DOUBLE PRECISION,
    historical_avg DOUBLE PRECISION,
    time TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    acknowledged BOOLEAN DEFAULT FALSE
);

CREATE INDEX IF NOT EXISTS idx_alerts_waterwheel_id ON alerts (waterwheel_id, time DESC);

CREATE TABLE IF NOT EXISTS optimization_results (
    id SERIAL PRIMARY KEY,
    waterwheel_id INTEGER NOT NULL REFERENCES waterwheels(id),
    bucket_shape_params JSONB NOT NULL,
    bucket_angle DOUBLE PRECISION NOT NULL,
    optimized_lift_rate DOUBLE PRECISION NOT NULL,
    original_lift_rate DOUBLE PRECISION NOT NULL,
    improvement_percent DOUBLE PRECISION NOT NULL,
    generation_count INTEGER NOT NULL,
    fitness_history JSONB,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_optimization_waterwheel_id ON optimization_results (waterwheel_id, created_at DESC);

CREATE TABLE IF NOT EXISTS efficiency_history (
    time TIMESTAMPTZ NOT NULL,
    waterwheel_id INTEGER NOT NULL REFERENCES waterwheels(id),
    avg_mechanical_efficiency DOUBLE PRECISION NOT NULL,
    avg_hydraulic_efficiency DOUBLE PRECISION NOT NULL,
    period_hours INTEGER NOT NULL DEFAULT 24
);

SELECT create_hypertable('efficiency_history', 'time', if_not_exists => TRUE);

ALTER TABLE telemetry_data SET (
  timescaledb.compress,
  timescaledb.compress_orderby = 'time DESC',
  timescaledb.compress_segmentby = 'waterwheel_id'
);

ALTER TABLE efficiency_history SET (
  timescaledb.compress,
  timescaledb.compress_orderby = 'time DESC',
  timescaledb.compress_segmentby = 'waterwheel_id'
);

SELECT add_compression_policy('telemetry_data', INTERVAL '7 days', if_not_exists => TRUE);
SELECT add_compression_policy('efficiency_history', INTERVAL '30 days', if_not_exists => TRUE);

SELECT add_retention_policy('telemetry_data', INTERVAL '365 days', if_not_exists => TRUE);
SELECT add_retention_policy('efficiency_history', INTERVAL '730 days', if_not_exists => TRUE);

SELECT add_continuous_aggregate_policy('efficiency_history_cagg',
  start_offset => INTERVAL '3 days',
  end_offset   => INTERVAL '1 hour',
  schedule_interval => INTERVAL '1 hour',
  if_not_exists => TRUE
) WHERE EXISTS (
  SELECT 1 FROM timescaledb_information.continuous_aggregates
  WHERE view_name = 'efficiency_history_cagg'
);

INSERT INTO waterwheels (name, location, diameter, bucket_count, bucket_capacity, max_flow_rate) VALUES
('筒车一号', '四川成都都江堰', 8.5, 24, 0.08, 120.0),
('筒车二号', '陕西西安沣惠渠', 7.2, 20, 0.06, 95.0),
('筒车三号', '云南丽江黑龙潭', 6.8, 18, 0.05, 85.0),
('筒车四号', '湖南湘西凤凰', 9.1, 28, 0.10, 140.0),
('筒车五号', '广西桂林灵渠', 7.8, 22, 0.07, 110.0),
('筒车六号', '广东佛山三水', 6.5, 16, 0.05, 75.0),
('筒车七号', '福建莆田木兰陂', 8.0, 24, 0.08, 115.0),
('筒车八号', '浙江丽水通济堰', 7.5, 20, 0.06, 100.0),
('筒车九号', '安徽黄山渔梁坝', 8.2, 26, 0.09, 130.0),
('筒车十号', '江西吉安富田', 7.0, 18, 0.06, 90.0)
ON CONFLICT DO NOTHING;
