# 古代筒车自动提水效率监测与水力优化系统

对10座古代筒车遗址进行复原监测，每5分钟通过4G DTU上报遥测数据，后端基于水力模型实时计算效率，
遗传算法优化水斗形状，效率低于阈值时触发 MQTT 告警。前端 Canvas 动画实时展示筒车运转。

---

## 一、系统架构

```
┌──────────────────────────────────────────────────────────────────────────────────┐
│                               前端 (Nginx + Gzip)                                │
│  ┌───────────────┐  ┌──────────────────┐  ┌────────────────┐                     │
│  │ norria_view.js│  │efficiency_panel.js│  │   app.js 协调   │  Canvas 动画 + 面板 │
│  └───────┬───────┘  └────────┬─────────┘  └────────┬───────┘                     │
└──────────┼───────────────────┼─────────────────────┼─────────────────────────────┘
           │                   │                     │
           │  REST / WebSocket │                     │
           ▼                   ▼                     ▼
┌──────────────────────────────────────────────────────────────────────────────────┐
│                             Go 后端 (Gin + Channel 管道)                          │
│                                                                                  │
│  ┌──────────────────┐   ┌──────────────────┐   ┌──────────────────┐              │
│  │  dtu_receiver    │──▶│ hydraulic_model  │──▶│   alarm_mqtt     │───▶  MQTT    │
│  │  HTTP 采集       │   │  水力效率计算     │   │  告警分级/防抖   │     Broker   │
│  │  参数校验        │   │  动态充水模型     │   │  MQTT 推送       │              │
│  └──────────────────┘   └────────┬─────────┘   └──────────────────┘              │
│                                   │                                               │
│                                   ├───────────────────┐                           │
│                                   ▼                   ▼                           │
│                         ┌──────────────────┐  ┌──────────────────┐                │
│                         │   TimescaleDB    │  │ shape_optimizer  │                │
│                         │  时序数据存储     │  │  遗传算法+代理模型 │                │
│                         │  自动压缩/保留策略 │  │  KNN Surrogate    │                │
│                         └──────────────────┘  └──────────────────┘                │
│                                                                                  │
│  ┌──────────────────────────────┐  ┌──────────────────────────────────┐          │
│  │  /metrics  Prometheus 指标   │  │  /debug/pprof/  Go pprof 性能    │          │
│  └──────────────────────────────┘  └──────────────────────────────────┘          │
└──────────────────────────────────────────────────────────────────────────────────┘
           ▲
           │ HTTP POST
┌──────────┴──────────┐
│  10 × 筒车 DTU 模拟器 │  realistic / flood / drought / failure / storm
└─────────────────────┘
```

### 管道数据流

```
DTU → [RawCh 1024] → hydraulic_model(2 workers) → [EnrichedCh] → DB
                                          ↘ [AlertCh] → alarm_mqtt → MQTT
                                          ↘ [OptimizeReqCh] → shape_optimizer
```

---

## 二、目录结构

```
.
├── backend/                    # Go 后端
│   ├── cmd/server/main.go      # 入口，管道编排
│   ├── internal/
│   │   ├── dtu_receiver/       # DTU 数据采集模块
│   │   ├── hydraulic_model/    # 水力效率计算
│   │   ├── shape_optimizer/    # 遗传算法 + KNN 代理
│   │   ├── alarm_mqtt/         # 告警推送 (防抖/分级)
│   │   ├── pipeline/           # 管道 (channels 定义)
│   │   ├── metrics/            # Prometheus 指标
│   │   ├── config/             # 参数外置
│   │   ├── models/             # 数据模型
│   │   ├── database/           # DB 操作 (双写兼容新旧 schema)
│   │   └── handlers/           # HTTP 接口
│   └── go.mod
├── frontend/                   # 前端 (原生 JS + Canvas)
│   ├── index.html
│   └── js/
│       ├── config.js           # 前端参数外置
│       ├── norria_view.js      # 筒车 Canvas 视图
│       ├── efficiency_panel.js # 效率曲线 + 分析面板
│       └── app.js              # 主协调器
├── simulator/                  # Python 筒车模拟器
│   ├── simulator.py
│   └── requirements.txt
├── sql/
│   └── init.sql                # 建表 + Hypertable + 压缩策略
├── deploy/
│   ├── nginx.conf              # 前端 Nginx + Gzip
│   ├── mosquitto.conf          # MQTT Broker 配置
│   └── prometheus.yml          # Prometheus 抓取配置
├── docker-compose.yml          # 一键编排
├── Dockerfile.backend          # Go 多阶段构建
├── Dockerfile.frontend
├── Dockerfile.simulator
└── .env.example
```

