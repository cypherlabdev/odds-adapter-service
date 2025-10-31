package aggregator

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/cypherlabdev/odds-adapter-service/internal/models"
	"github.com/google/uuid"
	"github.com/rs/zerolog"
	"github.com/segmentio/kafka-go"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockKafkaWriter is a mock implementation of Kafka writer for testing
type mockKafkaWriter struct {
	messages []kafka.Message
	writeErr error
	closed   bool
}

func (m *mockKafkaWriter) WriteMessages(ctx context.Context, msgs ...kafka.Message) error {
	if m.writeErr != nil {
		return m.writeErr
	}
	m.messages = append(m.messages, msgs...)
	return nil
}

func (m *mockKafkaWriter) Close() error {
	m.closed = true
	return nil
}

// testAggregatorSetup is a helper struct to hold test dependencies
type testAggregatorSetup struct {
	aggregator  *OddsAggregator
	oddsInput   chan *models.RawOdds
	mockWriter  *mockKafkaWriter
}

// setupTestAggregator creates a test aggregator with mock Kafka writer
func setupTestAggregator(t *testing.T, batchSize int, batchTimeout time.Duration) *testAggregatorSetup {
	oddsInput := make(chan *models.RawOdds, 1000)
	logger := zerolog.Nop()
	mockWriter := &mockKafkaWriter{
		messages: []kafka.Message{},
	}

	config := AggregatorConfig{
		KafkaBrokers: []string{"localhost:9092"},
		KafkaTopic:   "test_topic",
		BatchSize:    batchSize,
		BatchTimeout: batchTimeout,
	}

	aggregator := NewOddsAggregator(config, oddsInput, logger)
	// Replace the real Kafka writer with mock
	aggregator.kafkaWriter = mockWriter

	return &testAggregatorSetup{
		aggregator: aggregator,
		oddsInput:  oddsInput,
		mockWriter: mockWriter,
	}
}

// cleanup cleans up test resources
func (s *testAggregatorSetup) cleanup() {
	close(s.oddsInput)
}

// TestNewOddsAggregator_Success tests successful aggregator creation
func TestNewOddsAggregator_Success(t *testing.T) {
	oddsInput := make(chan *models.RawOdds, 100)
	logger := zerolog.Nop()

	config := AggregatorConfig{
		KafkaBrokers: []string{"localhost:9092", "localhost:9093"},
		KafkaTopic:   "raw_odds",
		BatchSize:    50,
		BatchTimeout: 3 * time.Second,
	}

	aggregator := NewOddsAggregator(config, oddsInput, logger)

	assert.NotNil(t, aggregator)
	assert.Equal(t, 50, aggregator.batchSize)
	assert.Equal(t, 3*time.Second, aggregator.batchTimeout)
	assert.NotNil(t, aggregator.kafkaWriter)
}

