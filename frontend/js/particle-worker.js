let particles = [];
let config = {
    count: 80,
    width: 800,
    height: 500,
    waterY: 0.75,
    waterHeight: 0.25,
    flowSpeed: 1.0,
};

let waterDrops = [];

function init(cfg) {
    config = Object.assign(config, cfg);
    particles = [];
    for (let i = 0; i < config.count; i++) {
        particles.push(createParticle());
    }
    waterDrops = [];
    postParticles();
}

function createParticle() {
    const yBase = config.height * config.waterY;
    return {
        x: Math.random() * config.width,
        y: yBase + Math.random() * (config.height * config.waterHeight),
        vx: (1 + Math.random() * 3) * config.flowSpeed,
        vy: (Math.random() - 0.5) * 0.5,
        size: 1 + Math.random() * 3,
        alpha: 0.3 + Math.random() * 0.5,
    };
}

function update(dt) {
    const waterY = config.height * config.waterY;
    const waterBottom = config.height - 5;

    for (let i = 0; i < particles.length; i++) {
        const p = particles[i];
        p.x += p.vx * dt * 60;
        p.y += p.vy * dt * 60;

        if (p.x > config.width + 10) {
            p.x = -10;
            p.y = waterY + Math.random() * (config.height * config.waterHeight);
        }
        if (p.y < waterY - 5) {
            p.y = waterY + 2;
        }
        if (p.y > waterBottom) {
            p.y = waterBottom - 2;
        }
    }

    for (let i = waterDrops.length - 1; i >= 0; i--) {
        const d = waterDrops[i];
        d.x += d.vx * dt * 60;
        d.y += d.vy * dt * 60;
        d.vy += 0.15 * dt * 60;
        d.life -= dt;
        if (d.life <= 0 || d.y > config.height) {
            waterDrops.splice(i, 1);
        }
    }

    postParticles();
}

function addWaterDrops(x, y, count) {
    for (let i = 0; i < count; i++) {
        waterDrops.push({
            x: x + (Math.random() - 0.5) * 8,
            y: y,
            vx: (Math.random() - 0.5) * 1,
            vy: 1 + Math.random(),
            size: 2 + Math.random() * 3,
            life: 1.5,
        });
    }
}

function updateConfig(newCfg) {
    config = Object.assign(config, newCfg);
}

function postParticles() {
    const pData = new Float32Array(particles.length * 6);
    for (let i = 0; i < particles.length; i++) {
        const p = particles[i];
        const o = i * 6;
        pData[o]     = p.x;
        pData[o + 1] = p.y;
        pData[o + 2] = p.vx;
        pData[o + 3] = p.vy;
        pData[o + 4] = p.size;
        pData[o + 5] = p.alpha;
    }

    const dData = new Float32Array(waterDrops.length * 6);
    for (let i = 0; i < waterDrops.length; i++) {
        const d = waterDrops[i];
        const o = i * 6;
        dData[o]     = d.x;
        dData[o + 1] = d.y;
        dData[o + 2] = d.vx;
        dData[o + 3] = d.vy;
        dData[o + 4] = d.size;
        dData[o + 5] = d.life;
    }

    self.postMessage({
        type: 'particles',
        particles: pData.buffer,
        particleCount: particles.length,
        drops: dData.buffer,
        dropCount: waterDrops.length,
    }, [pData.buffer, dData.buffer]);
}

self.onmessage = function(e) {
    const msg = e.data;
    switch (msg.type) {
        case 'init':
            init(msg.config);
            break;
        case 'update':
            update(msg.dt);
            break;
        case 'config':
            updateConfig(msg.config);
            break;
        case 'addDrops':
            addWaterDrops(msg.x, msg.y, msg.count || 2);
            break;
    }
};
