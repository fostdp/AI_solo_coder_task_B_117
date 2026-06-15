/**
 * FeaturePanels - 三大新功能面板：
 *   1. IrrigationSchedulerPanel    - 多筒车协同灌溉调度
 *   2. WaterLevelForecastPanel     - 季节性水位预测与高度调节
 *   3. EfficiencyComparePanel      - 古代筒车 vs 现代水泵 能效对比
 */

// ============================================================
// 面板一: 灌溉调度
// ============================================================
class IrrigationSchedulerPanel {
  constructor(rootEl, apiBase) {
    this.root = typeof rootEl === 'string' ? document.querySelector(rootEl) : rootEl;
    this.api = apiBase || '/api';
    this.fields = [];
    this.selectedField = null;
    this.lastSolution = null;
    this._build();
    this._loadFields();
  }

  _build() {
    this.root.innerHTML = `
      <div class="fp-layout">
        <div class="fp-section">
          <h3 class="fp-title">🌾 灌溉田块</h3>
          <div class="fp-field-list" id="fpFieldList"></div>
        </div>
        <div class="fp-section fp-main">
          <h3 class="fp-title">⚙️ 调度参数</h3>
          <div class="fp-form" id="fpForm" style="display:none">
            <div class="fp-row">
              <label>目标水量 (m³)：<input type="number" id="schTarget" value="500" step="50" min="0"/></label>
              <label>时间要求 (小时)：<input type="number" id="schDeadline" value="24" step="1" min="1" max="168"/></label>
              <label>电价 (元/kWh)：<input type="number" id="schElec" value="0.85" step="0.05" min="0"/></label>
            </div>
            <div class="fp-row">
              <label><input type="checkbox" id="schPump" checked /> 允许启用电动水泵补充</label>
            </div>
            <div class="fp-row">
              <label>选用筒车：</label>
              <div class="fp-chip-row" id="schWheels"></div>
            </div>
            <div class="fp-row">
              <button class="btn btn-primary" id="schRunBtn">🚀 运行最优调度</button>
            </div>
          </div>
          <div class="fp-empty" id="fpEmpty" style="display:flex">请从左侧选择一个田块查看调度方案</div>
        </div>
        <div class="fp-section">
          <h3 class="fp-title">📋 调度方案结果</h3>
          <div class="fp-solution" id="fpSolution">暂无调度方案，运行后将显示最优组合</div>
          <h3 class="fp-title" style="margin-top:18px">📜 历史方案</h3>
          <div class="fp-history" id="fpHistory">暂无历史记录</div>
        </div>
      </div>
    `;

    this.root.querySelector('#schRunBtn').addEventListener('click', () => this._runSchedule());
  }

  async _loadFields() {
    try {
      const res = await fetch(`${this.api}/irrigation/fields`);
      this.fields = await res.json();
      const list = this.root.querySelector('#fpFieldList');
      list.innerHTML = this.fields.map(f => `
        <div class="fp-field-item" data-id="${f.id}">
          <div class="fp-field-head">
            <span class="fp-field-name">${f.name}</span>
            <span class="fp-field-prio">优先级 ${f.priority}</span>
          </div>
          <div class="fp-field-meta">${f.location} · ${f.crop_type} · ${f.area_hectare}公顷</div>
          <div class="fp-field-water">
            <div class="fp-field-water-bar">
              <div style="width:${Math.min(100, (f.current_filled_m3 / f.daily_water_req_m3) * 100)}%"></div>
            </div>
            <div class="fp-field-water-text">
              需水 ${f.daily_water_req_m3.toFixed(0)} m³/日
              <span style="margin-left:auto">已达 ${((f.current_filled_m3 / f.daily_water_req_m3) * 100).toFixed(0)}%</span>
            </div>
          </div>
        </div>
      `).join('');
      list.querySelectorAll('.fp-field-item').forEach(el => {
        el.addEventListener('click', () => this._selectField(parseInt(el.dataset.id)));
      });
    } catch (e) {
      this.root.querySelector('#fpFieldList').innerHTML = `<div class="fp-empty">加载田块失败：${e.message}</div>`;
    }
  }

