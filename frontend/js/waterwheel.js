class WaterwheelRenderer {
    constructor(canvasId) {
        this.canvas = document.getElementById(canvasId);
        this.ctx = this.canvas.getContext('2d');
        this.waterwheel = null;
        this.telemetry = null;
        this.angle = 0;
        this.running = true;
        this.speedMultiplier = 1;
        this.particleData = new Float32Array(0);
        this.particleCount = 0;
        this.dropData = new Float32Array(0);
        this.dropCount = 0;
        this.lastTime = 0;
        this.onClick = null;
        this.hoveredBucket = -1;
        this.useWorker = typeof Worker !== 'undefined';
        this.particleWorker = null;
        this._workerReady = false;

        this.resize();
        window.addEventListener('resize', () => this.resize());
        this.canvas.addEventListener('click', (e) => this.handleClick(e));
        this.canvas.addEventListener('mousemove', (e) => this.handleMouseMove(e));

        this.initParticles();
        this.animate(performance.now());
    }

    resize() {
        const rect = this.canvas.getBoundingClientRect();
        const dpr = window.devicePixelRatio || 1;
        this.canvas.width = rect.width * dpr;
        this.canvas.height = 500 * dpr;
        this.ctx.scale(dpr, dpr);
        this.viewWidth = rect.width;
        this.viewHeight = 500;

        if (this.particleWorker && this._workerReady) {
            this.particleWorker.postMessage({
                type: 'config',
                config: { width: this.viewWidth, height: this.viewHeight }
            });
        }
    }

    setWaterwheel(wheel) {
        this.waterwheel = wheel;
        this.hoveredBucket = -1;
    }

    setTelemetry(data) {
        this.telemetry = data;
    }

    setRunning(running) {
        this.running = running;
    }

    setSpeed(mult) {
        this.speedMultiplier = mult;
    }

    initParticles() {
        if (this.useWorker) {
            try {
                this.particleWorker = new Worker('js/particle-worker.js');
                this.particleWorker.onmessage = (e) => this._onWorkerMessage(e);
                this.particleWorker.postMessage({
                    type: 'init',
                    config: {
                        count: 120,
                        width: this.viewWidth,
                        height: this.viewHeight,
                        waterY: 0.75,
                        waterHeight: 0.22,
                        flowSpeed: 1.0,
                    }
                });
                this._workerReady = true;
            } catch (e) {
                console.warn('Web Worker 启动失败，回退到主线程模式', e);
                this.useWorker = false;
                this._initFallbackParticles();
            }
        } else {
            this._initFallbackParticles();
        }
    }

    _initFallbackParticles() {
        this._fallbackParticles = [];
        this._fallbackDrops = [];
        for (let i = 0; i < 80; i++) {
            this._fallbackParticles.push(this._createParticle());
        }
    }

    _createParticle() {
        return {
            x: Math.random() * this.viewWidth,
            y: this.viewHeight * 0.72 + Math.random() * (this.viewHeight * 0.25),
            vx: 1 + Math.random() * 3,
            vy: (Math.random() - 0.5) * 0.5,
            size: 1 + Math.random() * 3,
            alpha: 0.3 + Math.random() * 0.5,
        };
    }

    _onWorkerMessage(e) {
        const msg = e.data;
        if (msg.type === 'particles') {
            this.particleData = new Float32Array(msg.particles);
            this.particleCount = msg.particleCount;
            this.dropData = new Float32Array(msg.drops);
            this.dropCount = msg.dropCount;
        }
    }

    handleClick(e) {
        if (!this.onClick || !this.waterwheel) return;
        const rect = this.canvas.getBoundingClientRect();
        const x = e.clientX - rect.left;
        const y = e.clientY - rect.top;
        this.onClick({ x, y });
    }

    handleMouseMove(e) {
        if (!this.waterwheel) return;
        const rect = this.canvas.getBoundingClientRect();
        const x = e.clientX - rect.left;
        const y = e.clientY - rect.top;

        const cx = this.viewWidth / 2;
        const cy = this.viewHeight * 0.45;
        const wheelRadius = Math.min(this.viewWidth * 0.32, this.viewHeight * 0.38);

        this.hoveredBucket = -1;
        const bucketCount = this.waterwheel.bucket_count || 20;
        for (let i = 0; i < bucketCount; i++) {
            const bucketAngle = this.angle + (i / bucketCount) * Math.PI * 2;
            const bx = cx + Math.cos(bucketAngle) * wheelRadius * 0.92;
            const by = cy + Math.sin(bucketAngle) * wheelRadius * 0.92;
            const dist = Math.sqrt((x - bx) ** 2 + (y - by) ** 2);
            if (dist < 25) {
                this.hoveredBucket = i;
                this.canvas.style.cursor = 'pointer';
                return;
            }
        }

        const centerDist = Math.sqrt((x - cx) ** 2 + (y - cy) ** 2);
        if (centerDist < wheelRadius) {
            this.canvas.style.cursor = 'pointer';
        } else {
            this.canvas.style.cursor = 'default';
        }
    }

    animate(currentTime) {
        const deltaTime = Math.min((currentTime - this.lastTime) / 1000, 0.1);
        this.lastTime = currentTime;

        if (this.running) {
            const rpm = (this.telemetry && this.telemetry.rotation_speed) || 3;
            this.angle += (rpm * 2 * Math.PI / 60) * deltaTime * this.speedMultiplier;
        }

        this._updateParticles(deltaTime);
        this.draw();

        requestAnimationFrame((t) => this.animate(t));
    }

    _updateParticles(dt) {
        if (this.useWorker && this._workerReady) {
            this.particleWorker.postMessage({ type: 'update', dt: dt });
        } else {
            this._updateFallbackParticles(dt);
        }
    }

    _updateFallbackParticles(dt) {
        const parts = this._fallbackParticles;
        const waterY = this.viewHeight * 0.75;
        for (let i = 0; i < parts.length; i++) {
            const p = parts[i];
            p.x += p.vx * dt * 60;
            p.y += p.vy * dt * 60;

            if (p.x > this.viewWidth + 10) {
                p.x = -10;
                p.y = waterY + Math.random() * (this.viewHeight * 0.22);
            }
            if (p.y < waterY - 5) {
                p.y = waterY + 2;
            }
            if (p.y > this.viewHeight - 5) {
                p.y = this.viewHeight - 10;
            }
        }

        const drops = this._fallbackDrops;
        for (let i = drops.length - 1; i >= 0; i--) {
            const d = drops[i];
            d.x += d.vx * dt * 60;
            d.y += d.vy * dt * 60;
            d.vy += 0.15 * dt * 60;
            d.life -= dt;
            if (d.life <= 0 || d.y > this.viewHeight) {
                drops.splice(i, 1);
            }
        }
    }

    draw() {
        this.ctx.clearRect(0, 0, this.viewWidth, this.viewHeight);

        this.drawBackground();
        this.drawWater();
        this.drawWaterParticles();
        this.drawFlowIndicators();

        if (this.waterwheel) {
            this.drawWaterwheel();
            this.drawWaterDrops();
        } else {
            this.drawPlaceholder();
        }

        this.drawGround();
    }

    drawBackground() {
        const grad = this.ctx.createLinearGradient(0, 0, 0, this.viewHeight);
        grad.addColorStop(0, '#0d1b2a');
        grad.addColorStop(0.6, '#1b2838');
        grad.addColorStop(1, '#1a3a4a');
        this.ctx.fillStyle = grad;
        this.ctx.fillRect(0, 0, this.viewWidth, this.viewHeight);

        this.ctx.fillStyle = 'rgba(255, 255, 255, 0.3)';
        for (let i = 0; i < 30; i++) {
            const x = (i * 137.5) % this.viewWidth;
            const y = (i * 73.3) % (this.viewHeight * 0.55);
            const r = (i % 3) * 0.5 + 0.5;
            this.ctx.beginPath();
            this.ctx.arc(x, y, r, 0, Math.PI * 2);
            this.ctx.fill();
        }

        this.ctx.fillStyle = 'rgba(30, 60, 80, 0.5)';
        this.ctx.beginPath();
        this.ctx.moveTo(0, this.viewHeight * 0.65);
        for (let x = 0; x <= this.viewWidth; x += 50) {
            const y = this.viewHeight * 0.65 - Math.sin(x * 0.01) * 20 - Math.sin(x * 0.02 + 1) * 15;
            this.ctx.lineTo(x, y);
        }
        this.ctx.lineTo(this.viewWidth, this.viewHeight);
        this.ctx.lineTo(0, this.viewHeight);
        this.ctx.closePath();
        this.ctx.fill();
    }

    drawWater() {
        const waterY = this.viewHeight * 0.75;

        const waterGrad = this.ctx.createLinearGradient(0, waterY - 20, 0, this.viewHeight);
        waterGrad.addColorStop(0, 'rgba(30, 136, 229, 0.6)');
        waterGrad.addColorStop(0.3, 'rgba(21, 101, 192, 0.7)');
        waterGrad.addColorStop(1, 'rgba(13, 71, 161, 0.9)');

        this.ctx.fillStyle = waterGrad;
        this.ctx.beginPath();
        this.ctx.moveTo(0, waterY);

        const time = Date.now() * 0.002;
        for (let x = 0; x <= this.viewWidth; x += 10) {
            const wave = Math.sin(x * 0.02 + time) * 4 + Math.sin(x * 0.04 + time * 1.5) * 2;
            this.ctx.lineTo(x, waterY + wave);
        }
        this.ctx.lineTo(this.viewWidth, this.viewHeight);
        this.ctx.lineTo(0, this.viewHeight);
        this.ctx.closePath();
        this.ctx.fill();

        this.ctx.strokeStyle = 'rgba(129, 212, 250, 0.4)';
        this.ctx.lineWidth = 2;
        this.ctx.beginPath();
        for (let x = 0; x <= this.viewWidth; x += 10) {
            const wave = Math.sin(x * 0.02 + time) * 4 + Math.sin(x * 0.04 + time * 1.5) * 2;
            if (x === 0) this.ctx.moveTo(x, waterY + wave);
            else this.ctx.lineTo(x, waterY + wave);
        }
        this.ctx.stroke();
    }

    drawWaterParticles() {
        const ctx = this.ctx;
        if (this.useWorker) {
            const data = this.particleData;
            const count = this.particleCount;
            for (let i = 0; i < count; i++) {
                const o = i * 6;
                const x = data[o];
                const y = data[o + 1];
                const size = data[o + 4];
                const alpha = data[o + 5];
                ctx.fillStyle = `rgba(129, 212, 250, ${alpha})`;
                ctx.beginPath();
                ctx.arc(x, y, size, 0, Math.PI * 2);
                ctx.fill();
            }
        } else {
            const parts = this._fallbackParticles;
            for (let i = 0; i < parts.length; i++) {
                const p = parts[i];
                ctx.fillStyle = `rgba(129, 212, 250, ${p.alpha})`;
                ctx.beginPath();
                ctx.arc(p.x, p.y, p.size, 0, Math.PI * 2);
                ctx.fill();
            }
        }
    }

    drawFlowIndicators() {
        if (!this.telemetry) return;

        const flowSpeed = Math.min(this.telemetry.flow_velocity || 1, 5);
        const arrowCount = 5;
        const time = Date.now() * 0.001;

        this.ctx.strokeStyle = 'rgba(129, 212, 250, 0.6)';
        this.ctx.fillStyle = 'rgba(129, 212, 250, 0.6)';
        this.ctx.lineWidth = 2;

        for (let i = 0; i < arrowCount; i++) {
            const baseX = ((time * flowSpeed * 50 + i * (this.viewWidth / arrowCount)) % (this.viewWidth + 60)) - 30;
            const y = this.viewHeight * 0.78 + (i % 2) * 25;

            this.ctx.beginPath();
            this.ctx.moveTo(baseX, y);
            this.ctx.lineTo(baseX + 20, y);
            this.ctx.lineTo(baseX + 15, y - 5);
            this.ctx.moveTo(baseX + 20, y);
            this.ctx.lineTo(baseX + 15, y + 5);
            this.ctx.stroke();
        }
    }

    drawWaterwheel() {
        const cx = this.viewWidth / 2;
        const cy = this.viewHeight * 0.45;
        const wheelRadius = Math.min(this.viewWidth * 0.32, this.viewHeight * 0.38);
        const bucketCount = this.waterwheel.bucket_count || 20;
        const axleRadius = wheelRadius * 0.12;
        const spokeCount = 12;

        this.drawSupports(cx, cy, wheelRadius);

        this.ctx.save();
        this.ctx.translate(cx, cy);
        this.ctx.rotate(this.angle);

        this.drawOuterRing(wheelRadius);
        this.drawInnerRing(wheelRadius, axleRadius);
        this.drawSpokes(wheelRadius, axleRadius, spokeCount);
        this.drawBuckets(wheelRadius, bucketCount);

        this.ctx.restore();

        this.drawAxle(cx, cy, axleRadius);
        this.drawWaterChannel(cx, cy, wheelRadius);
    }

    drawSupports(cx, cy, r) {
        this.ctx.fillStyle = '#5d4037';
        this.ctx.strokeStyle = '#3e2723';
        this.ctx.lineWidth = 2;

        this.ctx.beginPath();
        this.ctx.moveTo(cx - r * 0.9, cy + r * 0.85);
        this.ctx.lineTo(cx - r * 0.3, cy + r * 0.1);
        this.ctx.lineTo(cx - r * 0.2, cy + r * 0.15);
        this.ctx.lineTo(cx - r * 0.75, cy + r * 0.95);
        this.ctx.closePath();
        this.ctx.fill();
        this.ctx.stroke();

        this.ctx.beginPath();
        this.ctx.moveTo(cx + r * 0.9, cy + r * 0.85);
        this.ctx.lineTo(cx + r * 0.3, cy + r * 0.1);
        this.ctx.lineTo(cx + r * 0.2, cy + r * 0.15);
        this.ctx.lineTo(cx + r * 0.75, cy + r * 0.95);
        this.ctx.closePath();
        this.ctx.fill();
        this.ctx.stroke();

        this.ctx.fillStyle = '#6d4c41';
        this.ctx.fillRect(cx - r * 0.1, cy - r * 0.05, r * 0.2, r * 0.15);
    }

    drawOuterRing(r) {
        this.ctx.strokeStyle = '#8d6e63';
        this.ctx.lineWidth = 10;
        this.ctx.beginPath();
        this.ctx.arc(0, 0, r, 0, Math.PI * 2);
        this.ctx.stroke();

        this.ctx.strokeStyle = '#6d4c41';
        this.ctx.lineWidth = 3;
        this.ctx.beginPath();
        this.ctx.arc(0, 0, r - 8, 0, Math.PI * 2);
        this.ctx.stroke();
    }

    drawInnerRing(r, axleR) {
        this.ctx.strokeStyle = '#795548';
        this.ctx.lineWidth = 5;
        this.ctx.beginPath();
        this.ctx.arc(0, 0, r * 0.4, 0, Math.PI * 2);
        this.ctx.stroke();
    }

    drawSpokes(r, axleR, count) {
        this.ctx.strokeStyle = '#6d4c41';
        this.ctx.lineWidth = 4;

        for (let i = 0; i < count; i++) {
            const a = (i / count) * Math.PI * 2;
            this.ctx.beginPath();
            this.ctx.moveTo(Math.cos(a) * axleR, Math.sin(a) * axleR);
            this.ctx.lineTo(Math.cos(a) * (r - 5), Math.sin(a) * (r - 5));
            this.ctx.stroke();
        }
    }

    drawBuckets(r, count) {
        for (let i = 0; i < count; i++) {
            const bucketAngle = (i / count) * Math.PI * 2;
            const bx = Math.cos(bucketAngle) * r * 0.92;
            const by = Math.sin(bucketAngle) * r * 0.92;

            this.ctx.save();
            this.ctx.translate(bx, by);
            this.ctx.rotate(bucketAngle + Math.PI / 2);

            const bucketWidth = r * 0.18;
            const bucketHeight = r * 0.14;
            const isHovered = (i === this.hoveredBucket);
            const isSubmerged = by > r * 0.3;

            this.drawBucket(bucketWidth, bucketHeight, isHovered, isSubmerged, bucketAngle);

            this.ctx.restore();

            if (by < -r * 0.5 && this.running && Math.random() < 0.15) {
                this.addWaterDrop(bx + this.viewWidth / 2, by + this.viewHeight * 0.45);
            }
        }
    }

    drawBucket(w, h, hovered, submerged, angle) {
        const grad = this.ctx.createLinearGradient(0, -h / 2, 0, h / 2);
        if (hovered) {
            grad.addColorStop(0, '#bcaaa4');
            grad.addColorStop(1, '#8d6e63');
        } else {
            grad.addColorStop(0, '#a1887f');
            grad.addColorStop(1, '#6d4c41');
        }

        this.ctx.fillStyle = grad;
        this.ctx.strokeStyle = hovered ? '#ffcc80' : '#4e342e';
        this.ctx.lineWidth = hovered ? 3 : 2;

        this.ctx.beginPath();
        this.ctx.moveTo(-w / 2, -h / 2);
        this.ctx.quadraticCurveTo(-w / 2 - 5, 0, -w / 2 + 3, h / 2);
        this.ctx.lineTo(w / 2 - 3, h / 2);
        this.ctx.quadraticCurveTo(w / 2 + 5, 0, w / 2, -h / 2);
        this.ctx.closePath();
        this.ctx.fill();
        this.ctx.stroke();

        this.ctx.strokeStyle = hovered ? '#ffab91' : '#5d4037';
        this.ctx.lineWidth = 1.5;
        this.ctx.beginPath();
        this.ctx.moveTo(-w / 2, -h / 2);
        this.ctx.lineTo(w / 2, -h / 2);
        this.ctx.stroke();

        if (submerged || angle > Math.PI * 0.3 && angle < Math.PI * 0.9) {
            const fillLevel = submerged ? 0.85 : 0.5 + Math.sin(angle) * 0.3;
            if (fillLevel > 0) {
                this.ctx.fillStyle = 'rgba(30, 136, 229, 0.75)';
                this.ctx.beginPath();
                const waterTop = h / 2 - fillLevel * h;
                this.ctx.moveTo(-w / 2 + 4, waterTop + 3);
                this.ctx.quadraticCurveTo(0, waterTop, w / 2 - 4, waterTop + 3);
                this.ctx.lineTo(w / 2 - 3, h / 2 - 2);
                this.ctx.quadraticCurveTo(0, h / 2 - 1, -w / 2 + 3, h / 2 - 2);
                this.ctx.closePath();
                this.ctx.fill();

                this.ctx.fillStyle = 'rgba(129, 212, 250, 0.6)';
                this.ctx.beginPath();
                this.ctx.ellipse(0, waterTop + 4, w / 3, 1.5, 0, 0, Math.PI * 2);
                this.ctx.fill();
            }
        }
    }

    drawAxle(cx, cy, r) {
        const grad = this.ctx.createRadialGradient(cx, cy, 0, cx, cy, r);
        grad.addColorStop(0, '#9e9e9e');
        grad.addColorStop(0.7, '#616161');
        grad.addColorStop(1, '#424242');

        this.ctx.fillStyle = grad;
        this.ctx.beginPath();
        this.ctx.arc(cx, cy, r, 0, Math.PI * 2);
        this.ctx.fill();

        this.ctx.fillStyle = '#424242';
        this.ctx.beginPath();
        this.ctx.arc(cx, cy, r * 0.4, 0, Math.PI * 2);
        this.ctx.fill();

        this.ctx.strokeStyle = `rgba(255, 200, 100, ${this.running ? 0.6 : 0.2})`;
        this.ctx.lineWidth = 2;
        for (let i = 0; i < 6; i++) {
            const a = this.angle * 3 + (i / 6) * Math.PI * 2;
            this.ctx.beginPath();
            this.ctx.moveTo(cx + Math.cos(a) * r * 0.45, cy + Math.sin(a) * r * 0.45);
            this.ctx.lineTo(cx + Math.cos(a) * r * 0.8, cy + Math.sin(a) * r * 0.8);
            this.ctx.stroke();
        }
    }

    drawWaterChannel(cx, cy, r) {
        this.ctx.fillStyle = 'rgba(30, 136, 229, 0.3)';
        this.ctx.strokeStyle = 'rgba(100, 181, 246, 0.5)';
        this.ctx.lineWidth = 2;

        this.ctx.beginPath();
        this.ctx.moveTo(cx + r * 0.3, cy - r * 0.2);
        this.ctx.lineTo(cx + r * 1.4, cy - r * 0.5);
        this.ctx.lineTo(cx + r * 1.4, cy - r * 0.3);
        this.ctx.lineTo(cx + r * 0.3, cy);
        this.ctx.closePath();
        this.ctx.fill();
        this.ctx.stroke();
    }

    addWaterDrop(x, y) {
        if (this.useWorker && this._workerReady) {
            this.particleWorker.postMessage({ type: 'addDrops', x: x, y: y, count: 2 });
        } else {
            for (let i = 0; i < 2; i++) {
                this._fallbackDrops.push({
                    x: x + (Math.random() - 0.5) * 8,
                    y: y,
                    vx: (Math.random() - 0.5) * 1,
                    vy: 1 + Math.random(),
                    size: 2 + Math.random() * 3,
                    life: 1.5,
                });
            }
        }
    }

    drawWaterDrops() {
        const ctx = this.ctx;
        if (this.useWorker) {
            const data = this.dropData;
            const count = this.dropCount;
            for (let i = 0; i < count; i++) {
                const o = i * 6;
                const x = data[o];
                const y = data[o + 1];
                const size = data[o + 4];
                const life = data[o + 5];
                const alpha = Math.min(life, 1);
                ctx.fillStyle = `rgba(100, 181, 246, ${alpha})`;
                ctx.beginPath();
                ctx.ellipse(x, y, size * 0.6, size, 0, 0, Math.PI * 2);
                ctx.fill();
            }
        } else {
            const drops = this._fallbackDrops;
            for (let i = 0; i < drops.length; i++) {
                const d = drops[i];
                const alpha = Math.min(d.life, 1);
                ctx.fillStyle = `rgba(100, 181, 246, ${alpha})`;
                ctx.beginPath();
                ctx.ellipse(d.x, d.y, d.size * 0.6, d.size, 0, 0, Math.PI * 2);
                ctx.fill();
            }
        }
    }

    drawGround() {
        this.ctx.fillStyle = '#2d1f12';
        this.ctx.fillRect(0, this.viewHeight - 15, this.viewWidth, 15);

        this.ctx.fillStyle = '#3e2723';
        for (let x = 0; x < this.viewWidth; x += 20) {
            this.ctx.fillRect(x, this.viewHeight - 12, 15, 3);
        }
    }

    drawPlaceholder() {
        this.ctx.fillStyle = 'rgba(144, 164, 174, 0.5)';
        this.ctx.font = '20px sans-serif';
        this.ctx.textAlign = 'center';
        this.ctx.fillText('请从左侧选择筒车开始监测', this.viewWidth / 2, this.viewHeight / 2);
    }
}