// TestOddsAggregator_PublishBatch_Success tests successful batch publishing
func TestOddsAggregator_PublishBatch_Success(t *testing.T) {
	setup := setupTestAggregator(t, 10, 1*time.Second)
	defer setup.cleanup()

	ctx := context.Background()

	// Create test odds
	odds1 := &models.RawOdds{
		ID:          uuid.New(),
		Provider:    models.ProviderOptiOdds,
		EventID:     "event-1",
		EventName:   "Test Event 1",
		Sport:       "football",
		Competition: "Test League",
		Market:      models.MarketTypeMatchOdds,
		Selection:   "Team A",
		BackPrice:   decimal.NewFromFloat(2.50),
		LayPrice:    decimal.Zero,
		BackSize:    decimal.NewFromFloat(1000.00),
		LaySize:     decimal.Zero,
		Timestamp:   time.Now().UTC(),
		ReceivedAt:  time.Now().UTC(),
		RawPayload:  map[string]interface{}{},
	}

	odds2 := &models.RawOdds{
		ID:          uuid.New(),
		Provider:    models.ProviderOptiOdds,
		EventID:     "event-2",
		EventName:   "Test Event 2",
		Sport:       "tennis",
		Competition: "Grand Slam",
		Market:      models.MarketTypeMatchOdds,
		Selection:   "Player X",
		BackPrice:   decimal.NewFromFloat(1.80),
		LayPrice:    decimal.Zero,
		BackSize:    decimal.NewFromFloat(500.00),
		LaySize:     decimal.Zero,
		Timestamp:   time.Now().UTC(),
		ReceivedAt:  time.Now().UTC(),
		RawPayload:  map[string]interface{}{},
	}

	batch := []*models.RawOdds{odds1, odds2}

	// Execute
	err := setup.aggregator.publishBatch(ctx, batch)

	// Assert
	assert.NoError(t, err)
	assert.Equal(t, 1, len(setup.mockWriter.messages))

	// Verify message content
	msg := setup.mockWriter.messages[0]
	assert.Equal(t, []byte(string(models.ProviderOptiOdds)), msg.Key)

	var kafkaMsg models.KafkaOddsMessage
	err = json.Unmarshal(msg.Value, &kafkaMsg)
	require.NoError(t, err)

	assert.Equal(t, models.ProviderOptiOdds, kafkaMsg.Provider)
	assert.Equal(t, 2, len(kafkaMsg.OddsData))
	assert.NotEmpty(t, kafkaMsg.BatchID)
}

// TestOddsAggregator_PublishBatch_EmptyBatch tests publishing empty batch
func TestOddsAggregator_PublishBatch_EmptyBatch(t *testing.T) {
	setup := setupTestAggregator(t, 10, 1*time.Second)
	defer setup.cleanup()

	ctx := context.Background()

	// Execute with empty batch
	err := setup.aggregator.publishBatch(ctx, []*models.RawOdds{})

	// Assert
	assert.NoError(t, err)
	assert.Equal(t, 0, len(setup.mockWriter.messages))
}

// TestOddsAggregator_PublishBatch_MultipleProviders tests batch grouping by provider
func TestOddsAggregator_PublishBatch_MultipleProviders(t *testing.T) {
	setup := setupTestAggregator(t, 10, 1*time.Second)
	defer setup.cleanup()

	ctx := context.Background()

	// Create odds from different providers
	optiOdds := &models.RawOdds{
		ID:          uuid.New(),
		Provider:    models.ProviderOptiOdds,
		EventID:     "event-opti",
		EventName:   "OptiOdds Event",
		Sport:       "football",
		Competition: "Test",
		Market:      models.MarketTypeMatchOdds,
		Selection:   "Team A",
		BackPrice:   decimal.NewFromFloat(2.00),
		LayPrice:    decimal.Zero,
		BackSize:    decimal.NewFromFloat(100.00),
		LaySize:     decimal.Zero,
		Timestamp:   time.Now().UTC(),
		ReceivedAt:  time.Now().UTC(),
		RawPayload:  map[string]interface{}{},
	}

	trade360Odds := &models.RawOdds{
		ID:          uuid.New(),
		Provider:    models.ProviderTrade360,
		EventID:     "event-trade",
		EventName:   "Trade360 Event",
		Sport:       "tennis",
		Competition: "Test",
		Market:      models.MarketTypeMatchOdds,
		Selection:   "Player X",
		BackPrice:   decimal.NewFromFloat(1.80),
		LayPrice:    decimal.NewFromFloat(1.85),
		BackSize:    decimal.NewFromFloat(200.00),
		LaySize:     decimal.NewFromFloat(190.00),
		Timestamp:   time.Now().UTC(),
		ReceivedAt:  time.Now().UTC(),
		RawPayload:  map[string]interface{}{},
	}

	batch := []*models.RawOdds{optiOdds, trade360Odds}

	// Execute
	err := setup.aggregator.publishBatch(ctx, batch)

	// Assert
	assert.NoError(t, err)
	assert.Equal(t, 2, len(setup.mockWriter.messages)) // One message per provider

	// Verify both providers were published
	providers := make(map[string]bool)
	for _, msg := range setup.mockWriter.messages {
		providers[string(msg.Key)] = true
	}
	assert.True(t, providers[string(models.ProviderOptiOdds)])
	assert.True(t, providers[string(models.ProviderTrade360)])
}

