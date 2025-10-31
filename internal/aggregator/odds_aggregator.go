package aggregator

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog"
	"github.com/segmentio/kafka-go"

	"github.com/cypherlabdev/odds-adapter-service/internal/models"
)

// KafkaWriter interface for testing
type KafkaWriter interface {
	WriteMessages(ctx context.Context, msgs ...kafka.Message) error
	Close() error
}

// OddsAggregator aggregates odds from multiple providers and publishes to Kafka
type OddsAggregator struct {
	oddsInput    <-chan *models.RawOdds
	kafkaWriter  KafkaWriter
	batchSize    int
	batchTimeout time.Duration
	logger       zerolog.Logger
}

// AggregatorConfig holds configuration for the odds aggregator
type AggregatorConfig struct {
	KafkaBrokers []string      // e.g., ["localhost:9092"]
	KafkaTopic   string        // e.g., "raw_odds"
	BatchSize    int           // e.g., 100 (send batch when this many odds collected)
	BatchTimeout time.Duration // e.g., 5 * time.Second (send batch after this duration)
}

// NewOddsAggregator creates a new odds aggregator
func NewOddsAggregator(
	config AggregatorConfig,
	oddsInput <-chan *models.RawOdds,
	logger zerolog.Logger,
) *OddsAggregator {
	// Create Kafka writer
	writer := &kafka.Writer{
		Addr:         kafka.TCP(config.KafkaBrokers...),
		Topic:        config.KafkaTopic,
		Balancer:     &kafka.LeastBytes{},
		RequiredAcks: kafka.RequireAll, // Wait for all replicas
		MaxAttempts:  3,
		BatchSize:    config.BatchSize,
		BatchTimeout: config.BatchTimeout,
		Compression:  kafka.Snappy,
	}

	return &OddsAggregator{
		oddsInput:    oddsInput,
		kafkaWriter:  writer,
		batchSize:    config.BatchSize,
		batchTimeout: config.BatchTimeout,
		logger:       logger.With().Str("component", "odds_aggregator").Logger(),
	}
}

// Start begins aggregating and publishing odds
func (a *OddsAggregator) Start(ctx context.Context) error {
	a.logger.Info().
		Int("batch_size", a.batchSize).
		Dur("batch_timeout", a.batchTimeout).
		Msg("started odds aggregator")

	batch := make([]*models.RawOdds, 0, a.batchSize)
	ticker := time.NewTicker(a.batchTimeout)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			// Flush remaining batch
			if len(batch) > 0 {
				if err := a.publishBatch(ctx, batch); err != nil {
					a.logger.Error().Err(err).Msg("failed to flush final batch")
				}
			}
			a.logger.Info().Msg("stopping odds aggregator")
			return a.kafkaWriter.Close()

		case odds, ok := <-a.oddsInput:
			if !ok {
				a.logger.Warn().Msg("odds input channel closed")
				return nil
			}

			// Add to batch
			batch = append(batch, odds)

			// Publish batch if full
			if len(batch) >= a.batchSize {
				if err := a.publishBatch(ctx, batch); err != nil {
					a.logger.Error().Err(err).Msg("failed to publish batch")
					// Continue collecting, don't lose data
				} else {
					batch = batch[:0] // Clear batch
				}
			}

		case <-ticker.C:
			// Publish batch on timeout
			if len(batch) > 0 {
				if err := a.publishBatch(ctx, batch); err != nil {
					a.logger.Error().Err(err).Msg("failed to publish timeout batch")
				} else {
					batch = batch[:0] // Clear batch
				}
			}
		}
	}
}

// publishBatch publishes a batch of odds to Kafka
func (a *OddsAggregator) publishBatch(ctx context.Context, batch []*models.RawOdds) error {
	if len(batch) == 0 {
		return nil
	}

	// Group by provider for better organization
	providerBatches := make(map[models.Provider][]*models.RawOdds)
	for _, odds := range batch {
		providerBatches[odds.Provider] = append(providerBatches[odds.Provider], odds)
	}

	// Publish each provider batch
	for provider, providerOdds := range providerBatches {
		// Convert slice of pointers to slice of values
		oddsData := make([]models.RawOdds, len(providerOdds))
		for i, odds := range providerOdds {
			oddsData[i] = *odds
		}

		message := &models.KafkaOddsMessage{
			OddsData:  oddsData,
			Provider:  provider,
			Timestamp: time.Now().UTC(),
			BatchID:   uuid.New().String(),
		}

		// Serialize to JSON
		payload, err := json.Marshal(message)
		if err != nil {
			return fmt.Errorf("failed to marshal Kafka message: %w", err)
		}

		// Create Kafka message
		kafkaMsg := kafka.Message{
			Key:   []byte(string(provider)), // Partition by provider
			Value: payload,
			Headers: []kafka.Header{
				{Key: "provider", Value: []byte(string(provider))},
				{Key: "batch_id", Value: []byte(message.BatchID)},
				{Key: "timestamp", Value: []byte(message.Timestamp.Format(time.RFC3339))},
			},
		}

		// Write to Kafka
		if err := a.kafkaWriter.WriteMessages(ctx, kafkaMsg); err != nil {
			a.logger.Error().
				Err(err).
				Str("provider", string(provider)).
				Int("odds_count", len(providerOdds)).
				Msg("failed to write to Kafka")
			return fmt.Errorf("failed to write to Kafka: %w", err)
		}

		a.logger.Info().
			Str("provider", string(provider)).
			Int("odds_count", len(providerOdds)).
			Str("batch_id", message.BatchID).
			Msg("published odds batch to Kafka")
	}

	return nil
}

// Close closes the Kafka writer
func (a *OddsAggregator) Close() error {
	return a.kafkaWriter.Close()
}
