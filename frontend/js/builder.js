/**
 * VirtualBuilder - 公众虚拟建造筒车
 * 功能：拖拽式搭建 + 实时模拟 + 蓝图导出 + 作品保存
 */
class VirtualBuilder {
  constructor(rootEl, options = {}) {
    this.root = typeof rootEl === 'string' ? document.querySelector(rootEl) : rootEl;
    this.apiBase = options.apiBase || window.AppConfig.apiBase || '/api';
    this.config = Object.assign({}, window.AppConfig.build || VirtualBuilder.DEFAULT_CONFIG, options.config || {});

    this.state = {
      diameter: 6.0,
      bucketCount: 16,
      bucketCapacity: 0.06,
      spokeCount: 10,
      material: '杉木',
      installHeight: 2.7,
      wheelAngle: 0,
      flowVelocity: 1.5,
      waterDrop: 2.0,
      rotation: 0,
      running: true,
      simResult: null,
      parts: [],
      dragging: null,
    };

    this.presets = options.presets || [];
    this.buildName = options.buildName || '我的创意筒车';
    this._build();
    this._bind();
    this._startAnimation();
    this.runSimulation(true);
  }

  static get DEFAULT_CONFIG() {
    return {
      canvas: { width: 760, height: 520 },
      diameter: { min: 2.0, max: 15.0, step: 0.1 },
      buckets: { min: 6, max: 48, step: 1 },
      spokes: { min: 4, max: 24, step: 2 },
      capacity: { min: 0.01, max: 0.5, step: 0.005 },
      height: { min: 0, max: 8.0, step: 0.05 },
      flow: { min: 0.2, max: 5.0, step: 0.1 },
      drop: { min: 0.5, max: 8.0, step: 0.1 },
      materials: [
        { name: '楠木', density: 0.61, color: '#6B4423' },
        { name: '杉木', density: 0.38, color: '#8B7355' },
        { name: '柏木', density: 0.58, color: '#5C4033' },
        { name: '松木', density: 0.45, color: '#A0826D' },
        { name: '竹制', density: 0.65, color: '#7CB342' },
        { name: '铸铁', density: 7.20, color: '#424242' },
      ],
    };
  }

  _build() {
    const c = this.config;
    this.root.innerHTML = `
      <div class="vb-layout">
        <div class="vb-canvas-col">
          <div class="vb-canvas-wrap">
            <canvas class="vb-canvas" width="${c.canvas.width}" height="${c.canvas.height}"></canvas>
            <div class="vb-canvas-hint">💡 可拖动左右侧零件拖放到画布上</div>
          </div>
          <div class="vb-sim-card" id="vbSimCard">
            <div class="vb-sim-title">实时模拟结果</div>
            <div class="vb-sim-grid" id="vbSimGrid"></div>
            <div class="vb-warn" id="vbWarn" style="display:none"></div>
          </div>
        </div>
        <div class="vb-panel-col">
          <div class="vb-toolbar">
            <label>建造名称：<input type="text" id="vbBuildName" value="${this.buildName}" /></label>
            <select id="vbPresetSelect">
              <option value="">-- 加载经典模板 --</option>
              ${this.presets.map(p => `<option value="${p.id}">${p.name} (${p.culture})</option>`).join('')}
            </select>
          </div>
          <div class="vb-sliders" id="vbSliders"></div>
          <div class="vb-actions">
            <button class="btn btn-primary" id="vbSimBtn">🔄 重新模拟</button>
            <button class="btn btn-success" id="vbSaveBtn">💾 保存作品</button>
            <button class="btn btn-info" id="vbBpBtn">🖨 生成蓝图</button>
            <button class="btn btn-ghost" id="vbResetBtn">↺ 重置</button>
          </div>
          <div class="vb-parts-palette">
            <h4>零件库（拖拽到画布）</h4>
            <div class="vb-parts-grid">
              ${this._renderPartsPalette()}
            </div>
          </div>
        </div>
      </div>
    `;
    this.canvas = this.root.querySelector('.vb-canvas');
    this.ctx = this.canvas.getContext('2d');
    this._renderSliders();
  }