// TestOddsAggregator_PublishBatch_KafkaHeaders tests Kafka message headers
func TestOddsAggregator_PublishBatch_KafkaHeaders(t *testing.T) {
	setup := setupTestAggregator(t, 10, 1*time.Second)
	defer setup.cleanup()

	ctx := context.Background()

	odds := &models.RawOdds{
		ID:          uuid.New(),
		Provider:    models.ProviderOptiOdds,
		EventID:     "event-headers",
		EventName:   "Headers Test",
		Sport:       "football",
		Competition: "Test",
		Market:      models.MarketTypeMatchOdds,
		Selection:   "Team A",
		BackPrice:   decimal.NewFromFloat(2.00),
		LayPrice:    decimal.Zero,
		BackSize:    decimal.NewFromFloat(100.00),
		LaySize:     decimal.Zero,
		Timestamp:   time.Now().UTC(),
		ReceivedAt:  time.Now().UTC(),
		RawPayload:  map[string]interface{}{},
	}

	batch := []*models.RawOdds{odds}

	// Execute
	err := setup.aggregator.publishBatch(ctx, batch)

	// Assert
	assert.NoError(t, err)
	require.Equal(t, 1, len(setup.mockWriter.messages))

	msg := setup.mockWriter.messages[0]
	assert.Equal(t, 3, len(msg.Headers))

	// Verify headers
	headers := make(map[string]string)
	for _, h := range msg.Headers {
		headers[h.Key] = string(h.Value)
	}

	assert.Equal(t, string(models.ProviderOptiOdds), headers["provider"])
	assert.NotEmpty(t, headers["batch_id"])
	assert.NotEmpty(t, headers["timestamp"])
}

// TestOddsAggregator_Start_BatchSizeTrigger tests batch publishing when size reached
func TestOddsAggregator_Start_BatchSizeTrigger(t *testing.T) {
	setup := setupTestAggregator(t, 3, 10*time.Second) // Small batch size, long timeout
	defer setup.cleanup()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start aggregator in goroutine
	done := make(chan error)
	go func() {
		done <- setup.aggregator.Start(ctx)
	}()

	// Send 3 odds to trigger batch
	for i := 0; i < 3; i++ {
		odds := &models.RawOdds{
			ID:          uuid.New(),
			Provider:    models.ProviderOptiOdds,
			EventID:     "event-size",
			EventName:   "Size Trigger Test",
			Sport:       "football",
			Competition: "Test",
			Market:      models.MarketTypeMatchOdds,
			Selection:   "Team A",
			BackPrice:   decimal.NewFromFloat(2.00),
			LayPrice:    decimal.Zero,
			BackSize:    decimal.NewFromFloat(100.00),
			LaySize:     decimal.Zero,
			Timestamp:   time.Now().UTC(),
			ReceivedAt:  time.Now().UTC(),
			RawPayload:  map[string]interface{}{},
		}
		setup.oddsInput <- odds
	}

	// Wait for batch to be published
	time.Sleep(100 * time.Millisecond)

	// Stop aggregator
	cancel()

	// Wait for graceful shutdown
	select {
	case err := <-done:
		assert.NoError(t, err)
	case <-time.After(1 * time.Second):
		t.Fatal("aggregator did not stop gracefully")
	}

	// Verify batch was published
	assert.GreaterOrEqual(t, len(setup.mockWriter.messages), 1)
}

