package config

import (
	"fmt"
	"time"

	"github.com/spf13/viper"
)

// Config holds all configuration for odds-adapter-service
type Config struct {
	Server    ServerConfig
	Trade360  Trade360Config
	OptiOdds  OptiOddsConfig
	Kafka     KafkaConfig
	Logging   LoggingConfig
}

// ServerConfig holds HTTP server configuration
type ServerConfig struct {
	Port         int
	ReadTimeout  time.Duration
	WriteTimeout time.Duration
}

// Trade360Config holds Trade360 RabbitMQ configuration
type Trade360Config struct {
	Enabled    bool
	URL        string // RabbitMQ connection URL
	QueueName  string
	Exchange   string
	RoutingKey string
}

// OptiOddsConfig holds OptiOdds REST API configuration
type OptiOddsConfig struct {
	Enabled      bool
	BaseURL      string
	APIKey       string
	PollInterval time.Duration
}

// KafkaConfig holds Kafka configuration
type KafkaConfig struct {
	Brokers      []string
	Topic        string
	BatchSize    int
	BatchTimeout time.Duration
}

// LoggingConfig holds logging configuration
type LoggingConfig struct {
	Level  string // debug, info, warn, error
	Format string // json, console
}

// LoadConfig loads configuration from file and environment variables
func LoadConfig(configPath string) (*Config, error) {
	v := viper.New()

	// Set defaults
	v.SetDefault("server.port", 8080)
	v.SetDefault("server.read_timeout", 30*time.Second)
	v.SetDefault("server.write_timeout", 30*time.Second)

	v.SetDefault("trade360.enabled", true)
	v.SetDefault("trade360.url", "amqp://guest:guest@localhost:5672/")
	v.SetDefault("trade360.queue_name", "trade360_odds")
	v.SetDefault("trade360.exchange", "")
	v.SetDefault("trade360.routing_key", "")

	v.SetDefault("optiodds.enabled", true)
	v.SetDefault("optiodds.base_url", "https://api.optiodds.com/v1")
	v.SetDefault("optiodds.api_key", "")
	v.SetDefault("optiodds.poll_interval", 5*time.Second)

	v.SetDefault("kafka.brokers", []string{"localhost:9092"})
	v.SetDefault("kafka.topic", "raw_odds")
	v.SetDefault("kafka.batch_size", 100)
	v.SetDefault("kafka.batch_timeout", 5*time.Second)

	v.SetDefault("logging.level", "info")
	v.SetDefault("logging.format", "json")

	// Read config file if provided
	if configPath != "" {
		v.SetConfigFile(configPath)
		if err := v.ReadInConfig(); err != nil {
			return nil, fmt.Errorf("failed to read config file: %w", err)
		}
	}

	// Override with environment variables
	v.SetEnvPrefix("ODDS_ADAPTER")
	v.AutomaticEnv()

	// Unmarshal to struct
	var config Config
	if err := v.Unmarshal(&config); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config: %w", err)
	}

	return &config, nil
}
