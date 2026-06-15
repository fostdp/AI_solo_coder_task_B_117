package config

import (
	"fmt"
	"os"
	"strconv"

	"github.com/joho/godotenv"
)

type Config struct {
	DBHost                   string
	DBPort                   string
	DBUser                   string
	DBPassword               string
	DBName                   string
	ServerPort               string
	MQTTBroker               string
	MQTTClientID             string
	MQTTUsername             string
	MQTTPassword             string
	MQTTTopicPrefix          string
	MQTTTopic                string
	EfficiencyAlertThreshold float64
}

func Load() (*Config, error) {
	_ = godotenv.Load()

	threshold := 0.8
	if t, err := strconv.ParseFloat(getEnv("EFFICIENCY_ALERT_THRESHOLD", "0.8"), 64); err == nil {
		threshold = t
	}

	return &Config{
		DBHost:                   getEnv("DB_HOST", "localhost"),
		DBPort:                   getEnv("DB_PORT", "5432"),
		DBUser:                   getEnv("DB_USER", "postgres"),
		DBPassword:               getEnv("DB_PASSWORD", "postgres"),
		DBName:                   getEnv("DB_NAME", "waterwheel"),
		ServerPort:               getEnv("SERVER_PORT", "8080"),
		MQTTBroker:               getEnv("MQTT_BROKER", "tcp://localhost:1883"),
		MQTTClientID:             getEnv("MQTT_CLIENT_ID", "waterwheel-alert"),
		MQTTUsername:             getEnv("MQTT_USERNAME", ""),
		MQTTPassword:             getEnv("MQTT_PASSWORD", ""),
		MQTTTopicPrefix:          getEnv("MQTT_TOPIC_PREFIX", "waterwheel/"),
		MQTTTopic:                getEnv("MQTT_TOPIC", "waterwheel/alerts"),
		EfficiencyAlertThreshold: threshold,
	}, nil
}

func (c *Config) DSN() string {
	return fmt.Sprintf(
		"host=%s port=%s user=%s password=%s dbname=%s sslmode=disable",
		c.DBHost, c.DBPort, c.DBUser, c.DBPassword, c.DBName,
	)
}

func getEnv(key, fallback string) string {
	if value, exists := os.LookupEnv(key); exists {
		return value
	}
	return fallback
}
