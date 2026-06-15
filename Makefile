.PHONY: init-db run-backend run-simulator run-all tidy

init-db:
	@echo "初始化 TimescaleDB 数据库..."
	psql -U postgres -c "CREATE DATABASE waterwheel;" 2>/dev/null || true
	psql -U postgres -d waterwheel -f sql/init.sql

run-backend:
	@echo "启动后端服务..."
	cd backend && go run ./cmd/server

run-simulator:
	@echo "启动筒车模拟器 (含历史数据种子)..."
	python3 simulator/simulator.py --seed

run-simulator-live:
	@echo "启动筒车模拟器 (仅实时上报)..."
	python3 simulator/simulator.py

tidy:
	cd backend && go mod tidy

build-backend:
	cd backend && go build -o ../bin/server ./cmd/server

help:
	@echo "古代筒车监测系统 - Makefile 帮助"
	@echo "  make init-db           - 初始化 TimescaleDB 数据库"
	@echo "  make run-backend       - 启动 Go 后端服务"
	@echo "  make run-simulator     - 启动模拟器并注入24h历史数据"
	@echo "  make run-simulator-live - 启动模拟器仅实时上报"
	@echo "  make tidy              - 整理 Go 依赖"
	@echo "  make build-backend     - 编译后端二进制"