---

## 三、快速启动

### 1. 复制环境变量

```bash
cp .env.example .env
```

### 2. 启动核心服务 (DB + MQTT + 后端 + 前端)

```bash
docker compose up -d --build
```

### 3. 启动模拟器 (可选)

```bash
docker compose --profile simulator up -d simulator
```

### 4. 启动监控 (可选，Prometheus)

```bash
docker compose --profile monitoring up -d prometheus
```

### 访问地址

| 服务 | 地址 |
|------|------|
| 前端 | http://localhost:8081 |
| 后端 API | http://localhost:8080/api |
| Prometheus 指标 | http://localhost:8080/metrics |
| pprof 性能分析 | http://localhost:8080/debug/pprof/ |
| Prometheus UI | http://localhost:9090 (monitoring profile) |
| MQTT Broker | localhost:1883 |

---

## 四、核心功能

### 4.1 数据采集 (dtu_receiver)
- HTTP POST `/api/telemetry` 接收 DTU 上报
- 参数边界校验 (转速/流量/落差范围)
- 管道非阻塞写入，队列满丢弃并计数
- 健康检查 `/api/health` 返回队列深度

### 4.2 水力效率模型 (hydraulic_model)
- **动态充水指数模型**：`fillEff = 0.15 + 0.77 * (1 - exp(-t_immerse / 0.15))`
- 转矩平衡：水流冲击转矩 vs 提水阻力转矩
- 输出：机械效率 / 水力效率 / 综合效率 / 功率
- 双 worker 并发消费 RawCh

### 4.3 遗传算法优化 (shape_optimizer)
- 5 维参数空间：bucket_angle / depth_ratio / width_ratio / active_angle / backsweep_angle
- **KNN 代理模型**：K=7 距离倒数加权，仅对前 30% 个体做真实物理评估
- 锦标赛选择 + BLX-α 交叉 + 高斯变异 + 精英保留
- 通过 OptimizeReqCh 管道异步执行

### 4.4 告警推送 (alarm_mqtt)
- 三级告警：critical / major / warning
- **60 分钟冷却防抖**，避免告警风暴
- MQTT 3 秒超时发布
- 同步写入 DB (新/旧 schema 双写兼容)

### 4.5 前端
- **NorriaView**：Canvas 筒车侧视图 + 水流粒子 + 动态水斗 + Web Worker 物理计算
- **EfficiencyPanel**：效率曲线 + 水利分析 + 告警列表 + 优化历史
- 所有可配置参数外置 `config.js`

---

## 五、可观测性

### 5.1 Prometheus 指标

| 指标名 | 类型 | 说明 |
|--------|------|------|
| `http_requests_total` | Counter | HTTP 请求总数 (按 method/path/status 分) |
| `http_request_duration_seconds` | Histogram | HTTP 请求时延 |
| `http_requests_in_flight` | Gauge | 进行中请求数 |
| `telemetry_received_total` | Counter | 遥测接收总数 |
| `alerts_total` | Counter | 告警数 (按 severity 分) |
| `optimization_runs_total` | Counter | 优化执行次数 |
| `optimization_duration_seconds` | Histogram | 优化耗时 |
| `db_query_duration_seconds` | Histogram | DB 查询耗时 |
| `hydraulic_model_duration_seconds` | Histogram | 水力模型计算耗时 |
| `channel_depth` | Gauge | 各管道队列深度 |
| `waterwheel_efficiency` | Gauge | 每台筒车实时效率 |

### 5.2 Go pprof