  async _selectField(id) {
    this.selectedField = this.fields.find(f => f.id === id);
    this.root.querySelectorAll('.fp-field-item').forEach(el => el.classList.toggle('active', parseInt(el.dataset.id) === id));
    if (!this.selectedField) return;

    this.root.querySelector('#fpEmpty').style.display = 'none';
    const form = this.root.querySelector('#fpForm');
    form.style.display = 'block';
    this.root.querySelector('#schTarget').value = Math.ceil(this.selectedField.daily_water_req_m3);
    this.root.querySelector('#schWheels').innerHTML =
      (this.selectedField.assigned_waterwheels || []).map(wid => `
        <label class="fp-chip">
          <input type="checkbox" data-wid="${wid}" checked /> 筒车 #${wid}
        </label>
      `).join('') || '<span style="color:#888">未分配筒车，请手动在后端添加</span>';

    await this._loadHistory();
  }

  async _runSchedule() {
    const widEls = this.root.querySelectorAll('#schWheels input[type=checkbox]:checked');
    const useIDs = Array.from(widEls).map(el => parseInt(el.dataset.wid));
    const body = {
      field_id: this.selectedField.id,
      target_water_m3: parseFloat(this.root.querySelector('#schTarget').value),
      deadline_hours: parseInt(this.root.querySelector('#schDeadline').value),
      electricity_cost_per_kwh: parseFloat(this.root.querySelector('#schElec').value),
      allow_electric_pump: this.root.querySelector('#schPump').checked,
      use_waterwheel_ids: useIDs,
    };
    try {
      this.root.querySelector('#fpSolution').innerHTML = '<div class="fp-loading">⏳ 求解线性规划最优组合...</div>';
      const res = await fetch(`${this.api}/irrigation/schedule`, {
        method: 'POST', headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(body),
      });
      const sol = await res.json();
      if (!res.ok) throw new Error(sol.error || '求解失败');
      this.lastSolution = sol;
      this._renderSolution(sol);
      await this._loadHistory();
    } catch (e) {
      this.root.querySelector('#fpSolution').innerHTML = `<div class="fp-empty" style="color:#C62828">❌ ${e.message}</div>`;
    }
  }

  _renderSolution(sol) {
    const html = `
      <div class="fp-sol-header">
        <div class="fp-sol-stat"><span>总供水量</span><b>${sol.total_water_m3.toFixed(1)} m³</b></div>
        <div class="fp-sol-stat"><span>总耗时</span><b>${sol.total_duration_hours.toFixed(1)} 小时</b></div>
        <div class="fp-sol-stat"><span>总费用</span><b>¥${sol.total_cost_yuan.toFixed(2)}</b></div>
        <div class="fp-sol-stat green"><span>清洁能源占比</span><b>${sol.renewable_ratio.toFixed(1)}%</b></div>
      </div>
      <h5>🛖 筒车运行计划</h5>
      <div class="fp-table-wrap">
        <table class="fp-table">
          <thead><tr><th>筒车ID</th><th>名称</th><th>开机小时</th><th>供水量 m³</th><th>等效省电 kWh</th><th>等效省钱 ¥</th></tr></thead>
          <tbody>
            ${(sol.waterwheel_plans || []).map(p => `
              <tr>
                <td>#${p.waterwheel_id}</td>
                <td>${p.waterwheel_name || '-'}</td>
                <td>${p.run_hours.toFixed(1)} h</td>
                <td>${p.water_m3.toFixed(1)}</td>
                <td style="color:#2E7D32">${p.energy_saved_kwh.toFixed(1)}</td>
                <td style="color:#E65100">${p.cost_saved_yuan.toFixed(2)}</td>
              </tr>
            `).join('')}
          </tbody>
        </table>
      </div>
      ${sol.pump_plan ? `
        <h5>⚡ 电动水泵补充计划</h5>
        <div class="fp-pump-card">
          <div>水泵类型：<b>${sol.pump_plan.pump_type}</b></div>
          <div>运行：<b>${sol.pump_plan.run_hours.toFixed(1)} h</b> · 流量 <b>${sol.pump_plan.flow_rate_m3h} m³/h</b> · 功率 <b>${sol.pump_plan.power_kw} kW</b></div>
          <div>耗电：<b style="color:#C62828">${sol.pump_plan.energy_kwh.toFixed(1)} kWh</b> · 电费 <b style="color:#C62828">¥${sol.pump_plan.cost_yuan.toFixed(2)}</b></div>
        </div>
      ` : ''}
    `;
    this.root.querySelector('#fpSolution').innerHTML = html;
  }

