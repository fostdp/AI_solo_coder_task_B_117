class App {
    constructor() {
        const cfg = window.AppConfig;
        this.apiBase = cfg.apiBase;
        this.refreshIntervalMs = cfg.refreshIntervalMs;
        this.chartRefreshIntervalMs = cfg.chartRefreshIntervalMs;
        this.speedOptions = cfg.view.speedOptions;

        this.waterwheels = [];
        this.selectedWheel = null;
        this.latestTelemetry = {};
        this.view = null;
        this.panel = null;
        this.refreshInterval = null;
        this.speedLevel = 0;
        this.isPlaying = true;
        this._lastChartRefresh = 0;

        this.init();
    }

    init() {
        this.view = new NorriaView('waterwheelCanvas');
        this.view.onClick = () => this.showDetailModal();
        this.panel = new EfficiencyPanel(this.apiBase);

        this.schedulerPanel = null;
        this.forecastPanel = null;
        this.efficiencyComparePanel = null;
        this.virtualBuilder = null;
        this.activeTab = 'monitor';
        this.buildPresetsCache = [];

        window.addEventListener('resize', () => this.onResize());

        this.bindEvents();
        this.initTabs();
        this.loadWaterwheels();
        this.startDataRefresh();
    }

    onResize() {
        if (this.panel) {
            this.panel.refreshChartSize();
        }
    }

    bindEvents() {
        document.getElementById('playPauseBtn').addEventListener('click', () => this.togglePlay());
        document.getElementById('speedBtn').addEventListener('click', () => this.cycleSpeed());
        document.getElementById('chartRange').addEventListener('change', () => this.refreshEfficiencyChart());
        document.getElementById('runOptimizationBtn').addEventListener('click', () => this.runOptimization());
    }

    async loadWaterwheels() {
        try {
            const res = await fetch(`${this.apiBase}/waterwheels`);
            this.waterwheels = await res.json();
            this.renderWaterwheelList();
            this.updateHeaderStats();
        } catch (e) {
            console.error('加载筒车列表失败:', e);
        }
    }

    renderWaterwheelList() {
        const list = document.getElementById('waterwheelList');
        list.innerHTML = '';

        for (const wheel of this.waterwheels) {
            const telemetry = this.latestTelemetry[wheel.id];
            let statusClass = 'status-offline';
            if (telemetry) {
                const mechEff = telemetry.mechanical_efficiency ?? 0.5;
                const hydEff = telemetry.hydraulic_efficiency ?? 0.5;
                if (mechEff * hydEff > 0.35) {
                    statusClass = 'status-online';
                } else {
                    statusClass = 'status-warning';
                }
            }

            const item = document.createElement('div');
            item.className = 'wheel-item' + (this.selectedWheel?.id === wheel.id ? ' active' : '');
            item.innerHTML = `
                <div class="wheel-item-name">
                    <span class="wheel-item-status ${statusClass}"></span>${wheel.name}
                </div>
                <div class="wheel-item-loc">${wheel.location}</div>
            `;
            item.addEventListener('click', () => this.selectWaterwheel(wheel));
            list.appendChild(item);
        }
    }

    selectWaterwheel(wheel) {
        this.selectedWheel = wheel;
        this.view.setWaterwheel(wheel);
        document.getElementById('canvasTitle').textContent = `${wheel.name} - ${wheel.location}`;
        document.getElementById('playPauseBtn').disabled = false;
        document.getElementById('speedBtn').disabled = false;
        document.getElementById('runOptimizationBtn').disabled = false;

        this.renderWaterwheelList();
        this.loadWheelData();
    }

    async loadWheelData() {
        if (!this.selectedWheel) return;

        await Promise.all([
            this.loadLatestTelemetry(),
            this.refreshEfficiencyChart(),
            this.loadAnalysis(),
            this.loadAlerts(),
            this.loadOptimizations()
        ]);
    }

    async loadLatestTelemetry() {
        if (!this.selectedWheel) return;
        try {
            const res = await fetch(`${this.apiBase}/waterwheels/${this.selectedWheel.id}/telemetry?limit=1`);
            const data = await res.json();
            if (data && data.length > 0) {
                this.latestTelemetry[this.selectedWheel.id] = data[0];
                this.view.setTelemetry(data[0]);
                this.panel.renderMetrics(data[0]);
            }
        } catch (e) {
            console.error('加载遥测数据失败:', e);
        }
    }

    async refreshEfficiencyChart() {
        if (!this.selectedWheel) return;
        const hours = parseInt(document.getElementById('chartRange').value);
        await this.panel.fetchAndDrawChart(this.selectedWheel.id, hours);
    }

    async loadAnalysis() {
        if (!this.selectedWheel) return;
        await this.panel.fetchAndRenderAnalysis(this.selectedWheel.id);
    }

    async loadAlerts() {
        if (!this.selectedWheel) return;
        await this.panel.fetchAndRenderAlerts(this.selectedWheel.id);
    }

    async loadOptimizations() {
        if (!this.selectedWheel) return;
        await this.panel.fetchAndRenderOptimizations(this.selectedWheel.id);
    }

    async runOptimization() {
        if (!this.selectedWheel) return;
        const btn = document.getElementById('runOptimizationBtn');
        await this.panel.runOptimization(this.selectedWheel.id, btn);
    }

    showDetailModal() {
        if (!this.selectedWheel) return;

        const wheel = this.selectedWheel;
        const telemetry = this.latestTelemetry[wheel.id];

        document.getElementById('modalTitle').textContent = wheel.name;
        document.getElementById('modalBody').innerHTML = `
            <div class="analysis-grid" style="margin-bottom: 20px;">
                <div class="analysis-item">
                    <div class="analysis-item-label">位置</div>
                    <div class="analysis-item-value">${wheel.location}</div>
                </div>
                <div class="analysis-item">
                    <div class="analysis-item-label">直径</div>
                    <div class="analysis-item-value">${wheel.diameter} m</div>
                </div>
                <div class="analysis-item">
                    <div class="analysis-item-label">水斗数量</div>
                    <div class="analysis-item-value">${wheel.bucket_count} 个</div>
                </div>
                <div class="analysis-item">
                    <div class="analysis-item-label">单斗容量</div>
                    <div class="analysis-item-value">${wheel.bucket_capacity * 1000} L</div>
                </div>
                <div class="analysis-item" style="grid-column: span 2;">
                    <div class="analysis-item-label">最大提水量</div>
                    <div class="analysis-item-value">${wheel.max_flow_rate} m³/h</div>
                </div>
            </div>
            ${telemetry ? `
                <h3 style="color: #4fc3f7; margin-bottom: 12px; font-size: 1rem;">实时遥测</h3>
                <div class="analysis-grid">
                    <div class="analysis-item">
                        <div class="analysis-item-label">转速</div>
                        <div class="analysis-item-value">${telemetry.rotation_speed?.toFixed(2)} rpm</div>
                    </div>
                    <div class="analysis-item">
                        <div class="analysis-item-label">提水量</div>
                        <div class="analysis-item-value">${telemetry.water_lift?.toFixed(1)} m³/h</div>
                    </div>
                    <div class="analysis-item">
                        <div class="analysis-item-label">水位落差</div>
                        <div class="analysis-item-value">${telemetry.water_level_drop?.toFixed(2)} m</div>
                    </div>
                    <div class="analysis-item">
                        <div class="analysis-item-label">水流流速</div>
                        <div class="analysis-item-value">${telemetry.flow_velocity?.toFixed(2)} m/s</div>
                    </div>
                    <div class="analysis-item">
                        <div class="analysis-item-label">机械效率</div>
                        <div class="analysis-item-value">${((telemetry.mechanical_efficiency ?? 0) * 100).toFixed(1)}%</div>
                    </div>
                    <div class="analysis-item">
                        <div class="analysis-item-label">水力效率</div>
                        <div class="analysis-item-value">${((telemetry.hydraulic_efficiency ?? 0) * 100).toFixed(1)}%</div>
                    </div>
                </div>
            ` : '<div class="analysis-empty">暂无遥测数据</div>'}
        `;

        document.getElementById('detailModal').classList.remove('hidden');
    }

    togglePlay() {
        this.isPlaying = !this.isPlaying;
        this.view.setRunning(this.isPlaying);
        document.getElementById('playPauseBtn').textContent = this.isPlaying ? '⏸ 暂停' : '▶ 播放';
    }

    cycleSpeed() {
        this.speedLevel = (this.speedLevel + 1) % this.speedOptions.length;
        const speed = this.speedOptions[this.speedLevel];
        this.view.setSpeed(speed);
        document.getElementById('speedBtn').textContent = speed + 'x 速度';
    }

    startDataRefresh() {
        this.refreshInterval = setInterval(() => this.refreshData(), this.refreshIntervalMs);
    }

    async refreshData() {
        await this.loadLatestTelemetry();
        this.renderWaterwheelList();
        this.updateHeaderStats();

        if (this.selectedWheel) {
            const now = new Date();
            const lastRefresh = this._lastChartRefresh || 0;
            if (now.getTime() - lastRefresh > this.chartRefreshIntervalMs) {
                this.refreshEfficiencyChart();
                this.loadAlerts();
                this._lastChartRefresh = now.getTime();
            }
        }
    }

    updateHeaderStats() {
        document.getElementById('totalWheels').textContent = this.waterwheels.length;

        let online = 0;
        for (const wheel of this.waterwheels) {
            if (this.latestTelemetry[wheel.id]) {
                online++;
            }
        }
        document.getElementById('onlineWheels').textContent = online;
        document.getElementById('alertCount').textContent = '0';
    }

    initTabs() {
        const tabs = document.querySelectorAll('#tabsNav .tab-btn');
        tabs.forEach(btn => {
            btn.addEventListener('click', () => this.switchTab(btn.dataset.tab));
        });
    }

    async switchTab(tabName) {
        if (this.activeTab === tabName) return;

        document.querySelectorAll('#tabsNav .tab-btn').forEach(b => {
            b.classList.toggle('active', b.dataset.tab === tabName);
        });

        document.querySelectorAll('.tab-content').forEach(el => {
            el.classList.toggle('hidden', el.id !== `tab-${tabName}`);
        });

        const sidebar = document.getElementById('monitorSidebar');
        if (sidebar) {
            sidebar.style.display = (tabName === 'monitor') ? '' : 'none';
        }

        this.activeTab = tabName;

        if (tabName === 'monitor') {
            if (this.view) this.view.resize();
            return;
        }

        if (tabName === 'scheduler' && !this.schedulerPanel) {
            if (typeof IrrigationSchedulerPanel !== 'undefined') {
                this.schedulerPanel = new IrrigationSchedulerPanel('#schedulerPanelRoot', this.apiBase);
            }
        }

        if (tabName === 'forecast' && !this.forecastPanel) {
            if (typeof WaterLevelForecastPanel !== 'undefined') {
                this.forecastPanel = new WaterLevelForecastPanel('#forecastPanelRoot', this.apiBase, this.waterwheels);
            } else if (window.WaterLevelForecastPanel) {
                this.forecastPanel = new window.WaterLevelForecastPanel('#forecastPanelRoot', this.apiBase, this.waterwheels);
            }
        }

        if (tabName === 'efficiency' && !this.efficiencyComparePanel) {
            if (typeof EfficiencyComparePanel !== 'undefined') {
                this.efficiencyComparePanel = new EfficiencyComparePanel('#efficiencyPanelRoot', this.apiBase, this.waterwheels);
            }
        }

        if (tabName === 'build' && !this.virtualBuilder) {
            try {
                if (this.buildPresetsCache.length === 0) {
                    const presets = await this._fetchBuildPresets();
                    this.buildPresetsCache = presets || [];
                }
                if (typeof VirtualBuilder !== 'undefined') {
                    this.virtualBuilder = new VirtualBuilder('#builderPanelRoot', {
                        apiBase: this.apiBase,
                        presets: this.buildPresetsCache,
                    });
                }
            } catch (e) {
                console.error('初始化虚拟建造器失败:', e);
                if (typeof VirtualBuilder !== 'undefined') {
                    this.virtualBuilder = new VirtualBuilder('#builderPanelRoot', { apiBase: this.apiBase });
                }
            }
        }

        if (tabName === 'forecast' && this.forecastPanel && typeof this.forecastPanel.setWaterwheels === 'function') {
            this.forecastPanel.setWaterwheels(this.waterwheels);
        }
        if (tabName === 'efficiency' && this.efficiencyComparePanel && typeof this.efficiencyComparePanel.setWaterwheels === 'function') {
            this.efficiencyComparePanel.setWaterwheels(this.waterwheels);
        }
    }

    async _fetchBuildPresets() {
        try {
            const res = await fetch(`${this.apiBase}/build-presets`);
            if (!res.ok) return [];
            const data = await res.json();
            return data && Array.isArray(data) ? data : [];
        } catch (e) {
            console.warn('获取建造模板失败，使用内置模板');
            return [
                { id: 'dujiangyan24', name: '都江堰24斗筒车', culture: '岷江文化' },
                { id: 'fenghuang28', name: '凤凰28斗筒车', culture: '湘西文化' },
                { id: 'lijiang18', name: '丽江18斗筒车', culture: '纳西文化' },
            ];
        }
    }
}

document.addEventListener('DOMContentLoaded', () => {
    window.app = new App();
});