// TestOddsAggregator_Start_TimeoutTrigger tests batch publishing on timeout
func TestOddsAggregator_Start_TimeoutTrigger(t *testing.T) {
	setup := setupTestAggregator(t, 100, 100*time.Millisecond) // Large batch size, short timeout
	defer setup.cleanup()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start aggregator in goroutine
	done := make(chan error)
	go func() {
		done <- setup.aggregator.Start(ctx)
	}()

	// Send single odds (below batch size)
	odds := &models.RawOdds{
		ID:          uuid.New(),
		Provider:    models.ProviderOptiOdds,
		EventID:     "event-timeout",
		EventName:   "Timeout Trigger Test",
		Sport:       "football",
		Competition: "Test",
		Market:      models.MarketTypeMatchOdds,
		Selection:   "Team A",
		BackPrice:   decimal.NewFromFloat(2.00),
		LayPrice:    decimal.Zero,
		BackSize:    decimal.NewFromFloat(100.00),
		LaySize:     decimal.Zero,
		Timestamp:   time.Now().UTC(),
		ReceivedAt:  time.Now().UTC(),
		RawPayload:  map[string]interface{}{},
	}
	setup.oddsInput <- odds

	// Wait for timeout to trigger
	time.Sleep(200 * time.Millisecond)

	// Stop aggregator
	cancel()

	// Wait for graceful shutdown
	select {
	case err := <-done:
		assert.NoError(t, err)
	case <-time.After(1 * time.Second):
		t.Fatal("aggregator did not stop gracefully")
	}

	// Verify batch was published due to timeout
	assert.GreaterOrEqual(t, len(setup.mockWriter.messages), 1)
}

// TestOddsAggregator_Start_ContextCancellation tests graceful shutdown
func TestOddsAggregator_Start_ContextCancellation(t *testing.T) {
	setup := setupTestAggregator(t, 10, 1*time.Second)
	defer setup.cleanup()

	ctx, cancel := context.WithCancel(context.Background())

	// Start aggregator in goroutine
	done := make(chan error)
	go func() {
		done <- setup.aggregator.Start(ctx)
	}()

	// Cancel immediately
	time.Sleep(50 * time.Millisecond)
	cancel()

	// Wait for graceful shutdown
	select {
	case err := <-done:
		assert.NoError(t, err)
	case <-time.After(1 * time.Second):
		t.Fatal("aggregator did not stop gracefully")
	}

	// Verify Kafka writer was closed
	assert.True(t, setup.mockWriter.closed)
}

// TestOddsAggregator_Start_FlushOnShutdown tests flushing remaining batch on shutdown
func TestOddsAggregator_Start_FlushOnShutdown(t *testing.T) {
	setup := setupTestAggregator(t, 100, 10*time.Second) // Large batch size and timeout
	defer setup.cleanup()

	ctx, cancel := context.WithCancel(context.Background())

	// Start aggregator in goroutine
	done := make(chan error)
	go func() {
		done <- setup.aggregator.Start(ctx)
	}()

	// Send a few odds (below batch size)
	for i := 0; i < 3; i++ {
		odds := &models.RawOdds{
			ID:          uuid.New(),
			Provider:    models.ProviderOptiOdds,
			EventID:     "event-flush",
			EventName:   "Flush Test",
			Sport:       "football",
			Competition: "Test",
			Market:      models.MarketTypeMatchOdds,
			Selection:   "Team A",
			BackPrice:   decimal.NewFromFloat(2.00),
			LayPrice:    decimal.Zero,
			BackSize:    decimal.NewFromFloat(100.00),
			LaySize:     decimal.Zero,
			Timestamp:   time.Now().UTC(),
			ReceivedAt:  time.Now().UTC(),
			RawPayload:  map[string]interface{}{},
		}
		setup.oddsInput <- odds
	}

	// Wait a bit for odds to be received
	time.Sleep(50 * time.Millisecond)

	// Cancel to trigger shutdown
	cancel()

	// Wait for graceful shutdown
	select {
	case err := <-done:
		assert.NoError(t, err)
	case <-time.After(1 * time.Second):
		t.Fatal("aggregator did not stop gracefully")
	}

	// Verify batch was flushed
	assert.Equal(t, 1, len(setup.mockWriter.messages))

	// Verify all 3 odds were published
	var kafkaMsg models.KafkaOddsMessage
	err := json.Unmarshal(setup.mockWriter.messages[0].Value, &kafkaMsg)
	require.NoError(t, err)
	assert.Equal(t, 3, len(kafkaMsg.OddsData))
}