  async _loadHistory() {
    if (!this.selectedField) return;
    try {
      const res = await fetch(`${this.api}/irrigation/schedules?field_id=${this.selectedField.id}&limit=10`);
      const list = await res.json();
      const box = this.root.querySelector('#fpHistory');
      if (!list || list.length === 0) { box.innerHTML = '暂无历史记录'; return; }
      box.innerHTML = list.map(s => `
        <div class="fp-hist-item" data-sol='${JSON.stringify(s).replace(/'/g, '&#39;')}'>
          <div><b>${new Date(s.time).toLocaleString('zh-CN', { month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit' })}</b> · ${s.total_water_m3.toFixed(0)}m³</div>
          <div class="fp-hist-meta">耗时 ${s.total_duration_hours.toFixed(1)}h · 费用¥${s.total_cost_yuan.toFixed(0)} · 绿色${s.renewable_ratio.toFixed(0)}%</div>
        </div>
      `).join('');
      box.querySelectorAll('.fp-hist-item').forEach(el => {
        el.addEventListener('click', () => {
          try { this._renderSolution(JSON.parse(el.dataset.sol)); } catch (e) {}
        });
      });
    } catch (e) {
      this.root.querySelector('#fpHistory').innerHTML = '历史加载失败';
    }
  }
}

// ============================================================
// 面板二: 水位预测与高度调节
// ============================================================
class WaterLevelForecastPanel {
  constructor(rootEl, options) {
    this.root = typeof rootEl === 'string' ? document.querySelector(rootEl) : rootEl;
    this.api = options.apiBase || '/api';
    this.waterwheels = options.waterwheels || [];
    this.selectedID = null;
    this._build();
  }

  _build() {
    this.root.innerHTML = `
      <div class="fp-layout">
        <div class="fp-section">
          <h3 class="fp-title">🛖 选择筒车</h3>
          <div class="fp-wheel-list" id="flWheels">
            ${this.waterwheels.map(w => `
              <div class="fp-wheel-item" data-id="${w.id}">
                <div class="fp-wheel-name">${w.name}</div>
                <div class="fp-wheel-meta">直径${w.diameter}m · ${w.bucket_count}斗</div>
              </div>
            `).join('') || '<div class="fp-empty">请先加载筒车列表</div>'}
          </div>
        </div>
        <div class="fp-section fp-main" style="flex:2">
          <h3 class="fp-title">📊 水位/水文预测图</h3>
          <canvas class="fp-chart" id="flChart" width="560" height="260"></canvas>
          <div class="fp-row">
            <label>预测天数：
              <select id="flHorizon">
                <option value="7">7天</option>
                <option value="30" selected>30天</option>
                <option value="90">90天</option>
              </select>
            </label>
            <label>当前安装高度 (m)：<input type="number" id="flHeight" value="3.5" step="0.05"/></label>
            <button class="btn btn-primary" id="flGenBtn" disabled>🔮 生成水位预测</button>
            <button class="btn btn-success" id="flAdjBtn" disabled>📐 推荐高度调节</button>
          </div>
          <div id="flForecastCard" class="fp-solution">请选择筒车并生成预测</div>
        </div>
        <div class="fp-section">
          <h3 class="fp-title">📐 高度调节建议</h3>
          <div id="flAdjCard" class="fp-solution">暂未生成</div>
          <h3 class="fp-title" style="margin-top:18px">📜 历史调节记录</h3>
          <div id="flAdjList" class="fp-history">暂无记录</div>
        </div>
      </div>
    `;
    this.root.querySelectorAll('.fp-wheel-item').forEach(el => {
      el.addEventListener('click', () => this._select(parseInt(el.dataset.id)));
    });
    this.root.querySelector('#flGenBtn').addEventListener('click', () => this._generateForecast());
    this.root.querySelector('#flAdjBtn').addEventListener('click', () => this._proposeAdjust());
    this.chart = this.root.querySelector('#flChart');
    this.ctx = this.chart.getContext('2d');
  }