- CPU Profile：`go tool pprof http://localhost:8080/debug/pprof/profile?seconds=30`
- Heap：`go tool pprof http://localhost:8080/debug/pprof/heap`
- Goroutine：`http://localhost:8080/debug/pprof/goroutine?debug=1`
- Allocs：`http://localhost:8080/debug/pprof/allocs`
- Mutex：`http://localhost:8080/debug/pprof/mutex`

---

## 六、TimescaleDB 数据生命周期

| 策略 | telemetry_data | efficiency_history |
|------|---------------|-------------------|
| 压缩阈值 | 7 天 | 30 天 |
| 保留期限 | 365 天 | 730 天 |
| 分段键 | waterwheel_id | waterwheel_id |
| 排序键 | time DESC | time DESC |

压缩比可达 10x - 20x，大幅降低存储成本。

---

## 七、模拟器模式

通过环境变量 `SIMULATION_MODE` 切换：

| 模式 | 说明 |
|------|------|
| `realistic` | 真实场景，昼夜节律波动 (默认) |
| `flood` | 汛期，水流+60%，转速+25%，落差+40% |
| `drought` | 枯水期，水流-55%，转速-40% |
| `failure` | 逐渐磨损模式，30 天内转速衰减 45%，随机卡滞 |
| `storm` | 暴风雨，周期性能量脉冲，最高 2x 流量 |

其他环境变量：
- `WATER_FLOW_BASE`：水流基准倍率
- `ROTATION_SPEED_BASE`：转速基准倍率
- `REPORT_INTERVAL_SECONDS`：上报间隔秒数
- `WHEEL_COUNT`：筒车数量 (1-10)
- `SEED_HISTORICAL=1` + `SEED_HOURS=24`：启动时注入历史数据

---

## 八、前端 Gzip 压缩

Nginx 已启用 gzip，压缩等级 6，覆盖类型：
- text/html, text/css, text/plain, text/xml
- application/javascript, application/json, application/xml
- image/svg+xml, image/png, image/jpeg
- woff/woff2/ttf/eot 字体

静态资源缓存 30 天 (Cache-Control: public, immutable)。

---

## 九、API 接口

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | /api/health | 健康检查 |
| GET | /api/waterwheels | 筒车列表 |
| GET | /api/waterwheels/:id | 单台筒车详情 |
| POST | /api/telemetry | 上报遥测数据 |
| GET | /api/waterwheels/:id/telemetry | 最新遥测 |
| GET | /api/waterwheels/:id/telemetry/range | 时间范围遥测 |
| GET | /api/waterwheels/:id/efficiency | 效率分析 |
| GET | /api/waterwheels/:id/alerts | 告警列表 |
| POST | /api/waterwheels/:id/optimize | 触发优化 (30s 超时) |
| GET | /api/waterwheels/:id/optimizations | 优化历史 |
| GET | /metrics | Prometheus 指标 |
| GET | /debug/pprof/ | pprof 索引 |

---

## 十、配置参数

### Go 后端参数 (internal/config/params.go)
- `HydraulicParams`：水密度、重力、摩擦系数、充水时间常数等
- `OptimizerParams`：种群大小、代数、变异率、代理K值等
- `AlarmParams`：冷却时间、分级阈值等
- `ReceiverParams`：参数边界校验范围

### 前端参数 (frontend/js/config.js)
- `view`：筒车几何尺寸、颜色、转速倍率
- `particles`：粒子数、速度、大小
- `colors`：主题色板
- `panel`：图表尺寸、刷新间隔、超时

---

## 十一、开发调试

```bash
# 仅启 DB 和 MQTT，本地跑 Go
docker compose up timescaledb mqtt-broker -d
cd backend && go run ./cmd/server

# 前端直接用浏览器打开 frontend/index.html
# 或用任何静态服务器
cd frontend && python -m http.server 8081
```

---

## 十二、技术栈

| 层 | 技术 |
|----|------|
| 后端 | Go 1.21 + Gin + pgx |
| 数据库 | PostgreSQL 16 + TimescaleDB 2.14 |
| MQTT | Eclipse Mosquitto 2.0 |
| 前端 | 原生 JS + Canvas 2D + Web Worker |
| 监控 | Prometheus + Go pprof |
| 部署 | Docker 多阶段构建 + docker compose |
| 模拟器 | Python 3.11 |