  _renderPartsPalette() {
    const parts = [
      { type: 'frame', name: '支架基座', icon: '▲' },
      { type: 'axle', name: '主轴', icon: '━' },
      { type: 'spoke', name: '加固辐条', icon: '│' },
      { type: 'bucket', name: '加大水斗', icon: '▢' },
      { type: 'waterwheel_big', name: '巨轮外框', icon: '◯' },
      { type: 'chain', name: '传动链', icon: '⛓' },
    ];
    return parts.map(p => `
      <div class="vb-part-item" draggable="true" data-type="${p.type}" data-name="${p.name}">
        <div class="vb-part-icon">${p.icon}</div>
        <div class="vb-part-name">${p.name}</div>
      </div>
    `).join('');
  }

  _renderSliders() {
    const c = this.config;
    const specs = [
      { key: 'diameter', label: '筒车直径', unit: 'm', cfg: c.diameter },
      { key: 'bucketCount', label: '水斗数量', unit: '个', cfg: c.buckets, isInt: true },
      { key: 'bucketCapacity', label: '单斗容量', unit: 'm³', cfg: c.capacity },
      { key: 'spokeCount', label: '辐条数量', unit: '根', cfg: c.spokes, isInt: true },
      { key: 'installHeight', label: '安装高度', unit: 'm', cfg: c.height },
      { key: 'flowVelocity', label: '水流速度', unit: 'm/s', cfg: c.flow },
      { key: 'waterDrop', label: '水位落差', unit: 'm', cfg: c.drop },
    ];
    const html = specs.map(s => `
      <div class="vb-slider-row">
        <label class="vb-slider-label">${s.label}：<span id="vbVal_${s.key}">${this.state[s.key]}</span> ${s.unit}</label>
        <input type="range" id="vbRange_${s.key}" min="${s.cfg.min}" max="${s.cfg.max}" step="${s.cfg.step}" value="${this.state[s.key]}" />
      </div>
    `).join('') + `
      <div class="vb-slider-row">
        <label class="vb-slider-label">建造材质：</label>
        <select id="vbMaterial">
          ${c.materials.map(m => `<option value="${m.name}" ${m.name === this.state.material ? 'selected' : ''}>${m.name} (ρ=${m.density})</option>`).join('')}
        </select>
      </div>
    `;
    this.root.querySelector('#vbSliders').innerHTML = html;
  }

  _bind() {
    const s = this.state;
    const inputs = ['diameter', 'bucketCount', 'bucketCapacity', 'spokeCount', 'installHeight', 'flowVelocity', 'waterDrop'];
    inputs.forEach(k => {
      const el = this.root.querySelector(`#vbRange_${k}`);
      const val = this.root.querySelector(`#vbVal_${k}`);
      if (!el) return;
      el.addEventListener('input', () => {
        let v = parseFloat(el.value);
        if (k === 'bucketCount' || k === 'spokeCount') v = parseInt(el.value);
        s[k] = v;
        val.textContent = v;
        this._debouncedSim();
      });
    });
    const mat = this.root.querySelector('#vbMaterial');
    if (mat) mat.addEventListener('change', () => { s.material = mat.value; this._debouncedSim(); });

    this.root.querySelector('#vbSimBtn').addEventListener('click', () => this.runSimulation(true));
    this.root.querySelector('#vbSaveBtn').addEventListener('click', () => this._onSave());
    this.root.querySelector('#vbBpBtn').addEventListener('click', () => this._onBlueprint());
    this.root.querySelector('#vbResetBtn').addEventListener('click', () => this._onReset());

    const presetEl = this.root.querySelector('#vbPresetSelect');
    if (presetEl) {
      presetEl.addEventListener('change', () => this._applyPreset(presetEl.value));
    }

    const palette = this.root.querySelectorAll('.vb-part-item');
    palette.forEach(el => {
      el.addEventListener('dragstart', (e) => {
        e.dataTransfer.setData('text/plain', JSON.stringify({
          type: el.dataset.type, name: el.dataset.name
        }));
      });
    });
    this.canvas.addEventListener('dragover', e => e.preventDefault());
    this.canvas.addEventListener('drop', (e) => {
      e.preventDefault();
      const data = JSON.parse(e.dataTransfer.getData('text/plain') || '{}');
      const rect = this.canvas.getBoundingClientRect();
      this._addPart(data, e.clientX - rect.left, e.clientY - rect.top);
    });
  }