  _select(id) {
    this.selectedID = id;
    this.root.querySelectorAll('.fp-wheel-item').forEach(el => el.classList.toggle('active', parseInt(el.dataset.id) === id));
    this.root.querySelector('#flGenBtn').disabled = false;
    const w = this.waterwheels.find(x => x.id === id);
    if (w) this.root.querySelector('#flHeight').value = (w.diameter * 0.45).toFixed(2);
  }

  async _generateForecast() {
    if (!this.selectedID) return;
    try {
      const horizon = parseInt(this.root.querySelector('#flHorizon').value);
      const res = await fetch(`${this.api}/waterwheels/${this.selectedID}/forecast/generate?horizon_days=${horizon}`, { method: 'POST' });
      const f = await res.json();
      if (!res.ok) throw new Error(f.error);
      await this._drawChart(f, horizon);
      this._renderForecast(f);
      this.root.querySelector('#flAdjBtn').disabled = false;
      await this._loadAdjustments();
    } catch (e) {
      this.root.querySelector('#flForecastCard').innerHTML = `<div class="fp-empty" style="color:#C62828">❌ ${e.message}</div>`;
    }
  }

  async _proposeAdjust() {
    if (!this.selectedID) return;
    try {
      const h = parseFloat(this.root.querySelector('#flHeight').value);
      const res = await fetch(`${this.api}/waterwheels/${this.selectedID}/adjustment/propose`, {
        method: 'POST', headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ current_height_m: h })
      });
      const a = await res.json();
      if (!res.ok) throw new Error(a.error);
      this._renderAdjust(a);
      await this._loadAdjustments();
    } catch (e) {
      this.root.querySelector('#flAdjCard').innerHTML = `<div class="fp-empty" style="color:#C62828">❌ ${e.message}</div>`;
    }
  }

  _renderForecast(f) {
    this.root.querySelector('#flForecastCard').innerHTML = `
      <div class="fp-sol-header">
        <div class="fp-sol-stat"><span>预测时间</span><b>${new Date(f.forecast_date).toLocaleDateString('zh-CN')}</b></div>
        <div class="fp-sol-stat"><span>季节</span><b>${f.season}</b></div>
        <div class="fp-sol-stat"><span>置信度</span><b>${(f.confidence * 100).toFixed(0)}%</b></div>
      </div>
      <div class="fp-row">
        <div class="fp-forecast-item">
          <div class="fp-forecast-k">预测水位落差</div>
          <div class="fp-forecast-v blue">${f.predicted_drop.toFixed(2)} m</div>
          <div class="fp-forecast-range">区间 [${f.lower_bound.toFixed(2)} ~ ${f.upper_bound.toFixed(2)}]</div>
        </div>
        <div class="fp-forecast-item">
          <div class="fp-forecast-k">预测水流速度</div>
          <div class="fp-forecast-v green">${f.predicted_flow.toFixed(2)} m/s</div>
        </div>
      </div>
    `;
  }

  _renderAdjust(a) {
    const color = a.adjustment_cm > 0 ? '#E65100' : (a.adjustment_cm < 0 ? '#1565C0' : '#2E7D32');
    const arrow = a.adjustment_cm > 0 ? '⬆' : (a.adjustment_cm < 0 ? '⬇' : '➡');
    this.root.querySelector('#flAdjCard').innerHTML = `
      <div class="fp-sol-header">
        <div class="fp-sol-stat"><span>当前高度</span><b>${a.current_height.toFixed(2)} m</b></div>
        <div class="fp-sol-stat"><span style="color:${color}">建议高度 ${arrow}</span><b style="color:${color}">${a.recommended_height.toFixed(2)} m</b></div>
        <div class="fp-sol-stat"><span>调节量</span><b style="color:${color}">${a.adjustment_cm > 0 ? '+' : ''}${a.adjustment_cm.toFixed(0)} cm</b></div>
      </div>
      <div class="fp-row">
        <div>浸没度 前：<b>${a.submergence_before.toFixed(0)}%</b> → 后：<b style="color:#1565C0">${a.submergence_after.toFixed(0)}%</b></div>
        <div>预计提水增益：<b style="color:#2E7D32">+${a.expected_lift_gain.toFixed(1)}%</b></div>
        <div>预计效率增益：<b style="color:#2E7D32">+${a.expected_eff_gain.toFixed(1)}%</b></div>
      </div>
      <div class="fp-reason">💡 ${a.reason}</div>
      <div class="fp-row">
        <span class="fp-status ${a.status}">状态：${a.status === 'suggested' ? '待执行' : a.status}</span>
        ${a.status === 'suggested' ? `<button class="btn btn-success btn-sm" onclick="__markAdj(${a.id})">✓ 标记为已实施</button>` : ''}
      </div>
    `;
    window.__markAdj = async (id) => {
      await fetch(`${this.api}/adjustments/${id}/implement`, { method: 'POST' });
      this._loadAdjustments();
      this._proposeAdjust();
    };
  }

  async _loadAdjustments() {
    if (!this.selectedID) return;
    try {
      const res = await fetch(`${this.api}/waterwheels/${this.selectedID}/adjustments?limit=10`);
      const list = await res.json();
      const box = this.root.querySelector('#flAdjList');
      if (!list || list.length === 0) { box.innerHTML = '暂无记录'; return; }
      box.innerHTML = list.map(a => `
        <div class="fp-hist-item">
          <div><b>${new Date(a.created_at).toLocaleDateString('zh-CN')}</b> · <span class="fp-status ${a.status}">${a.status}</span></div>
          <div class="fp-hist-meta">${a.current_height.toFixed(2)} → ${a.recommended_height.toFixed(2)}m (${a.adjustment_cm > 0 ? '+' : ''}${a.adjustment_cm.toFixed(0)}cm)</div>
        </div>
      `).join('');
    } catch (e) {}
  }

  async _drawChart(f, horizon) {
    const ctx = this.ctx;
    const W = this.chart.width, H = this.chart.height;
    ctx.clearRect(0, 0, W, H);
    ctx.fillStyle = '#FAFAFA'; ctx.fillRect(0, 0, W, H);

    let points = [];
    try {
      const res = await fetch(`${this.api}/waterwheels/${this.selectedID}/telemetry/range?hours=${Math.min(horizon * 24, 24 * 30)}`);
      const tele = await res.json();
      points = tele.map(t => ({ t: new Date(t.time), v: t.water_level_drop })).filter(p => p.v);
    } catch (e) {}

    const padding = { l: 44, r: 20, t: 20, b: 30 };
    const allVals = [f.predicted_drop, f.lower_bound, f.upper_bound, ...points.map(p => p.v)];
    const min = Math.min(...allVals) * 0.9;
    const max = Math.max(...allVals) * 1.1;
    const cw = W - padding.l - padding.r;
    const ch = H - padding.t - padding.b;
    const Y = v => padding.t + ch - ((v - min) / (max - min)) * ch;

    ctx.strokeStyle = '#E0E0E0'; ctx.lineWidth = 1;
    for (let i = 0; i <= 4; i++) {
      const y = padding.t + (ch / 4) * i;
      ctx.beginPath(); ctx.moveTo(padding.l, y); ctx.lineTo(W - padding.r, y); ctx.stroke();
      ctx.fillStyle = '#616161'; ctx.font = '11px sans-serif';
      ctx.textAlign = 'right';
      ctx.fillText((max - (max - min) * i / 4).toFixed(2) + 'm', padding.l - 6, y + 4);
    }

    const now = Date.now();
    const horizonMs = horizon * 24 * 3600 * 1000;
    const X = ms => padding.l + ((ms - (now - horizonMs / 2)) / horizonMs) * cw;

    if (points.length > 1) {
      ctx.strokeStyle = '#1565C0'; ctx.lineWidth = 2; ctx.beginPath();
      points.forEach((p, i) => {
        const x = X(p.t.getTime());
        const y = Y(p.v);
        i === 0 ? ctx.moveTo(x, y) : ctx.lineTo(x, y);
      });
      ctx.stroke();
    }

    const predX = X(new Date(f.forecast_date).getTime());
    ctx.strokeStyle = '#E65100'; ctx.setLineDash([6, 4]);
    ctx.beginPath(); ctx.moveTo(predX, Y(f.lower_bound)); ctx.lineTo(predX, Y(f.upper_bound)); ctx.stroke();
    ctx.setLineDash([]);
    ctx.fillStyle = '#E65100';
    ctx.beginPath(); ctx.arc(predX, Y(f.predicted_drop), 6, 0, Math.PI * 2); ctx.fill();
    ctx.fillStyle = '#E65100'; ctx.font = 'bold 12px sans-serif'; ctx.textAlign = 'left';
    ctx.fillText(`预测 ${f.predicted_drop.toFixed(2)}m (${f.season})`, predX + 10, Y(f.predicted_drop) - 8);

    ctx.fillStyle = '#616161'; ctx.font = '11px sans-serif'; ctx.textAlign = 'center';
    const labels = [now - horizonMs / 2, now, now + horizonMs / 2];
    labels.forEach(ms => ctx.fillText(new Date(ms).toLocaleDateString('zh-CN', { month: '2-digit', day: '2-digit' }), X(ms), H - 10));
  }
}