// TestOddsAggregator_Start_ChannelClosed tests handling of closed input channel
func TestOddsAggregator_Start_ChannelClosed(t *testing.T) {
	setup := setupTestAggregator(t, 10, 1*time.Second)

	ctx := context.Background()

	// Close input channel
	close(setup.oddsInput)

	// Start aggregator
	err := setup.aggregator.Start(ctx)

	// Assert - should return without error when channel closes
	assert.NoError(t, err)
}

// TestOddsAggregator_PublishBatch_LargeBatch tests publishing large batch
func TestOddsAggregator_PublishBatch_LargeBatch(t *testing.T) {
	setup := setupTestAggregator(t, 1000, 1*time.Second)
	defer setup.cleanup()

	ctx := context.Background()

	// Create 500 odds
	batch := make([]*models.RawOdds, 500)
	for i := 0; i < 500; i++ {
		batch[i] = &models.RawOdds{
			ID:          uuid.New(),
			Provider:    models.ProviderOptiOdds,
			EventID:     "event-large",
			EventName:   "Large Batch Test",
			Sport:       "football",
			Competition: "Test",
			Market:      models.MarketTypeMatchOdds,
			Selection:   "Team A",
			BackPrice:   decimal.NewFromFloat(2.00),
			LayPrice:    decimal.Zero,
			BackSize:    decimal.NewFromFloat(100.00),
			LaySize:     decimal.Zero,
			Timestamp:   time.Now().UTC(),
			ReceivedAt:  time.Now().UTC(),
			RawPayload:  map[string]interface{}{},
		}
	}

	// Execute
	err := setup.aggregator.publishBatch(ctx, batch)

	// Assert
	assert.NoError(t, err)
	assert.Equal(t, 1, len(setup.mockWriter.messages))

	// Verify all odds were published
	var kafkaMsg models.KafkaOddsMessage
	err = json.Unmarshal(setup.mockWriter.messages[0].Value, &kafkaMsg)
	require.NoError(t, err)
	assert.Equal(t, 500, len(kafkaMsg.OddsData))
}

// TestOddsAggregator_PublishBatch_MessageStructure tests Kafka message structure
func TestOddsAggregator_PublishBatch_MessageStructure(t *testing.T) {
	setup := setupTestAggregator(t, 10, 1*time.Second)
	defer setup.cleanup()

	ctx := context.Background()

	odds := &models.RawOdds{
		ID:          uuid.New(),
		Provider:    models.ProviderTrade360,
		EventID:     "event-structure",
		EventName:   "Structure Test",
		Sport:       "football",
		Competition: "Test League",
		Market:      models.MarketTypeMatchOdds,
		Selection:   "Team A",
		BackPrice:   decimal.NewFromFloat(2.50),
		LayPrice:    decimal.NewFromFloat(2.55),
		BackSize:    decimal.NewFromFloat(1000.00),
		LaySize:     decimal.NewFromFloat(950.00),
		Timestamp:   time.Now().UTC(),
		ReceivedAt:  time.Now().UTC(),
		RawPayload:  map[string]interface{}{"test": "data"},
	}

	batch := []*models.RawOdds{odds}

	// Execute
	err := setup.aggregator.publishBatch(ctx, batch)

	// Assert
	assert.NoError(t, err)
	require.Equal(t, 1, len(setup.mockWriter.messages))

	// Parse message
	var kafkaMsg models.KafkaOddsMessage
	err = json.Unmarshal(setup.mockWriter.messages[0].Value, &kafkaMsg)
	require.NoError(t, err)

	// Verify structure
	assert.Equal(t, models.ProviderTrade360, kafkaMsg.Provider)
	assert.NotEmpty(t, kafkaMsg.BatchID)
	assert.False(t, kafkaMsg.Timestamp.IsZero())
	assert.Equal(t, 1, len(kafkaMsg.OddsData))

	// Verify odds data
	publishedOdds := kafkaMsg.OddsData[0]
	assert.Equal(t, odds.ID, publishedOdds.ID)
	assert.Equal(t, odds.EventID, publishedOdds.EventID)
	assert.Equal(t, odds.EventName, publishedOdds.EventName)
	assert.Equal(t, odds.Sport, publishedOdds.Sport)
	assert.Equal(t, odds.Competition, publishedOdds.Competition)
	assert.Equal(t, odds.Market, publishedOdds.Market)
	assert.Equal(t, odds.Selection, publishedOdds.Selection)
	assert.True(t, publishedOdds.BackPrice.Equal(odds.BackPrice))
	assert.True(t, publishedOdds.LayPrice.Equal(odds.LayPrice))
}