  _addPart(data, x, y) {
    this.state.parts.push({
      part_type: data.type,
      name: data.name,
      quantity: 1,
      material: this.state.material,
      size_1: 1.0,
      pos_x: x,
      pos_y: y,
      rotation_deg: 0,
    });
    this.runSimulation(true);
  }

  _applyPreset(id) {
    if (!id) return;
    const p = this.presets.find(x => x.id === id);
    if (!p || !p.params) return;
    const pm = p.params;
    Object.assign(this.state, {
      diameter: pm.diameter_m || pm.diameter,
      bucketCount: pm.bucket_count,
      bucketCapacity: pm.bucket_capacity_m3 || pm.bucket_capacity,
      spokeCount: pm.spoke_count,
      material: pm.material,
      installHeight: pm.install_height_m || pm.install_height || 2.7,
    });
    if (pm.build_name) this.root.querySelector('#vbBuildName').value = pm.build_name;
    this._renderSliders();
    this._bind();
    this.runSimulation(true);
  }

  _debouncedSim() {
    clearTimeout(this._simTimer);
    this._simTimer = setTimeout(() => this.runSimulation(false), 200);
  }

  async runSimulation(force = false) {
    const s = this.state;
    try {
      const res = await fetch(`${this.apiBase}/virtual-build/simulate?flow_velocity=${s.flowVelocity}&water_drop=${s.waterDrop}`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          build_name: this.root.querySelector('#vbBuildName')?.value || '未命名',
          diameter_m: s.diameter,
          bucket_count: s.bucketCount,
          bucket_capacity_m3: s.bucketCapacity,
          spoke_count: s.spokeCount,
          material: s.material,
          install_height_m: s.installHeight,
          wheel_angle_deg: s.wheelAngle,
          parts_used: s.parts,
          user_id: 'visitor_' + (localStorage.getItem('vb_user') || 'guest'),
        })
      });
      const data = await res.json();
      if (!res.ok) throw new Error(data.error || '模拟失败');
      this._updateSimUI(data.simulation, data.build_params);
    } catch (e) {
      this._updateSimUI({
        rpm: 0, lift_rate_m3h: 0, bucket_fill_efficiency: 0,
        stress_level: 0, stable: false, submerged_buckets: 0,
        warning: e.message, torque_nm: 0,
        flow_velocity_mps: s.flowVelocity, water_drop_m: s.waterDrop,
      });
    }
  }

  _updateSimUI(sim, params) {
    const s = this.state;
    s.simResult = sim;
    if (params) {
      s.predictedLift = params.predicted_lift_m3h;
      s.predictedEff = params.predicted_efficiency;
    }
    const grid = this.root.querySelector('#vbSimGrid');
    const items = [
      ['转速', `${sim.rpm?.toFixed?.(1) ?? sim.Rpm?.toFixed(1) ?? 0} rpm`, '#1565C0'],
      ['提水流量', `${(sim.lift_rate_m3h ?? sim.LiftRate ?? 0).toFixed(1)} m³/h`, '#2E7D32'],
      ['充水效率', `${((sim.bucket_fill_efficiency ?? sim.BucketFillEff ?? 0) * 100).toFixed(0)}%`, '#F57F17'],
      ['结构应力', `${((sim.stress_level ?? sim.StressLevel ?? 0) * 100).toFixed(0)}%`,
        (sim.stress_level ?? sim.StressLevel ?? 0) > 0.75 ? '#C62828' : '#558B2F'],
      ['浸没斗数', `${sim.submerged_buckets ?? sim.SubmergedBuckets ?? 0} 个`, '#00838F'],
      ['净转矩', `${(sim.torque_nm ?? sim.Torque ?? 0).toFixed(0)} N·m`, '#6A1B9A'],
    ];
    grid.innerHTML = items.map(([k, v, color]) => `
      <div class="vb-sim-item">
        <div class="vb-sim-k">${k}</div>
        <div class="vb-sim-v" style="color:${color}">${v}</div>
      </div>
    `).join('');

    const warn = this.root.querySelector('#vbWarn');
    const msg = sim.warning || sim.Warning;
    if (msg) {
      warn.style.display = 'block';
      warn.textContent = (sim.stable ?? sim.Stable) ? '⚠️ ' + msg : '❌ 结构异常：' + msg;
      warn.className = 'vb-warn ' + ((sim.stable ?? sim.Stable) ? '' : 'is-danger');
    } else {
      warn.style.display = 'none';
    }
  }

  async _onSave() {
    const s = this.state;
    const userId = localStorage.getItem('vb_user') || 'guest_' + Math.random().toString(36).slice(2, 8);
    localStorage.setItem('vb_user', userId);
    const build = {
      user_id: userId,
      build_name: this.root.querySelector('#vbBuildName')?.value || '未命名筒车',
      diameter_m: s.diameter,
      bucket_count: s.bucketCount,
      bucket_capacity_m3: s.bucketCapacity,
      spoke_count: s.spokeCount,
      material: s.material,
      install_height_m: s.installHeight,
      wheel_angle_deg: s.wheelAngle,
      parts_used: s.parts,
      predicted_lift_m3h: s.simResult?.lift_rate_m3h ?? s.simResult?.LiftRate ?? 0,
      predicted_efficiency: (s.simResult?.bucket_fill_efficiency ?? s.simResult?.BucketFillEff ?? 0) * (s.simResult?.stress_level ?? s.simResult?.StressLevel ?? 0.5),
      is_public: true,
    };
    try {
      const res = await fetch(`${this.apiBase}/virtual-build/save`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ build, flow_velocity: s.flowVelocity, water_drop: s.waterDrop, generate_blueprint: true })
      });
      const data = await res.json();
      if (!res.ok) throw new Error(data.error || '保存失败');
      alert(`✅ 作品保存成功！ID: ${data.id}`);
      if (this.onSaveCb) this.onSaveCb(data);
    } catch (e) {
      alert('❌ ' + e.message);
    }
  }

  async _onBlueprint() {
    const s = this.state;
    const build = {
      build_name: this.root.querySelector('#vbBuildName')?.value || '未命名筒车',
      diameter_m: s.diameter,
      bucket_count: s.bucketCount,
      bucket_capacity_m3: s.bucketCapacity,
      spoke_count: s.spokeCount,
      material: s.material,
      install_height_m: s.installHeight,
      predicted_lift_m3h: s.simResult?.lift_rate_m3h ?? s.simResult?.LiftRate ?? 0,
      predicted_efficiency: (s.simResult?.bucket_fill_efficiency ?? s.simResult?.BucketFillEff ?? 0),
    };
    try {
      const res = await fetch(`${this.apiBase}/virtual-build/blueprint`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(build)
      });
      const svg = await res.text();
      const w = window.open('', 'blueprint', 'width=900,height=600');
      w.document.write(svg);
      w.document.title = '筒车蓝图 - ' + build.build_name;
      w.document.close();
    } catch (e) {
      alert('❌ 蓝图生成失败：' + e.message);
    }
  }

  _onReset() {
    Object.assign(this.state, {
      diameter: 6.0, bucketCount: 16, bucketCapacity: 0.06,
      spokeCount: 10, material: '杉木', installHeight: 2.7,
      flowVelocity: 1.5, waterDrop: 2.0, parts: [],
    });
    this.root.querySelector('#vbBuildName').value = '我的创意筒车';
    this._renderSliders();
    this._bind();
    this.runSimulation(true);
  }

  _startAnimation() {
    const render = () => {
      this._draw();
      if (this.state.running) {
        const rpm = this.state.simResult?.rpm ?? this.state.simResult?.Rpm ?? 5;
        this.state.rotation += (rpm / 60) * Math.PI * 2 / 60;
      }
      requestAnimationFrame(render);
    };
    requestAnimationFrame(render);
  }

  _draw() {
    const ctx = this.ctx;
    const W = this.canvas.width, H = this.canvas.height;
    const cx = W * 0.42, cy = H * 0.5;
    const s = this.state;
    const c = this.config;

    ctx.clearRect(0, 0, W, H);

    const bgGrad = ctx.createLinearGradient(0, 0, 0, H);
    bgGrad.addColorStop(0, '#0B1D3A');
    bgGrad.addColorStop(1, '#1A3959');
    ctx.fillStyle = bgGrad;
    ctx.fillRect(0, 0, W, H);

    for (let i = 0; i < 30; i++) {
      ctx.fillStyle = `rgba(255,255,255,${0.3 + Math.random() * 0.5})`;
      ctx.fillRect(Math.sin(i * 13 + Date.now() / 5000) * W / 2 + W / 2, (i * 17) % H, 1.5, 1.5);
    }

    const matObj = c.materials.find(m => m.name === s.material) || c.materials[1];
    const matColor = matObj.color;

    const maxDia = c.diameter.max;
    const pxPerM = Math.min(cx * 1.6 / maxDia, (cy - 60) / maxDia);
    const R = s.diameter * pxPerM / 2;
    const rInner = R * 0.55;
    const waterY = cy + R * 0.78 - s.installHeight * pxPerM;

    const wGrad = ctx.createLinearGradient(0, waterY, 0, H);
    wGrad.addColorStop(0, '#64B5F6');
    wGrad.addColorStop(1, '#0D47A1');
    ctx.fillStyle = wGrad;
    ctx.fillRect(0, waterY, W, H - waterY);

    ctx.strokeStyle = 'rgba(255,255,255,0.3)';
    ctx.lineWidth = 1;
    const t = Date.now() / 400;
    for (let x = 0; x < W; x += 18) {
      const yy = waterY + Math.sin(x / 28 + t) * 2.5;
      ctx.beginPath();
      ctx.moveTo(x, yy);
      ctx.lineTo(x + 12, yy + Math.sin(x / 28 + t + 1.1) * 2.5);
      ctx.stroke();
    }

    ctx.strokeStyle = '#5D4037';
    ctx.lineWidth = 10;
    ctx.lineCap = 'round';
    ctx.beginPath(); ctx.moveTo(cx - R * 0.95, cy + R * 0.95); ctx.lineTo(cx, cy); ctx.stroke();
    ctx.beginPath(); ctx.moveTo(cx + R * 0.95, cy + R * 0.95); ctx.lineTo(cx, cy); ctx.stroke();

    const parts = s.parts.filter(p => p.part_type === 'frame');
    parts.forEach(p => {
      ctx.fillStyle = 'rgba(93, 64, 55, 0.6)';
      ctx.fillRect(p.pos_x - 15, p.pos_y - 30, 30, 60);
    });

    ctx.save();
    ctx.translate(cx, cy);
    ctx.rotate(this.state.rotation);

    ctx.strokeStyle = matColor;
    ctx.lineWidth = 7;
    ctx.beginPath(); ctx.arc(0, 0, R, 0, Math.PI * 2); ctx.stroke();
    ctx.lineWidth = 4; ctx.globalAlpha = 0.75;
    ctx.beginPath(); ctx.arc(0, 0, rInner, 0, Math.PI * 2); ctx.stroke();
    ctx.globalAlpha = 1;
    ctx.lineWidth = 3;
    for (let i = 0; i < s.spokeCount; i++) {
      const ang = i * 2 * Math.PI / s.spokeCount;
      ctx.beginPath();
      ctx.moveTo(Math.cos(ang) * rInner, Math.sin(ang) * rInner);
      ctx.lineTo(Math.cos(ang) * R, Math.sin(ang) * R);
      ctx.stroke();
    }
    ctx.fillStyle = matColor;
    ctx.beginPath(); ctx.arc(0, 0, 14, 0, Math.PI * 2); ctx.fill();
    ctx.strokeStyle = '#3E2723'; ctx.lineWidth = 3; ctx.stroke();

    const bAng = 2 * Math.PI / s.bucketCount;
    const bw = Math.max(10, R * 0.14);
    const bh = bw * 1.4;
    for (let i = 0; i < s.bucketCount; i++) {
      const ang = i * bAng + bAng / 2;
      const bx = Math.cos(ang) * R;
      const by = Math.sin(ang) * R;
      const deg = ang * 180 / Math.PI;
      ctx.save();
      ctx.translate(bx, by);
      ctx.rotate(ang);
      ctx.fillStyle = matColor;
      ctx.strokeStyle = '#3E2723';
      ctx.lineWidth = 2;
      ctx.beginPath();
      ctx.moveTo(-bw / 2, -bh / 2);
      ctx.quadraticCurveTo(0, -bh / 2 - 4, bw / 2, -bh / 2);
      ctx.lineTo(bw / 2 - 3, bh / 2);
      ctx.quadraticCurveTo(0, bh / 2 + 5, -bw / 2 + 3, bh / 2);
      ctx.closePath();
      ctx.fill(); ctx.stroke();
      const waterLevel = 0.2 + Math.min(0.75, ((s.simResult?.bucket_fill_efficiency ?? s.simResult?.BucketFillEff ?? 0.4)) * (1 - i / s.bucketCount * 0.6));
      if (by > -R * 0.1) {
        ctx.fillStyle = 'rgba(33,150,243,0.7)';
        ctx.fillRect(-bw / 2 + 3, -bh / 2 + 3 + bh * (1 - waterLevel), bw - 6, bh * waterLevel - 5);
      }
      ctx.restore();
    }
    ctx.restore();

    const dropBarX = W - 68;
    ctx.fillStyle = 'rgba(0,0,0,0.3)';
    ctx.fillRect(dropBarX - 4, cy - R * 0.9, 52, R * 1.8 + 25);
    const dropH = s.waterDrop * pxPerM;
    const dGrad = ctx.createLinearGradient(0, waterY, 0, waterY - dropH);
    dGrad.addColorStop(0, '#0D47A1');
    dGrad.addColorStop(1, '#4FC3F7');
    ctx.fillStyle = dGrad;
    ctx.fillRect(dropBarX, waterY, 44, -dropH);
    ctx.strokeStyle = '#FFD54F';
    ctx.lineWidth = 2;
    ctx.setLineDash([4, 3]);
    ctx.beginPath(); ctx.moveTo(dropBarX, waterY); ctx.lineTo(dropBarX + 44, waterY); ctx.stroke();
    ctx.beginPath(); ctx.moveTo(dropBarX, waterY - dropH); ctx.lineTo(dropBarX + 44, waterY - dropH); ctx.stroke();
    ctx.setLineDash([]);
    ctx.fillStyle = '#FFF';
    ctx.font = 'bold 12px Microsoft YaHei';
    ctx.textAlign = 'center';
    ctx.fillText(`${s.waterDrop.toFixed(1)}m`, dropBarX + 22, waterY - dropH / 2 + 4);
    ctx.fillStyle = 'rgba(255,255,255,0.7)';
    ctx.font = '11px Microsoft YaHei';
    ctx.fillText('水位差', dropBarX + 22, waterY + 16);

    ctx.fillStyle = 'rgba(0,0,0,0.45)';
    ctx.fillRect(10, 10, 230, 80);
    ctx.strokeStyle = 'rgba(255,255,255,0.3)'; ctx.strokeRect(10, 10, 230, 80);
    ctx.fillStyle = '#FFF';
    ctx.textAlign = 'left';
    ctx.font = 'bold 14px Microsoft YaHei';
    ctx.fillText(`📐 建造参数`, 20, 30);
    ctx.font = '12px Microsoft YaHei';
    ctx.fillStyle = '#FFE082';
    ctx.fillText(`直径 ${s.diameter.toFixed(1)}m  |  斗数 ${s.bucketCount}  |  ${s.material}`, 20, 50);
    ctx.fillStyle = '#B3E5FC';
    ctx.fillText(`水流 ${s.flowVelocity.toFixed(1)}m/s  |  安装高 ${s.installHeight.toFixed(2)}m`, 20, 70);
    ctx.fillStyle = '#A5D6A7';
    const lift = s.simResult?.lift_rate_m3h ?? s.simResult?.LiftRate ?? 0;
    ctx.fillText(`预估提水：${lift.toFixed(1)} m³/h`, 20, 88);

    s.parts.forEach((p, idx) => {
      if (p.part_type === 'frame') return;
      const icons = { spoke: '│', bucket: '▢', waterwheel_big: '◯', axle: '━', chain: '⛓' };
      ctx.fillStyle = 'rgba(255,193,7,0.85)';
      ctx.font = 'bold 16px Microsoft YaHei';
      ctx.fillText(icons[p.part_type] || '+', p.pos_x - 8, p.pos_y - 12);
      ctx.fillStyle = 'rgba(255,255,255,0.6)';
      ctx.font = '10px Microsoft YaHei';
      ctx.fillText(p.name, p.pos_x - 16, p.pos_y);
    });
  }

  destroy() {
    this.state.running = false;
    this.root.innerHTML = '';
  }
}

window.VirtualBuilder = VirtualBuilder;