// ============================================================
// 面板三: 能效对比
// ============================================================
class EfficiencyComparePanel {
  constructor(rootEl, options) {
    this.root = typeof rootEl === 'string' ? document.querySelector(rootEl) : rootEl;
    this.api = options.apiBase || '/api';
    this.waterwheels = options.waterwheels || [];
    this.selectedID = null;
    this._build();
  }

  _build() {
    this.root.innerHTML = `
      <div class="fp-layout">
        <div class="fp-section">
          <h3 class="fp-title">🛖 选择筒车</h3>
          <div class="fp-wheel-list" id="ecWheels">
            ${this.waterwheels.map(w => `
              <div class="fp-wheel-item" data-id="${w.id}">
                <div class="fp-wheel-name">${w.name}</div>
                <div class="fp-wheel-meta">${w.location}</div>
              </div>
            `).join('') || '<div class="fp-empty">请先加载筒车列表</div>'}
          </div>
        </div>
        <div class="fp-section fp-main" style="flex:2">
          <h3 class="fp-title">⚖️ 古代智慧 vs 现代技术 能效对比</h3>
          <div class="fp-row">
            <label>对比时长：
              <select id="ecDays">
                <option value="30">30天</option>
                <option value="90">90天</option>
                <option value="365" selected>1年</option>
                <option value="1825">5年</option>
                <option value="3650">10年</option>
              </select>
            </label>
            <label>应用场景：
              <select id="ecScenario">
                <option value="standard">标准模式</option>
                <option value="flood">汛期高水位</option>
                <option value="drought">枯水期低水位</option>
                <option value="with_labor">计人工维护费</option>
              </select>
            </label>
            <button class="btn btn-primary" id="ecRunBtn" disabled>📊 执行对比</button>
          </div>
          <div id="ecBars" class="fp-bar-chart"></div>
          <div id="ecAdvantage" class="fp-adv-card"></div>
        </div>
        <div class="fp-section">
          <h3 class="fp-title">📜 历史对比</h3>
          <div id="ecList" class="fp-history">暂无记录</div>
        </div>
      </div>
    `;
    this.root.querySelectorAll('.fp-wheel-item').forEach(el => {
      el.addEventListener('click', () => {
        this.selectedID = parseInt(el.dataset.id);
        this.root.querySelectorAll('.fp-wheel-item').forEach(x => x.classList.toggle('active', parseInt(x.dataset.id) === this.selectedID));
        this.root.querySelector('#ecRunBtn').disabled = false;
      });
    });
    this.root.querySelector('#ecRunBtn').addEventListener('click', () => this._runCompare());
  }