// TestOddsAggregator_Start_ContinueOnPublishError tests aggregator continues after publish error
func TestOddsAggregator_Start_ContinueOnPublishError(t *testing.T) {
	setup := setupTestAggregator(t, 2, 10*time.Second)
	defer setup.cleanup()

	// Set mock to return error on first write
	callCount := 0
	originalWriteErr := setup.mockWriter.writeErr
	setup.mockWriter.writeErr = assert.AnError

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start aggregator in goroutine
	done := make(chan error)
	go func() {
		done <- setup.aggregator.Start(ctx)
	}()

	// Send first batch (should fail)
	for i := 0; i < 2; i++ {
		odds := &models.RawOdds{
			ID:          uuid.New(),
			Provider:    models.ProviderOptiOdds,
			EventID:     "event-error1",
			EventName:   "Error Test 1",
			Sport:       "football",
			Competition: "Test",
			Market:      models.MarketTypeMatchOdds,
			Selection:   "Team A",
			BackPrice:   decimal.NewFromFloat(2.00),
			LayPrice:    decimal.Zero,
			BackSize:    decimal.NewFromFloat(100.00),
			LaySize:     decimal.Zero,
			Timestamp:   time.Now().UTC(),
			ReceivedAt:  time.Now().UTC(),
			RawPayload:  map[string]interface{}{},
		}
		setup.oddsInput <- odds
		callCount++
	}

	time.Sleep(100 * time.Millisecond)

	// Clear error for next batch
	setup.mockWriter.writeErr = originalWriteErr

	// Send second batch (should succeed)
	for i := 0; i < 2; i++ {
		odds := &models.RawOdds{
			ID:          uuid.New(),
			Provider:    models.ProviderOptiOdds,
			EventID:     "event-error2",
			EventName:   "Error Test 2",
			Sport:       "football",
			Competition: "Test",
			Market:      models.MarketTypeMatchOdds,
			Selection:   "Team A",
			BackPrice:   decimal.NewFromFloat(2.00),
			LayPrice:    decimal.Zero,
			BackSize:    decimal.NewFromFloat(100.00),
			LaySize:     decimal.Zero,
			Timestamp:   time.Now().UTC(),
			ReceivedAt:  time.Now().UTC(),
			RawPayload:  map[string]interface{}{},
		}
		setup.oddsInput <- odds
	}

	time.Sleep(100 * time.Millisecond)

	// Stop aggregator
	cancel()

	// Wait for graceful shutdown
	select {
	case err := <-done:
		assert.NoError(t, err)
	case <-time.After(1 * time.Second):
		t.Fatal("aggregator did not stop gracefully")
	}

	// Verify aggregator continued after error
	// The failed batch stays in memory, second batch gets published
	assert.GreaterOrEqual(t, len(setup.mockWriter.messages), 1)
}
