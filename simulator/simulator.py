#!/usr/bin/env python3
"""
古代筒车遥测数据模拟器 - 多模式版本
支持5种仿真模式：realistic / flood / drought / failure / storm
"""

import json
import math
import os
import random
import sys
import time
from datetime import datetime
from urllib import request, error

API_BASE = os.environ.get("API_BASE_URL", "http://localhost:8080/api")
INTERVAL_SECONDS = int(os.environ.get("REPORT_INTERVAL_SECONDS", "300"))
SIMULATION_MODE = os.environ.get("SIMULATION_MODE", "realistic").lower()
WHEEL_COUNT = int(os.environ.get("WHEEL_COUNT", "10"))
WATER_FLOW_BASE = float(os.environ.get("WATER_FLOW_BASE", "1.5"))
ROTATION_SPEED_BASE = float(os.environ.get("ROTATION_SPEED_BASE", "8.0"))

WATERWHEEL_CONFIGS = [
    {"id": 1,  "name": "筒车一号", "base_rpm": 2.8, "base_drop": 2.1, "base_flow": 1.8, "capacity": 120.0},
    {"id": 2,  "name": "筒车二号", "base_rpm": 3.2, "base_drop": 1.8, "base_flow": 1.5, "capacity": 95.0},
    {"id": 3,  "name": "筒车三号", "base_rpm": 2.5, "base_drop": 1.6, "base_flow": 1.4, "capacity": 85.0},
    {"id": 4,  "name": "筒车四号", "base_rpm": 3.5, "base_drop": 2.5, "base_flow": 2.1, "capacity": 140.0},
    {"id": 5,  "name": "筒车五号", "base_rpm": 3.0, "base_drop": 2.0, "base_flow": 1.7, "capacity": 110.0},
    {"id": 6,  "name": "筒车六号", "base_rpm": 2.2, "base_drop": 1.5, "base_flow": 1.2, "capacity": 75.0},
    {"id": 7,  "name": "筒车七号", "base_rpm": 3.1, "base_drop": 2.2, "base_flow": 1.9, "capacity": 115.0},
    {"id": 8,  "name": "筒车八号", "base_rpm": 2.9, "base_drop": 1.9, "base_flow": 1.6, "capacity": 100.0},
    {"id": 9,  "name": "筒车九号", "base_rpm": 3.3, "base_drop": 2.3, "base_flow": 2.0, "capacity": 130.0},
    {"id": 10, "name": "筒车十号", "base_rpm": 2.6, "base_drop": 1.7, "base_flow": 1.3, "capacity": 90.0},
]


def apply_mode_factors(base_flow, base_rpm, base_drop, tick):
    """根据仿真模式调整水流、转速、落差基准值"""
    flow_factor = 1.0
    rpm_factor = 1.0
    drop_factor = 1.0

    day_phase = (tick * INTERVAL_SECONDS) % 86400 / 86400.0
    daily_curve = 0.85 + 0.3 * math.sin(2 * math.pi * (day_phase - 0.25))

    if SIMULATION_MODE == "realistic":
        flow_factor = daily_curve
        rpm_factor = daily_curve
        drop_factor = daily_curve

    elif SIMULATION_MODE == "flood":
        flood_level = 1.6 + 0.3 * math.sin(tick * 0.05)
        flow_factor = flood_level
        rpm_factor = 1.25 + 0.2 * math.sin(tick * 0.05)
        drop_factor = 1.4 + 0.2 * math.sin(tick * 0.05 + 0.3)

    elif SIMULATION_MODE == "drought":
        drought_level = 0.45 + 0.1 * math.sin(tick * 0.02)
        flow_factor = drought_level
        rpm_factor = 0.6 + 0.1 * math.sin(tick * 0.02)
        drop_factor = 0.6 + 0.08 * math.sin(tick * 0.02 + 0.5)

    elif SIMULATION_MODE == "failure":
        wear = min(1.0, tick * INTERVAL_SECONDS / (3600 * 24 * 30))
        rpm_factor = 1.0 - 0.45 * wear
        flow_factor = daily_curve * (1.0 - 0.1 * wear)
        drop_factor = daily_curve
        if random.random() < 0.02 * wear:
            rpm_factor *= 0.6

    elif SIMULATION_MODE == "storm":
        storm_cycle = math.sin(tick * 0.1)
        if storm_cycle > 0.7:
            flow_factor = 2.0 + random.gauss(0, 0.3)
            rpm_factor = 1.5 + random.gauss(0, 0.2)
            drop_factor = 1.8 + random.gauss(0, 0.2)
        elif storm_cycle > 0:
            flow_factor = 1.2 + 0.8 * storm_cycle
            rpm_factor = 1.1 + 0.4 * storm_cycle
            drop_factor = 1.1 + 0.7 * storm_cycle
        else:
            flow_factor = 0.8 + 0.2 * storm_cycle
            rpm_factor = 0.9 + 0.1 * storm_cycle
            drop_factor = 0.85 + 0.15 * storm_cycle

    flow_noise = random.gauss(0, 0.03)
    rpm_noise = random.gauss(0, 0.04)
    drop_noise = random.gauss(0, 0.02)

    return (
        base_flow * flow_factor * (1 + flow_noise),
        base_rpm * rpm_factor * (1 + rpm_noise),
        base_drop * drop_factor * (1 + drop_noise),
    )


