package config

import (
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestLoadConfig_Defaults tests loading config with default values
func TestLoadConfig_Defaults(t *testing.T) {
	// Load config without file
	cfg, err := LoadConfig("")

	// Assert
	assert.NoError(t, err)
	assert.NotNil(t, cfg)

	// Verify defaults
	assert.Equal(t, 8080, cfg.Server.Port)
	assert.Equal(t, 30*time.Second, cfg.Server.ReadTimeout)
	assert.Equal(t, 30*time.Second, cfg.Server.WriteTimeout)

	assert.True(t, cfg.Trade360.Enabled)
	assert.Equal(t, "amqp://guest:guest@localhost:5672/", cfg.Trade360.URL)
	assert.Equal(t, "trade360_odds", cfg.Trade360.QueueName)

	assert.True(t, cfg.OptiOdds.Enabled)
	assert.Equal(t, "https://api.optiodds.com/v1", cfg.OptiOdds.BaseURL)
	assert.Equal(t, 5*time.Second, cfg.OptiOdds.PollInterval)

	assert.Equal(t, []string{"localhost:9092"}, cfg.Kafka.Brokers)
	assert.Equal(t, "raw_odds", cfg.Kafka.Topic)
	assert.Equal(t, 100, cfg.Kafka.BatchSize)
	assert.Equal(t, 5*time.Second, cfg.Kafka.BatchTimeout)

	assert.Equal(t, "info", cfg.Logging.Level)
	assert.Equal(t, "json", cfg.Logging.Format)
}

// TestLoadConfig_FromFile tests loading config from YAML file
func TestLoadConfig_FromFile(t *testing.T) {
	// Create temporary config file
	configContent := `
server:
  port: 9090
  read_timeout: 60s
  write_timeout: 60s

trade360:
  enabled: false
  url: amqp://user:pass@localhost:5672/
  queue_name: custom_queue
  exchange: custom_exchange
  routing_key: custom_key

optiodds:
  enabled: true
  base_url: https://custom.optiodds.com
  api_key: custom-key
  poll_interval: 10s

kafka:
  brokers:
    - broker1:9092
    - broker2:9092
  topic: custom_topic
  batch_size: 200
  batch_timeout: 10s

logging:
  level: debug
  format: console
`

	tmpFile, err := os.CreateTemp("", "config-*.yaml")
	require.NoError(t, err)
	defer os.Remove(tmpFile.Name())

	_, err = tmpFile.WriteString(configContent)
	require.NoError(t, err)
	tmpFile.Close()

	// Load config from file
	cfg, err := LoadConfig(tmpFile.Name())

	// Assert
	assert.NoError(t, err)
	assert.NotNil(t, cfg)

	// Verify custom values
	assert.Equal(t, 9090, cfg.Server.Port)
	assert.Equal(t, 60*time.Second, cfg.Server.ReadTimeout)
	assert.Equal(t, 60*time.Second, cfg.Server.WriteTimeout)

	assert.False(t, cfg.Trade360.Enabled)
	assert.Equal(t, "amqp://user:pass@localhost:5672/", cfg.Trade360.URL)
	assert.Equal(t, "custom_queue", cfg.Trade360.QueueName)
	assert.Equal(t, "custom_exchange", cfg.Trade360.Exchange)
	assert.Equal(t, "custom_key", cfg.Trade360.RoutingKey)

	assert.True(t, cfg.OptiOdds.Enabled)
	assert.Equal(t, "https://custom.optiodds.com", cfg.OptiOdds.BaseURL)
	assert.Equal(t, "custom-key", cfg.OptiOdds.APIKey)
	assert.Equal(t, 10*time.Second, cfg.OptiOdds.PollInterval)

	assert.Equal(t, []string{"broker1:9092", "broker2:9092"}, cfg.Kafka.Brokers)
	assert.Equal(t, "custom_topic", cfg.Kafka.Topic)
	assert.Equal(t, 200, cfg.Kafka.BatchSize)
	assert.Equal(t, 10*time.Second, cfg.Kafka.BatchTimeout)

	assert.Equal(t, "debug", cfg.Logging.Level)
	assert.Equal(t, "console", cfg.Logging.Format)
}

// TestLoadConfig_InvalidFile tests loading config from non-existent file
func TestLoadConfig_InvalidFile(t *testing.T) {
	cfg, err := LoadConfig("/non/existent/config.yaml")

	// Assert
	assert.Error(t, err)
	assert.Nil(t, cfg)
}

// TestLoadConfig_MalformedYAML tests loading config from malformed YAML
func TestLoadConfig_MalformedYAML(t *testing.T) {
	configContent := `
server:
  port: invalid
  nested:
    - item1
  malformed yaml here
`

	tmpFile, err := os.CreateTemp("", "config-malformed-*.yaml")
	require.NoError(t, err)
	defer os.Remove(tmpFile.Name())

	_, err = tmpFile.WriteString(configContent)
	require.NoError(t, err)
	tmpFile.Close()

	// Load config from file
	cfg, err := LoadConfig(tmpFile.Name())

	// Assert - should error on malformed YAML
	assert.Error(t, err)
	assert.Nil(t, cfg)
}

// TestLoadConfig_PartialConfig tests loading config with partial values
func TestLoadConfig_PartialConfig(t *testing.T) {
	configContent := `
server:
  port: 8888

kafka:
  brokers:
    - custom:9092
`

	tmpFile, err := os.CreateTemp("", "config-partial-*.yaml")
	require.NoError(t, err)
	defer os.Remove(tmpFile.Name())

	_, err = tmpFile.WriteString(configContent)
	require.NoError(t, err)
	tmpFile.Close()

	// Load config from file
	cfg, err := LoadConfig(tmpFile.Name())

	// Assert
	assert.NoError(t, err)
	assert.NotNil(t, cfg)

	// Verify custom values
	assert.Equal(t, 8888, cfg.Server.Port)
	assert.Equal(t, []string{"custom:9092"}, cfg.Kafka.Brokers)

	// Verify defaults are still applied
	assert.Equal(t, 30*time.Second, cfg.Server.ReadTimeout)
	assert.Equal(t, "raw_odds", cfg.Kafka.Topic)
	assert.Equal(t, "info", cfg.Logging.Level)
}

// TestLoadConfig_EmptyFile tests loading config from empty file
func TestLoadConfig_EmptyFile(t *testing.T) {
	tmpFile, err := os.CreateTemp("", "config-empty-*.yaml")
	require.NoError(t, err)
	defer os.Remove(tmpFile.Name())
	tmpFile.Close()

	// Load config from empty file
	cfg, err := LoadConfig(tmpFile.Name())

	// Assert - should succeed and use defaults
	assert.NoError(t, err)
	assert.NotNil(t, cfg)

	// Verify defaults
	assert.Equal(t, 8080, cfg.Server.Port)
	assert.Equal(t, "raw_odds", cfg.Kafka.Topic)
}

// TestLoadConfig_TypeMismatch tests loading config with wrong types
func TestLoadConfig_TypeMismatch(t *testing.T) {
	configContent := `
server:
  port: "not-a-number"
  read_timeout: invalid-duration
`

	tmpFile, err := os.CreateTemp("", "config-type-*.yaml")
	require.NoError(t, err)
	defer os.Remove(tmpFile.Name())

	_, err = tmpFile.WriteString(configContent)
	require.NoError(t, err)
	tmpFile.Close()

	// Load config from file
	cfg, err := LoadConfig(tmpFile.Name())

	// Assert - should error on type mismatch
	assert.Error(t, err)
	assert.Nil(t, cfg)
}

// TestLoadConfig_AllAdaptersDisabled tests config with all adapters disabled
func TestLoadConfig_AllAdaptersDisabled(t *testing.T) {
	configContent := `
trade360:
  enabled: false

optiodds:
  enabled: false
`

	tmpFile, err := os.CreateTemp("", "config-disabled-*.yaml")
	require.NoError(t, err)
	defer os.Remove(tmpFile.Name())

	_, err = tmpFile.WriteString(configContent)
	require.NoError(t, err)
	tmpFile.Close()

	// Load config from file
	cfg, err := LoadConfig(tmpFile.Name())

	// Assert
	assert.NoError(t, err)
	assert.NotNil(t, cfg)

	// Verify adapters are disabled
	assert.False(t, cfg.Trade360.Enabled)
	assert.False(t, cfg.OptiOdds.Enabled)
}

// TestLoadConfig_MultipleKafkaBrokers tests config with multiple Kafka brokers
func TestLoadConfig_MultipleKafkaBrokers(t *testing.T) {
	configContent := `
kafka:
  brokers:
    - broker1.example.com:9092
    - broker2.example.com:9092
    - broker3.example.com:9092
`

	tmpFile, err := os.CreateTemp("", "config-brokers-*.yaml")
	require.NoError(t, err)
	defer os.Remove(tmpFile.Name())

	_, err = tmpFile.WriteString(configContent)
	require.NoError(t, err)
	tmpFile.Close()

	// Load config from file
	cfg, err := LoadConfig(tmpFile.Name())

	// Assert
	assert.NoError(t, err)
	assert.NotNil(t, cfg)

	// Verify brokers
	assert.Equal(t, 3, len(cfg.Kafka.Brokers))
	assert.Contains(t, cfg.Kafka.Brokers, "broker1.example.com:9092")
	assert.Contains(t, cfg.Kafka.Brokers, "broker2.example.com:9092")
	assert.Contains(t, cfg.Kafka.Brokers, "broker3.example.com:9092")
}

// TestLoadConfig_ZeroValues tests config with explicit zero values
func TestLoadConfig_ZeroValues(t *testing.T) {
	configContent := `
server:
  port: 0

kafka:
  batch_size: 0
`

	tmpFile, err := os.CreateTemp("", "config-zero-*.yaml")
	require.NoError(t, err)
	defer os.Remove(tmpFile.Name())

	_, err = tmpFile.WriteString(configContent)
	require.NoError(t, err)
	tmpFile.Close()

	// Load config from file
	cfg, err := LoadConfig(tmpFile.Name())

	// Assert
	assert.NoError(t, err)
	assert.NotNil(t, cfg)

	// Verify zero values are preserved (not overridden by defaults)
	assert.Equal(t, 0, cfg.Server.Port)
	assert.Equal(t, 0, cfg.Kafka.BatchSize)
}

// TestLoadConfig_LongDurations tests config with various duration formats
func TestLoadConfig_LongDurations(t *testing.T) {
	configContent := `
server:
  read_timeout: 2m30s
  write_timeout: 1h

optiodds:
  poll_interval: 500ms

kafka:
  batch_timeout: 15s
`

	tmpFile, err := os.CreateTemp("", "config-duration-*.yaml")
	require.NoError(t, err)
	defer os.Remove(tmpFile.Name())

	_, err = tmpFile.WriteString(configContent)
	require.NoError(t, err)
	tmpFile.Close()

	// Load config from file
	cfg, err := LoadConfig(tmpFile.Name())

	// Assert
	assert.NoError(t, err)
	assert.NotNil(t, cfg)

	// Verify durations
	assert.Equal(t, 2*time.Minute+30*time.Second, cfg.Server.ReadTimeout)
	assert.Equal(t, 1*time.Hour, cfg.Server.WriteTimeout)
	assert.Equal(t, 500*time.Millisecond, cfg.OptiOdds.PollInterval)
	assert.Equal(t, 15*time.Second, cfg.Kafka.BatchTimeout)
}

// TestLoadConfig_MinimalValidConfig tests minimal valid configuration
func TestLoadConfig_MinimalValidConfig(t *testing.T) {
	configContent := `
server:
  port: 8080

kafka:
  brokers:
    - localhost:9092
  topic: odds
`

	tmpFile, err := os.CreateTemp("", "config-minimal-*.yaml")
	require.NoError(t, err)
	defer os.Remove(tmpFile.Name())

	_, err = tmpFile.WriteString(configContent)
	require.NoError(t, err)
	tmpFile.Close()

	// Load config from file
	cfg, err := LoadConfig(tmpFile.Name())

	// Assert
	assert.NoError(t, err)
	assert.NotNil(t, cfg)

	// Verify essential fields
	assert.Equal(t, 8080, cfg.Server.Port)
	assert.Equal(t, []string{"localhost:9092"}, cfg.Kafka.Brokers)
	assert.Equal(t, "odds", cfg.Kafka.Topic)
}

// TestServerConfig_Structure tests ServerConfig structure
func TestServerConfig_Structure(t *testing.T) {
	cfg := ServerConfig{
		Port:         9000,
		ReadTimeout:  45 * time.Second,
		WriteTimeout: 45 * time.Second,
	}

	assert.Equal(t, 9000, cfg.Port)
	assert.Equal(t, 45*time.Second, cfg.ReadTimeout)
	assert.Equal(t, 45*time.Second, cfg.WriteTimeout)
}

// TestTrade360Config_Structure tests Trade360Config structure
func TestTrade360Config_Structure(t *testing.T) {
	cfg := Trade360Config{
		Enabled:    true,
		URL:        "amqp://test:test@host:5672/",
		QueueName:  "test_queue",
		Exchange:   "test_exchange",
		RoutingKey: "test_key",
	}

	assert.True(t, cfg.Enabled)
	assert.Equal(t, "amqp://test:test@host:5672/", cfg.URL)
	assert.Equal(t, "test_queue", cfg.QueueName)
	assert.Equal(t, "test_exchange", cfg.Exchange)
	assert.Equal(t, "test_key", cfg.RoutingKey)
}

// TestOptiOddsConfig_Structure tests OptiOddsConfig structure
func TestOptiOddsConfig_Structure(t *testing.T) {
	cfg := OptiOddsConfig{
		Enabled:      true,
		BaseURL:      "https://api.test.com",
		APIKey:       "test-key",
		PollInterval: 3 * time.Second,
	}

	assert.True(t, cfg.Enabled)
	assert.Equal(t, "https://api.test.com", cfg.BaseURL)
	assert.Equal(t, "test-key", cfg.APIKey)
	assert.Equal(t, 3*time.Second, cfg.PollInterval)
}

// TestKafkaConfig_Structure tests KafkaConfig structure
func TestKafkaConfig_Structure(t *testing.T) {
	cfg := KafkaConfig{
		Brokers:      []string{"broker1:9092", "broker2:9092"},
		Topic:        "test_topic",
		BatchSize:    150,
		BatchTimeout: 7 * time.Second,
	}

	assert.Equal(t, 2, len(cfg.Brokers))
	assert.Equal(t, "test_topic", cfg.Topic)
	assert.Equal(t, 150, cfg.BatchSize)
	assert.Equal(t, 7*time.Second, cfg.BatchTimeout)
}

// TestLoggingConfig_Structure tests LoggingConfig structure
func TestLoggingConfig_Structure(t *testing.T) {
	cfg := LoggingConfig{
		Level:  "warn",
		Format: "console",
	}

	assert.Equal(t, "warn", cfg.Level)
	assert.Equal(t, "console", cfg.Format)
}

// TestConfig_CompleteStructure tests complete Config structure
func TestConfig_CompleteStructure(t *testing.T) {
	cfg := Config{
		Server: ServerConfig{
			Port:         8080,
			ReadTimeout:  30 * time.Second,
			WriteTimeout: 30 * time.Second,
		},
		Trade360: Trade360Config{
			Enabled:    true,
			URL:        "amqp://localhost:5672/",
			QueueName:  "queue",
			Exchange:   "exchange",
			RoutingKey: "key",
		},
		OptiOdds: OptiOddsConfig{
			Enabled:      true,
			BaseURL:      "https://api.optiodds.com",
			APIKey:       "key",
			PollInterval: 5 * time.Second,
		},
		Kafka: KafkaConfig{
			Brokers:      []string{"localhost:9092"},
			Topic:        "topic",
			BatchSize:    100,
			BatchTimeout: 5 * time.Second,
		},
		Logging: LoggingConfig{
			Level:  "info",
			Format: "json",
		},
	}

	// Verify all fields are accessible
	assert.NotNil(t, cfg.Server)
	assert.NotNil(t, cfg.Trade360)
	assert.NotNil(t, cfg.OptiOdds)
	assert.NotNil(t, cfg.Kafka)
	assert.NotNil(t, cfg.Logging)
}