  async _runCompare() {
    if (!this.selectedID) return;
    try {
      const days = parseInt(this.root.querySelector('#ecDays').value);
      const scenario = this.root.querySelector('#ecScenario').value;
      const res = await fetch(`${this.api}/waterwheels/${this.selectedID}/comparison/run?period_days=${days}&scenario=${scenario}`, { method: 'POST' });
      const c = await res.json();
      if (!res.ok) throw new Error(c.error);
      this._renderBars(c);
      this._renderAdvantage(c);
      this._loadHistory();
    } catch (e) {
      this.root.querySelector('#ecBars').innerHTML = `<div class="fp-empty" style="color:#C62828">❌ ${e.message}</div>`;
    }
  }

  _renderBars(c) {
    const w = c.waterwheel_metrics;
    const m = c.modern_pump_metrics;
    const maxCost = Math.max(w.total_cost_yuan, m.total_cost_yuan) * 1.1;
    const maxEnergy = Math.max(w.total_energy_kwh, m.total_energy_kwh) * 1.1;
    const maxCO2 = Math.max(w.co2_emission_kg, m.co2_emission_kg) * 1.1;

    const barRow = (label, wv, mv, unit, max, color1, color2) => `
      <div class="fp-bar-row">
        <div class="fp-bar-label">${label}</div>
        <div class="fp-bar-side">
          <div class="fp-bar-head">🏯 古代筒车 <span class="fp-bar-val">${wv.toFixed(1)} ${unit}</span></div>
          <div class="fp-bar-track"><div class="fp-bar-fill fp-bar-fill-${color1}" style="width:${(wv / max * 100).toFixed(1)}%"></div></div>
        </div>
        <div class="fp-bar-side">
          <div class="fp-bar-head">⚡ 现代水泵 <span class="fp-bar-val">${mv.toFixed(1)} ${unit}</span></div>
          <div class="fp-bar-track"><div class="fp-bar-fill fp-bar-fill-${color2}" style="width:${(mv / max * 100).toFixed(1)}%"></div></div>
        </div>
      </div>
    `;

    this.root.querySelector('#ecBars').innerHTML = `
      <div class="fp-metrics-grid">
        <div class="fp-metric-card ancient">
          <div class="fp-metric-title">🏯 古代筒车 (${c.period_days}天)</div>
          <div class="fp-metric-row"><span>总供水量</span><b>${w.total_water_m3.toFixed(0)} m³</b></div>
          <div class="fp-metric-row"><span>综合效率</span><b>${(w.avg_efficiency * 100).toFixed(1)}%</b></div>
          <div class="fp-metric-row"><span>能源类型</span><b>${w.energy_source}</b></div>
          <div class="fp-metric-row total"><span>总费用</span><b>¥ ${w.total_cost_yuan.toFixed(2)}</b></div>
        </div>
        <div class="fp-metric-card modern">
          <div class="fp-metric-title">⚡ 现代离心泵 (${c.period_days}天)</div>
          <div class="fp-metric-row"><span>总供水量</span><b>${m.total_water_m3.toFixed(0)} m³</b></div>
          <div class="fp-metric-row"><span>综合效率</span><b>${(m.avg_efficiency * 100).toFixed(1)}%</b></div>
          <div class="fp-metric-row"><span>能源类型</span><b>${m.energy_source}</b></div>
          <div class="fp-metric-row total"><span>总费用</span><b>¥ ${m.total_cost_yuan.toFixed(2)}</b></div>
        </div>
      </div>
      <h4 style="margin:20px 0 10px">📈 指标对比</h4>
      ${barRow('费用成本', w.total_cost_yuan, m.total_cost_yuan, '元', maxCost, 'green', 'orange')}
      ${barRow('能源消耗', w.total_energy_kwh, m.total_energy_kwh, 'kWh', maxEnergy, 'green', 'red')}
      ${barRow('CO₂排放', w.co2_emission_kg, m.co2_emission_kg, 'kg', maxCO2, 'green', 'red')}
    `;
  }

