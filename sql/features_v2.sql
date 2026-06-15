-- ============================================================
-- Feature V2: 四大新功能扩展表
-- 1. 多筒车协同灌溉调度
-- 2. 季节性水位预测与高度调节
-- 3. 古今能效对比
-- 4. 公众虚拟建造筒车
-- ============================================================

-- ============================================================
-- 模块一: 灌溉田块与协同调度
-- ============================================================

CREATE TABLE IF NOT EXISTS irrigation_fields (
    id SERIAL PRIMARY KEY,
    name VARCHAR(100) NOT NULL,
    location VARCHAR(200) NOT NULL,
    area_hectare DOUBLE PRECISION NOT NULL DEFAULT 0,
    crop_type VARCHAR(50) NOT NULL DEFAULT '水稻',
    daily_water_req_m3 DOUBLE PRECISION NOT NULL DEFAULT 0,
    priority INTEGER NOT NULL DEFAULT 5,
    assigned_waterwheels INTEGER[] DEFAULT '{}',
    current_filled_m3 DOUBLE PRECISION DEFAULT 0,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS schedule_solutions (
    id SERIAL PRIMARY KEY,
    field_id INTEGER NOT NULL REFERENCES irrigation_fields(id),
    time TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    total_water_m3 DOUBLE PRECISION NOT NULL,
    total_duration_hours DOUBLE PRECISION NOT NULL,
    total_cost_yuan DOUBLE PRECISION NOT NULL,
    total_energy_kwh DOUBLE PRECISION NOT NULL,
    renewable_ratio DOUBLE PRECISION NOT NULL,
    waterwheel_plans JSONB NOT NULL DEFAULT '[]',
    pump_plan JSONB,
    status VARCHAR(20) NOT NULL DEFAULT 'pending',
    deadline_hours INTEGER NOT NULL DEFAULT 24
);

CREATE INDEX IF NOT EXISTS idx_schedules_field_id ON schedule_solutions (field_id, time DESC);

-- 预置田块数据 (对应10个筒车的分布区域)
INSERT INTO irrigation_fields (name, location, area_hectare, crop_type, daily_water_req_m3, priority, assigned_waterwheels) VALUES
('都江堰灌区A区',  '四川成都都江堰', 12.5, '水稻',   625.0, 1, ARRAY[1]),
('沣惠渠灌区北片', '陕西西安沣惠渠',  8.0, '小麦',   280.0, 3, ARRAY[2]),
('黑龙潭农耕地',   '云南丽江黑龙潭',  5.5, '玉米',   165.0, 4, ARRAY[3]),
('凤凰沱江灌区',   '湖南湘西凤凰',  15.0, '水稻',   870.0, 1, ARRAY[4]),
('灵渠北渠灌区',   '广西桂林灵渠',  10.0, '水稻',   520.0, 2, ARRAY[5]),
('三江汇灌区',     '广东佛山三水',   4.5, '蔬菜',   198.0, 3, ARRAY[6]),
('木兰陂南洋片',   '福建莆田木兰陂', 11.0, '水稻',   594.0, 2, ARRAY[7]),
('通济堰灌区',     '浙江丽水通济堰',  9.0, '茶叶',   315.0, 4, ARRAY[8]),
('渔梁坝灌区',     '安徽黄山渔梁坝', 13.0, '水稻',   702.0, 2, ARRAY[9]),
('富水河两岸地',   '江西吉安富田',   7.0, '油菜',   245.0, 3, ARRAY[10]),
('联合灌溉大区A',  '川湘联合灌区',   35.0, '水稻',  2100.0, 1, ARRAY[1,4,5,7,9])
ON CONFLICT DO NOTHING;

-- ============================================================
-- 模块二: 季节性水位预测与高度调节
-- ============================================================

CREATE TABLE IF NOT EXISTS water_level_forecasts (
    id SERIAL PRIMARY KEY,
    waterwheel_id INTEGER NOT NULL REFERENCES waterwheels(id),
    forecast_date TIMESTAMPTZ NOT NULL,
    horizon_days INTEGER NOT NULL DEFAULT 30,
    predicted_drop DOUBLE PRECISION NOT NULL,
    predicted_flow DOUBLE PRECISION NOT NULL,
    lower_bound DOUBLE PRECISION NOT NULL,
    upper_bound DOUBLE PRECISION NOT NULL,
    season VARCHAR(10) NOT NULL,
    confidence DOUBLE PRECISION NOT NULL,
    created_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_forecasts_wheel_time ON water_level_forecasts (waterwheel_id, forecast_date DESC);

CREATE TABLE IF NOT EXISTS height_adjustments (
    id SERIAL PRIMARY KEY,
    waterwheel_id INTEGER NOT NULL REFERENCES waterwheels(id),
    forecast_id INTEGER REFERENCES water_level_forecasts(id),
    current_height DOUBLE PRECISION NOT NULL,
    recommended_height DOUBLE PRECISION NOT NULL,
    adjustment_cm DOUBLE PRECISION NOT NULL,
    expected_lift_gain_percent DOUBLE PRECISION NOT NULL,
    expected_eff_gain_percent DOUBLE PRECISION NOT NULL,
    submergence_before DOUBLE PRECISION NOT NULL,
    submergence_after DOUBLE PRECISION NOT NULL,
    reason TEXT NOT NULL,
    status VARCHAR(20) NOT NULL DEFAULT 'suggested',
    implemented_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_adjustments_wheel ON height_adjustments (waterwheel_id, created_at DESC);

CREATE TABLE IF NOT EXISTS historical_hydrology (
    date TIMESTAMPTZ NOT NULL,
    waterwheel_id INTEGER NOT NULL REFERENCES waterwheels(id),
    avg_drop DOUBLE PRECISION NOT NULL,
    avg_flow DOUBLE PRECISION NOT NULL,
    rainfall_mm DOUBLE PRECISION DEFAULT 0,
    month INTEGER NOT NULL,
    PRIMARY KEY (waterwheel_id, date)
);

SELECT create_hypertable('historical_hydrology', 'date', if_not_exists => TRUE);

-- 注入3年月度水文历史数据 (10台筒车 × 36个月)
DO $$
DECLARE
    w_id INTEGER;
    m INTEGER;
    y INTEGER;
    base_drop DOUBLE PRECISION;
    base_flow DOUBLE PRECISION;
    season_fact DOUBLE PRECISION;
    rain_fall DOUBLE PRECISION;
BEGIN
    FOR w_id IN 1..10 LOOP
        base_drop := 1.5 + (w_id * 0.08);
        base_flow := 1.0 + (w_id * 0.08);
        FOR y IN 2023..2025 LOOP
            FOR m IN 1..12 LOOP
                CASE m
                    WHEN 1,2,12 THEN season_fact := 0.55; rain_fall := 10 + random()*20;    -- 冬枯
                    WHEN 3,4,5   THEN season_fact := 0.85; rain_fall := 50 + random()*60;    -- 春汛
                    WHEN 6,7,8   THEN season_fact := 1.45; rain_fall := 180 + random()*150; -- 夏丰
                    WHEN 9,10,11 THEN season_fact := 1.10; rain_fall := 60 + random()*70;    -- 秋平
                END CASE;
                INSERT INTO historical_hydrology (date, waterwheel_id, avg_drop, avg_flow, rainfall_mm, month)
                VALUES (
                    make_date(y, m, 15)::TIMESTAMPTZ,
                    w_id,
                    ROUND((base_drop * season_fact * (0.9 + random()*0.2))::numeric, 3),
                    ROUND((base_flow * season_fact * (0.9 + random()*0.2))::numeric, 3),
                    ROUND(rain_fall::numeric, 1),
                    m
                ) ON CONFLICT DO NOTHING;
            END LOOP;
        END LOOP;
    END LOOP;
END $$;

-- ============================================================
-- 模块三: 古今能效对比
-- ============================================================

CREATE TABLE IF NOT EXISTS efficiency_comparisons (
    id SERIAL PRIMARY KEY,
    waterwheel_id INTEGER NOT NULL REFERENCES waterwheels(id),
    time TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    period_days INTEGER NOT NULL DEFAULT 365,
    waterwheel_metrics JSONB NOT NULL,
    modern_pump_metrics JSONB NOT NULL,
    ancient_advantage JSONB NOT NULL,
    scenario VARCHAR(50) NOT NULL DEFAULT 'standard'
);

CREATE INDEX IF NOT EXISTS idx_comparisons_wheel ON efficiency_comparisons (waterwheel_id, time DESC);

-- ============================================================
-- 模块四: 公众虚拟建造筒车
-- ============================================================

CREATE TABLE IF NOT EXISTS virtual_builds (
    id SERIAL PRIMARY KEY,
    user_id VARCHAR(64) NOT NULL DEFAULT 'anonymous',
    build_name VARCHAR(100) NOT NULL,
    diameter_m DOUBLE PRECISION NOT NULL,
    bucket_count INTEGER NOT NULL,
    bucket_capacity_m3 DOUBLE PRECISION NOT NULL,
    spoke_count INTEGER NOT NULL,
    material VARCHAR(20) NOT NULL,
    wheel_angle_deg DOUBLE PRECISION NOT NULL DEFAULT 0,
    install_height_m DOUBLE PRECISION NOT NULL DEFAULT 0,
    parts_used JSONB NOT NULL DEFAULT '[]',
    predicted_lift_m3h DOUBLE PRECISION NOT NULL,
    predicted_efficiency DOUBLE PRECISION NOT NULL,
    blueprint_svg TEXT,
    is_public BOOLEAN NOT NULL DEFAULT true,
    likes INTEGER NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_builds_user ON virtual_builds (user_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_builds_public ON virtual_builds (is_public, likes DESC);

-- 预置经典筒车模板 (虚拟建造的参考样本)
INSERT INTO virtual_builds (user_id, build_name, diameter_m, bucket_count, bucket_capacity_m3, spoke_count, material,
    predicted_lift_m3h, predicted_efficiency, is_public, likes) VALUES
('preset', '都江堰经典24斗筒车', 8.5, 24, 0.08, 12, '楠木', 120.0, 0.42, true, 256),
('preset', '凤凰沱江巨型28斗',  9.1, 28, 0.10, 16, '杉木', 142.0, 0.38, true, 189),
('preset', '丽江小型18斗轻便型', 6.8, 18, 0.05, 10, '柏木',  82.0, 0.45, true, 145),
('preset', '竹制民俗12斗筒车',   5.5, 12, 0.04,  8, '竹制',  45.0, 0.36, true, 312),
('preset', '铸铁工业型32斗',    10.2, 32, 0.12, 16, '铸铁', 175.0, 0.50, true,  98)
ON CONFLICT DO NOTHING;

-- ============================================================
-- 压缩策略 (保留策略与原表保持一致风格)
-- ============================================================

ALTER TABLE historical_hydrology SET (
  timescaledb.compress,
  timescaledb.compress_orderby = 'date DESC',
  timescaledb.compress_segmentby = 'waterwheel_id'
);

SELECT add_compression_policy('historical_hydrology', INTERVAL '90 days', if_not_exists => TRUE);
SELECT add_retention_policy('historical_hydrology', INTERVAL '10 years', if_not_exists => TRUE);