def generate_telemetry(wheel_cfg, tick):
    base_flow = wheel_cfg["base_flow"] * WATER_FLOW_BASE / 1.5
    base_rpm = wheel_cfg["base_rpm"] * ROTATION_SPEED_BASE / 2.8
    base_drop = wheel_cfg["base_drop"]

    flow_velocity, rotation_speed, water_level_drop = apply_mode_factors(
        base_flow, base_rpm, base_drop, tick
    )

    flow_velocity = max(0.2, flow_velocity)
    rotation_speed = max(0.3, rotation_speed)
    water_level_drop = max(0.3, water_level_drop)

    bucket_count = 24 if wheel_cfg["id"] in [1, 4, 7, 9] else (20 if wheel_cfg["id"] in [2, 5, 8] else 18)
    bucket_capacity = 0.08 if wheel_cfg["id"] in [1, 4, 7, 9] else (0.06 if wheel_cfg["id"] in [2, 5, 8, 10] else 0.05)

    fill_ratio = min(1.0, flow_velocity / 2.5)
    fill_eff = 0.2 + 0.7 * (1 - math.exp(-fill_ratio / 0.3))

    volume_per_rotation = bucket_count * fill_eff * bucket_capacity
    water_lift = min(volume_per_rotation * rotation_speed * 60.0, wheel_cfg["capacity"])

    if random.random() < 0.005:
        rotation_speed *= random.uniform(0.5, 0.75)
        water_lift *= random.uniform(0.4, 0.65)

    return {
        "waterwheel_id": wheel_cfg["id"],
        "rotation_speed": round(rotation_speed, 3),
        "water_lift": round(water_lift, 2),
        "water_level_drop": round(water_level_drop, 3),
        "flow_velocity": round(flow_velocity, 3),
        "time": datetime.utcnow().isoformat() + "Z"
    }


def send_telemetry(data):
    payload = json.dumps(data).encode("utf-8")
    req = request.Request(
        f"{API_BASE}/telemetry",
        data=payload,
        headers={"Content-Type": "application/json"},
        method="POST"
    )
    try:
        with request.urlopen(req, timeout=10) as resp:
            body = json.loads(resp.read().decode("utf-8"))
            return True, body
    except error.URLError as e:
        return False, str(e)
    except error.HTTPError as e:
        return False, f"HTTP {e.code}: {e.read().decode('utf-8')}"


def seed_historical_data(hours=24):
    print(f"正在注入 {hours} 小时历史数据...")
    now = time.time()
    count = 0
    for hours_ago in range(hours, 0, -1):
        tick = int((now - hours_ago * 3600) / INTERVAL_SECONDS)
        for wheel in WATERWHEEL_CONFIGS[:WHEEL_COUNT]:
            data = generate_telemetry(wheel, tick)
            data["time"] = datetime.utcfromtimestamp(now - hours_ago * 3600).isoformat() + "Z"
            ok, resp = send_telemetry(data)
            if ok:
                count += 1
            else:
                print(f"  注入失败 {wheel['name']}: {resp}")
    print(f"历史数据注入完成，共 {count} 条")


def run_continuous():
    print(f"筒车遥测模拟器启动")
    print(f"  模式: {SIMULATION_MODE}")
    print(f"  上报间隔: {INTERVAL_SECONDS} 秒")
    print(f"  筒车数量: {WHEEL_COUNT}")
    print(f"  目标API: {API_BASE}")
    print("=" * 60)

    tick = int(time.time() / INTERVAL_SECONDS)
    fail_count = 0

    while True:
        ts = datetime.now().strftime("%Y-%m-%d %H:%M:%S")
        print(f"\n[{ts}] 上报周期 #{tick} (模式:{SIMULATION_MODE})")

        success_count = 0
        for wheel in WATERWHEEL_CONFIGS[:WHEEL_COUNT]:
            data = generate_telemetry(wheel, tick)
            ok, resp = send_telemetry(data)
            if ok:
                success_count += 1
                status = "✓"
            else:
                fail_count += 1
                status = "✗"
            print(f"  {status} {wheel['name']}: rpm={data['rotation_speed']:.2f} flow={data['flow_velocity']:.2f}m/s lift={data['water_lift']:.1f}m³/h")

        if success_count == WHEEL_COUNT:
            fail_count = 0

        tick += 1
        next_time = tick * INTERVAL_SECONDS
        sleep_secs = max(1, next_time - time.time())
        print(f"  成功 {success_count}/{WHEEL_COUNT}，下次上报 {sleep_secs:.0f}s 后")
        time.sleep(sleep_secs)


def wait_for_backend(max_wait=120):
    print(f"等待后端服务就绪 ({API_BASE})...")
    start = time.time()
    while time.time() - start < max_wait:
        try:
            req = request.Request(f"{API_BASE}/health", method="GET")
            with request.urlopen(req, timeout=5) as resp:
                if resp.status == 200:
                    print("后端服务就绪")
                    return True
        except Exception:
            pass
        time.sleep(2)
    print(f"警告：{max_wait}s 内后端未就绪，仍继续尝试")
    return False


if __name__ == "__main__":
    wait_for_backend()

    if "--seed" in sys.argv or os.environ.get("SEED_HISTORICAL", "0") == "1":
        seed_historical_data(int(os.environ.get("SEED_HOURS", "24")))

    try:
        run_continuous()
    except KeyboardInterrupt:
        print("\n模拟器已停止")