  _renderAdvantage(c) {
    const a = c.ancient_advantage;
    this.root.querySelector('#ecAdvantage').innerHTML = `
      <h4 style="margin:24px 0 12px">🌿 古代筒车的节能优势 (按全年换算)</h4>
      <div class="fp-adv-grid">
        <div class="fp-adv-item green">
          <div class="fp-adv-num">¥ ${a.cost_saved_yuan_per_year?.toFixed?.(0) ?? a.cost_saved_yuan.toFixed(0)}</div>
          <div class="fp-adv-k">每年节省费用</div>
        </div>
        <div class="fp-adv-item blue">
          <div class="fp-adv-num">${a.energy_saved_kwh_per_year?.toFixed?.(0) ?? a.energy_saved_kwh.toFixed(0)} kWh</div>
          <div class="fp-adv-k">每年节省电量</div>
        </div>
        <div class="fp-adv-item green">
          <div class="fp-adv-num">${a.co2_saved_kg_per_year?.toFixed?.(0) ?? a.co2_saved_kg.toFixed(0)} kg</div>
          <div class="fp-adv-k">每年减少碳排放</div>
        </div>
        <div class="fp-adv-item orange">
          <div class="fp-adv-num">${a.waterwheel_payback_years?.toFixed?.(1) ?? a.payback_years.toFixed(1)} 年</div>
          <div class="fp-adv-k">建造投资回收期</div>
        </div>
        <div class="fp-adv-item purple">
          <div class="fp-adv-num">${((a.cost_ratio_ancient_vs_modern ?? a.cost_ratio) * 100).toFixed(0)}%</div>
          <div class="fp-adv-k">古/今 成本比</div>
        </div>
        <div class="fp-adv-item teal">
          <div class="fp-adv-num">${(a.break_even_water_m3 ?? a.break_even_m3).toFixed(0)} m³</div>
          <div class="fp-adv-k">累计提水量回本线</div>
        </div>
      </div>
    `;
  }

  async _loadHistory() {
    if (!this.selectedID) return;
    try {
      const res = await fetch(`${this.api}/waterwheels/${this.selectedID}/comparisons?limit=10`);
      const list = await res.json();
      const box = this.root.querySelector('#ecList');
      if (!list || list.length === 0) { box.innerHTML = '暂无记录'; return; }
      box.innerHTML = list.map(c => `
        <div class="fp-hist-item">
          <div><b>${new Date(c.time).toLocaleDateString('zh-CN')}</b> · ${c.period_days}天</div>
          <div class="fp-hist-meta">
            省¥${(c.ancient_advantage.cost_saved_yuan_per_year ?? c.ancient_advantage.cost_saved_yuan).toFixed(0)}
            · CO₂↓${(c.ancient_advantage.co2_saved_kg_per_year ?? c.ancient_advantage.co2_saved_kg).toFixed(0)}kg
          </div>
        </div>
      `).join('');
    } catch (e) {}
  }
}

window.IrrigationSchedulerPanel = IrrigationSchedulerPanel;
window.WaterLevelForecastPanel = WaterLevelForecastPanel;
window.EfficiencyComparePanel = EfficiencyComparePanel;
