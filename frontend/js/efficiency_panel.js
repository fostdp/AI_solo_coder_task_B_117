class EfficiencyPanel {
    constructor(apiBase) {
        const cfg = window.AppConfig;
        this.apiBase = apiBase || cfg.apiBase;
        this.config = cfg.panel;
        this.colors = cfg.colors;

        this.chartCanvas = null;
        this.chartCtx = null;
        this.chartDims = null;
        this._lastData = null;

        this._initCanvases();
    }

    _initCanvases() {
        const canvas = document.getElementById('efficiencyChart');
        if (!canvas) return;

        this.chartCanvas = canvas;
        this.chartCtx = canvas.getContext('2d');
        this._recalcChartDims();
    }

    _recalcChartDims() {
        if (!this.chartCanvas) return null;
        const rect = this.chartCanvas.getBoundingClientRect();
        const dpr = window.devicePixelRatio || 1;
        const h = this.config.chartHeightPx;
        this.chartCanvas.width = rect.width * dpr;
        this.chartCanvas.height = h * dpr;
        this.chartCtx.setTransform(dpr, 0, 0, dpr, 0, 0);

        this.chartDims = {
            w: rect.width,
            h: h,
            padL: this.config.padL,
            padR: this.config.padR,
            padT: this.config.padT,
            padB: this.config.padB,
        };
        this.chartDims.cw = this.chartDims.w - this.chartDims.padL - this.chartDims.padR;
        this.chartDims.ch = this.chartDims.h - this.chartDims.padT - this.chartDims.padB;
        return this.chartDims;
    }

    refreshChartSize() {
        this._recalcChartDims();
        if (this._lastData) {
            this.drawEfficiencyChart(this._lastData);
        }
    }

    async fetchAndDrawChart(wheelId, hours) {
        const end = new Date();
        const start = new Date(end.getTime() - hours * 3600 * 1000);
        const url = `${this.apiBase}/waterwheels/${wheelId}/telemetry/range?start=${encodeURIComponent(start.toISOString())}&end=${encodeURIComponent(end.toISOString())}`;

        try {
            const res = await fetch(url);
            const data = await res.json();
            this.drawEfficiencyChart(data);
            return data;
        } catch (e) {
            console.error('加载效率曲线失败:', e);
            return null;
        }
    }

    drawEfficiencyChart(data) {
        if (!this.chartCtx) return;
        this._lastData = data;
        const dims = this.chartDims || this._recalcChartDims();
        if (!dims) return;

        const ctx = this.chartCtx;
        const { w, h, padL, padR, padT, padB, cw, ch } = dims;

        ctx.clearRect(0, 0, w, h);

        ctx.strokeStyle = 'rgba(255, 255, 255, 0.08)';
        ctx.lineWidth = 1;
        for (let i = 0; i <= 4; i++) {
            const y = padT + (ch / 4) * i;
            ctx.beginPath();
            ctx.moveTo(padL, y);
            ctx.lineTo(w - padR, y);
            ctx.stroke();

            ctx.fillStyle = '#78909c';
            ctx.font = '11px sans-serif';
            ctx.textAlign = 'right';
            ctx.fillText(((4 - i) * 25) + '%', padL - 6, y + 4);
        }

        if (!data || data.length === 0) {
            ctx.fillStyle = '#78909c';
            ctx.font = '14px sans-serif';
            ctx.textAlign = 'center';
            ctx.fillText('暂无数据', w / 2, h / 2);
            return;
        }

        const drawLine = (getter, color) => {
            const validData = data.filter(d => getter(d) != null);
            if (validData.length < 2) return;

            ctx.strokeStyle = color;
            ctx.lineWidth = 2;
            ctx.beginPath();
            validData.forEach((d, i) => {
                const x = padL + (i / (validData.length - 1)) * cw;
                const v = Math.min(1, Math.max(0, getter(d)));
                const y = padT + ch - v * ch;
                if (i === 0) ctx.moveTo(x, y);
                else ctx.lineTo(x, y);
            });
            ctx.stroke();
        };

        drawLine(d => d.mechanical_efficiency ?? 0, this.colors.info);
        drawLine(d => d.hydraulic_efficiency ?? 0, this.colors.good);
        drawLine(d => (d.mechanical_efficiency ?? 0) * (d.hydraulic_efficiency ?? 0), this.colors.warning);

        ctx.font = '11px sans-serif';
        const legendY = 5;
        const items = [
            { label: '机械效率', color: this.colors.info },
            { label: '水力效率', color: this.colors.good },
            { label: '综合效率', color: this.colors.warning }
        ];
        let lx = padL;
        for (const item of items) {
            ctx.fillStyle = item.color;
            ctx.fillRect(lx, legendY, 12, 12);
            ctx.fillStyle = '#b0bec5';
            ctx.textAlign = 'left';
            ctx.fillText(item.label, lx + 16, legendY + 10);
            lx += ctx.measureText(item.label).width + 40;
        }

        if (data.length > 0) {
            ctx.fillStyle = '#78909c';
            ctx.font = '10px sans-serif';
            ctx.textAlign = 'left';
            const firstTime = new Date(data[0].time);
            ctx.fillText(firstTime.toLocaleString('zh-CN', { hour: '2-digit', minute: '2-digit' }), padL, h - 8);

            ctx.textAlign = 'right';
            const lastTime = new Date(data[data.length - 1].time);
            ctx.fillText(lastTime.toLocaleString('zh-CN', { hour: '2-digit', minute: '2-digit' }), w - padR, h - 8);
        }
    }

    renderMetrics(data) {
        const setText = (id, val) => {
            const el = document.getElementById(id);
            if (el) el.textContent = val;
        };

        setText('metricSpeed', data.rotation_speed != null ? data.rotation_speed.toFixed(2) : '--');
        setText('metricLift', data.water_lift != null ? data.water_lift.toFixed(1) : '--');
        setText('metricDrop', data.water_level_drop != null ? data.water_level_drop.toFixed(2) : '--');
        setText('metricFlow', data.flow_velocity != null ? data.flow_velocity.toFixed(2) : '--');

        const mechEff = ((data.mechanical_efficiency ?? 0) * 100);
        const hydEff = ((data.hydraulic_efficiency ?? 0) * 100);
        setText('metricMechEff', mechEff.toFixed(1));
        setText('metricHydEff', hydEff.toFixed(1));
    }

    async fetchAndRenderAnalysis(wheelId) {
        try {
            const res = await fetch(`${this.apiBase}/waterwheels/${wheelId}/efficiency`);
            const analysis = await res.json();
            this.renderAnalysis(analysis);
            return analysis;
        } catch (e) {
            console.error('加载分析数据失败:', e);
            return null;
        }
    }

    renderAnalysis(analysis) {
        const container = document.getElementById('analysisContent');
        if (!container) return;
        if (!analysis) {
            container.innerHTML = '<div class="analysis-empty">暂无分析数据</div>';
            return;
        }

        const formatNum = (n, unit = '') => {
            if (n == null) return '--';
            if (Math.abs(n) < 100) return n.toFixed(2) + unit;
            return n.toFixed(1) + unit;
        };

        const getEffClass = (v) => {
            if (v >= 0.6) return 'good';
            if (v >= 0.4) return 'warning';
            return 'danger';
        };

        const col = this.colors;
        const mechEff = analysis.mechanical_efficiency ?? 0;
        const hydEff = analysis.hydraulic_efficiency ?? 0;
        const overall = analysis.overall_efficiency ?? 0;

        container.innerHTML = `
            <div class="analysis-grid">
                <div class="analysis-item">
                    <div class="analysis-item-label">输入功率</div>
                    <div class="analysis-item-value">${formatNum(analysis.input_power, ' W')}</div>
                </div>
                <div class="analysis-item">
                    <div class="analysis-item-label">输出功率</div>
                    <div class="analysis-item-value">${formatNum(analysis.output_power, ' W')}</div>
                </div>
                <div class="analysis-item">
                    <div class="analysis-item-label">输入转矩</div>
                    <div class="analysis-item-value">${formatNum(analysis.torque_input, ' N·m')}</div>
                </div>
                <div class="analysis-item">
                    <div class="analysis-item-label">输出转矩</div>
                    <div class="analysis-item-value">${formatNum(analysis.torque_output, ' N·m')}</div>
                </div>
                <div class="analysis-item">
                    <div class="analysis-item-label">提水阻力</div>
                    <div class="analysis-item-value">${formatNum(analysis.lift_resistance, ' N·m')}</div>
                </div>
                <div class="analysis-item">
                    <div class="analysis-item-label">转速</div>
                    <div class="analysis-item-value">${formatNum(analysis.rotation_speed, ' rpm')}</div>
                </div>
                <div class="analysis-item">
                    <div class="analysis-item-label">机械效率</div>
                    <div class="analysis-item-value ${getEffClass(mechEff)}" style="color:${getEffClass(mechEff) === 'good' ? col.good : getEffClass(mechEff) === 'warning' ? col.warning : col.danger}">${(mechEff * 100).toFixed(1)}%</div>
                </div>
                <div class="analysis-item">
                    <div class="analysis-item-label">水力利用率</div>
                    <div class="analysis-item-value ${getEffClass(hydEff)}" style="color:${getEffClass(hydEff) === 'good' ? col.good : getEffClass(hydEff) === 'warning' ? col.warning : col.danger}">${(hydEff * 100).toFixed(1)}%</div>
                </div>
                <div class="analysis-item" style="grid-column: span 2;">
                    <div class="analysis-item-label">综合效率</div>
                    <div class="analysis-item-value ${getEffClass(overall)}" style="font-size: 1.4rem; color:${getEffClass(overall) === 'good' ? col.good : getEffClass(overall) === 'warning' ? col.warning : col.danger}">${(overall * 100).toFixed(1)}%</div>
                </div>
            </div>
        `;
    }

    async fetchAndRenderAlerts(wheelId) {
        try {
            const res = await fetch(`${this.apiBase}/waterwheels/${wheelId}/alerts?limit=${this.config.alertLimit}`);
            const alerts = await res.json();
            this.renderAlerts(alerts);
            return alerts;
        } catch (e) {
            console.error('加载告警失败:', e);
            return null;
        }
    }

    renderAlerts(alerts) {
        const container = document.getElementById('alertsList');
        if (!container) return;
        if (!alerts || alerts.length === 0) {
            container.innerHTML = '<div class="analysis-empty">暂无告警记录</div>';
            return;
        }

        const severColor = (sev) => {
            const col = this.colors;
            switch (sev) {
                case 'critical': return col.danger;
                case 'major': return col.warning;
                default: return col.info;
            }
        };

        container.innerHTML = alerts.map(a => {
            const msg = a.message || '效率异常告警';
            const sev = a.severity || a.severity;
            return `
                <div class="alert-item" style="border-left: 3px solid ${severColor(sev)};">
                    <div class="alert-item-time">${new Date(a.time).toLocaleString('zh-CN')}</div>
                    <div class="alert-item-msg">${msg}</div>
                </div>
            `;
        }).join('');
    }

    async fetchAndRenderOptimizations(wheelId) {
        try {
            const res = await fetch(`${this.apiBase}/waterwheels/${wheelId}/optimizations?limit=${this.config.optHistoryLimit}`);
            const results = await res.json();
            this.renderOptimizations(results);
            return results;
        } catch (e) {
            console.error('加载优化记录失败:', e);
            return null;
        }
    }

    renderOptimizations(results) {
        const container = document.getElementById('optimizationList');
        if (!container) return;
        if (!results || results.length === 0) {
            container.innerHTML = '<div class="analysis-empty">暂无优化记录</div>';
            return;
        }

        container.innerHTML = results.map(r => {
            const angle = r.optimal_bucket_angle ?? r.bucket_angle ?? 0;
            const predLift = r.predicted_lift_lph ?? r.predictedLift ?? r.optimized_lift_rate ?? r.optimizedLift ?? 0;
            const improve = r.predicted_improvement_percent ?? r.improvement_percent ?? r.predictedImprovement ?? 0;
            const gens = r.generations ?? r.generation_count ?? r.Generations ?? 0;
            const t = r.time ?? r.created_at ?? r.Time ?? new Date().toISOString();
            return `
                <div class="opt-item">
                    <div class="opt-item-time">${new Date(t).toLocaleString('zh-CN')}</div>
                    <div class="opt-item-detail">
                        预测提水量: <strong>${predLift.toFixed(1)} L/h</strong>
                        (提升 ${improve.toFixed(1)}%)<br>
                        水斗最优布置角: ${angle.toFixed(1)}° | 进化代数: ${gens}
                    </div>
                </div>
            `;
        }).join('');
    }

    async runOptimization(wheelId, btnElement) {
        const btn = btnElement;
        if (btn) {
            btn.disabled = true;
            const origText = btn.textContent;
            btn.textContent = '优化中...';

            const controller = new AbortController();
            const timeout = setTimeout(() => controller.abort(), this.config.optimizationTimeoutMs);

            try {
                const res = await fetch(`${this.apiBase}/waterwheels/${wheelId}/optimize`, {
                    method: 'POST',
                    signal: controller.signal,
                });
                clearTimeout(timeout);
                if (res.ok) {
                    await this.fetchAndRenderOptimizations(wheelId);
                }
            } catch (e) {
                if (e.name === 'AbortError') {
                    console.warn('优化请求超时');
                } else {
                    console.error('运行优化失败:', e);
                }
            } finally {
                btn.disabled = false;
                btn.textContent = origText || '运行结构优化';
            }
        }
    }
}

if (typeof window !== 'undefined') {
    window.EfficiencyPanel = EfficiencyPanel;
}
