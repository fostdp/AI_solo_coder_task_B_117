package alarm_mqtt

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"sync"
	"time"

	mqttlib "github.com/eclipse/paho.mqtt.golang"

	"waterwheel-monitor/internal/config"
	"waterwheel-monitor/internal/database"
	"waterwheel-monitor/internal/metrics"
	"waterwheel-monitor/internal/models"
	"waterwheel-monitor/internal/pipeline"
)

type AlertPusher struct {
	db            *database.Database
	chans         *pipeline.Channels
	cfg           *models.MQTTConfig
	params        *config.AlarmParams
	client        mqttlib.Client
	lastAlert     map[int64]time.Time
	mu            sync.Mutex
	workers       int
}

func New(db *database.Database, chans *pipeline.Channels,
	cfg *models.MQTTConfig, params *config.AlarmParams) *AlertPusher {
	return &AlertPusher{
		db:        db,
		chans:     chans,
		cfg:       cfg,
		params:    params,
		lastAlert: make(map[int64]time.Time),
		workers:   1,
	}
}

func (a *AlertPusher) Start(ctx context.Context) {
	if err := a.connectMQTT(); err != nil {
		log.Printf("[Alarm MQTT] MQTT connection failed, will work in degraded mode (DB only): %v", err)
	}

	for i := 0; i < a.workers; i++ {
		go a.worker(ctx, i)
	}
	log.Printf("[Alarm MQTT] Started with %d workers, cooldown=%dmin, threshold=%.1f%%",
		a.workers, a.params.CooldownMinutes, a.params.EfficiencyThreshold*100)
}

func (a *AlertPusher) connectMQTT() error {
	opts := mqttlib.NewClientOptions().
		AddBroker(a.cfg.BrokerURL).
		SetClientID(a.cfg.ClientID).
		SetUsername(a.cfg.Username).
		SetPassword(a.cfg.Password).
		SetAutoReconnect(true).
		SetConnectRetryInterval(10 * time.Second).
		SetOnConnectHandler(func(c mqttlib.Client) {
			log.Printf("[Alarm MQTT] Connected to broker: %s", a.cfg.BrokerURL)
		}).
		SetConnectionLostHandler(func(c mqttlib.Client, err error) {
			log.Printf("[Alarm MQTT] Connection lost: %v", err)
		})

	a.client = mqttlib.NewClient(opts)
	token := a.client.Connect()
	if !token.WaitTimeout(5 * time.Second) {
		return token.Error()
	}
	return token.Error()
}

func (a *AlertPusher) worker(ctx context.Context, id int) {
	for {
		select {
		case <-ctx.Done():
			log.Printf("[Alarm MQTT] Worker %d stopped", id)
			return
		case msg, ok := <-a.chans.AlertCh:
			if !ok {
				log.Printf("[Alarm MQTT] Worker %d: AlertCh closed", id)
				return
			}
			a.handleAlert(&msg)
		}
	}
}

func (a *AlertPusher) handleAlert(msg *pipeline.AlertMsg) {
	if !a.shouldAlert(msg.Wheel.ID) {
		return
	}

	wheel := msg.Wheel
	alert := models.Alert{
		WaterwheelID:         wheel.ID,
		WaterwheelName:       wheel.Name,
		Time:                 msg.Data.Time,
		Type:                 models.AlertTypeLowEfficiency,
		Severity:             a.classifySeverity(msg.CurrentEff, msg.HistoricalAvg),
		Message:              a.buildMessage(wheel, msg),
		CurrentEfficiency:    msg.CurrentEff,
		HistoricalEfficiency: msg.HistoricalAvg,
		Threshold:            msg.Threshold,
		RotationSpeed:        msg.Data.RotationSpeed,
		WaterLift:            msg.Data.WaterLift,
		WaterLevelDrop:       msg.Data.WaterLevelDrop,
		FlowVelocity:         msg.Data.FlowVelocity,
		Acknowledged:         false,
	}

	if err := a.db.InsertAlert(context.Background(), &alert); err != nil {
		log.Printf("[Alarm MQTT] DB insert alert error (wheel=%d): %v", wheel.ID, err)
	}

	if a.client != nil && a.client.IsConnected() {
		a.publishMQTT(&alert)
	}

	a.markAlerted(wheel.ID)
	metrics.IncAlert(string(alert.Severity))
	log.Printf("[Alarm MQTT] Alerted wheel=%d eff=%.4f hist=%.4f sev=%s", wheel.ID, msg.CurrentEff, msg.HistoricalAvg, alert.Severity)
}

func (a *AlertPusher) shouldAlert(wheelID int64) bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	last, ok := a.lastAlert[wheelID]
	if !ok {
		return true
	}
	cooldown := time.Duration(a.params.CooldownMinutes) * time.Minute
	return time.Since(last) >= cooldown
}

func (a *AlertPusher) markAlerted(wheelID int64) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.lastAlert[wheelID] = time.Now()
}

func (a *AlertPusher) classifySeverity(current, historical float64) models.AlertSeverity {
	if historical <= 0 {
		return models.SeverityWarning
	}
	ratio := current / historical
	switch {
	case ratio < 0.5:
		return models.SeverityCritical
	case ratio < 0.65:
		return models.SeverityMajor
	default:
		return models.SeverityWarning
	}
}

func (a *AlertPusher) buildMessage(wheel *models.Waterwheel, msg *pipeline.AlertMsg) string {
	dropPct := (1.0 - msg.CurrentEff/msg.HistoricalAvg) * 100.0
	return fmt.Sprintf("%s 效率异常下降 %.1f%% 当前=%.2f%% 历史均值=%.2f%% rpm=%.2f lift=%.1fL/min",
		wheel.Name, dropPct, msg.CurrentEff*100, msg.HistoricalAvg*100,
		msg.Data.RotationSpeed, msg.Data.WaterLift)
}

func (a *AlertPusher) publishMQTT(alert *models.Alert) {
	topic := a.cfg.TopicPrefix + "alerts"
	payload, err := json.Marshal(alert)
	if err != nil {
		log.Printf("[Alarm MQTT] JSON marshal error: %v", err)
		return
	}

	token := a.client.Publish(topic, 1, false, payload)
	done := make(chan error, 1)
	go func() {
		token.Wait()
		done <- token.Error()
	}()

	select {
	case err := <-done:
		if err != nil {
			log.Printf("[Alarm MQTT] Publish failed: %v", err)
		}
	case <-time.After(3 * time.Second):
		log.Printf("[Alarm MQTT] Publish timeout (wheel=%d)", alert.WaterwheelID)
	}
}
