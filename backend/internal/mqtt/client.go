package mqtt

import (
	"encoding/json"
	"fmt"
	"log"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"

	"waterwheel-monitor/internal/config"
	"waterwheel-monitor/internal/models"
)

type AlertClient struct {
	client mqtt.Client
	topic  string
}

func NewAlertClient(cfg *config.Config) (*AlertClient, error) {
	opts := mqtt.NewClientOptions()
	opts.AddBroker(cfg.MQTTBroker)
	opts.SetClientID(cfg.MQTTClientID)
	opts.SetAutoReconnect(true)
	opts.SetConnectRetry(true)
	opts.SetConnectRetryInterval(5 * time.Second)
	opts.SetMaxReconnectInterval(1 * time.Minute)

	opts.OnConnect = func(c mqtt.Client) {
		log.Println("MQTT client connected")
	}
	opts.OnConnectionLost = func(c mqtt.Client, err error) {
		log.Printf("MQTT connection lost: %v", err)
	}

	client := mqtt.NewClient(opts)

	token := client.Connect()
	if token.WaitTimeout(10 * time.Second) {
		if err := token.Error(); err != nil {
			return nil, fmt.Errorf("mqtt connect: %w", err)
		}
	}

	return &AlertClient{
		client: client,
		topic:  cfg.MQTTTopic,
	}, nil
}

func (ac *AlertClient) Close() {
	if ac.client != nil && ac.client.IsConnected() {
		ac.client.Disconnect(250)
	}
}

type AlertMessage struct {
	Timestamp       time.Time `json:"timestamp"`
	WaterwheelID    int       `json:"waterwheel_id"`
	WaterwheelName  string    `json:"waterwheel_name,omitempty"`
	AlertType       string    `json:"alert_type"`
	Severity        string    `json:"severity"`
	Message         string    `json:"message"`
	EfficiencyValue float64   `json:"efficiency_value"`
	HistoricalAvg   float64   `json:"historical_avg"`
	Threshold       float64   `json:"threshold"`
}

func (ac *AlertClient) PublishAlert(alert *models.Alert, wheelName string, threshold float64) error {
	msg := AlertMessage{
		Timestamp:       alert.Time,
		WaterwheelID:    alert.WaterwheelID,
		WaterwheelName:  wheelName,
		AlertType:       alert.AlertType,
		Severity:        alert.Severity,
		Message:         alert.Message,
		EfficiencyValue: alert.EfficiencyValue,
		HistoricalAvg:   alert.HistoricalAvg,
		Threshold:       threshold,
	}

	payload, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("marshal alert: %w", err)
	}

	topic := fmt.Sprintf("%s/%d", ac.topic, alert.WaterwheelID)
	token := ac.client.Publish(topic, 1, false, payload)

	if !token.WaitTimeout(5 * time.Second) {
		return fmt.Errorf("mqtt publish timeout")
	}
	if err := token.Error(); err != nil {
		return fmt.Errorf("mqtt publish: %w", err)
	}

	log.Printf("Alert published to MQTT topic %s: waterwheel=%d, type=%s", topic, alert.WaterwheelID, alert.AlertType)
	return nil
}
