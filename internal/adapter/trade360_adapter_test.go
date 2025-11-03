package adapter

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/cypherlabdev/odds-adapter-service/internal/models"
	"github.com/rs/zerolog"
	"github.com/shopspring/decimal"
	"github.com/streadway/amqp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// testTrade360Setup is a helper struct to hold test dependencies
type testTrade360Setup struct {
	oddsOutput chan *models.RawOdds
}

// setupTestTrade360Adapter creates a test adapter setup
func setupTestTrade360Adapter(t *testing.T) *testTrade360Setup {
	oddsOutput := make(chan *models.RawOdds, 100)

	return &testTrade360Setup{
		oddsOutput: oddsOutput,
	}
}

// cleanup cleans up test resources
func (s *testTrade360Setup) cleanup() {
	close(s.oddsOutput)
}

// TestNewTrade360Adapter_InvalidURL tests adapter creation with invalid URL
func TestNewTrade360Adapter_InvalidURL(t *testing.T) {
	oddsOutput := make(chan *models.RawOdds, 100)
	logger := zerolog.Nop()

	config := Trade360Config{
		URL:       "invalid://url",
		QueueName: "test_queue",
	}

	adapter, err := NewTrade360Adapter(config, oddsOutput, logger)

	// Assert
	assert.Error(t, err)
	assert.Nil(t, adapter)
}

// TestTrade360Adapter_ProcessMessage_Success tests successful message processing
func TestTrade360Adapter_ProcessMessage_Success(t *testing.T) {
	setup := setupTestTrade360Adapter(t)
	defer setup.cleanup()

	logger := zerolog.Nop()

	// Note: We cannot create a real Trade360Adapter without RabbitMQ connection
	// So we'll test the market type mapping and message processing logic separately

	// Create mock Trade360 data
	data := models.Trade360OddsData{
		EventID:     "trade-event-123",
		EventName:   "Team X vs Team Y",
		Sport:       "football",
		Competition: "Championship",
		Timestamp:   time.Now().UTC().Format(time.RFC3339),
		Markets: []models.Trade360Market{
			{
				MarketType: "match_result",
				Selections: []models.Trade360Selection{
					{
						Name:      "Team X",
						BackPrice: 2.50,
						LayPrice:  2.60,
						BackSize:  1000.00,
						LaySize:   900.00,
					},
					{
						Name:      "Draw",
						BackPrice: 3.20,
						LayPrice:  3.30,
						BackSize:  800.00,
						LaySize:   750.00,
					},
				},
			},
		},
	}

	// Marshal to JSON
	payload, err := json.Marshal(data)
	require.NoError(t, err)

	// Create mock AMQP delivery
	msg := amqp.Delivery{
		Body:      payload,
		MessageId: "msg-123",
	}

	// Create adapter with mock connection (we'll only test processMessage logic)
	// Since we can't mock RabbitMQ connection easily, we test the core logic
	adapter := &Trade360Adapter{
		oddsOutput: setup.oddsOutput,
		logger:     logger.With().Str("component", "trade360_adapter").Logger(),
	}

	// Execute
	err = adapter.processMessage(msg)

	// Assert
	assert.NoError(t, err)

	// Verify odds were sent (2 selections)
	receivedCount := 0
	timeout := time.After(1 * time.Second)

	for receivedCount < 2 {
		select {
		case odds := <-setup.oddsOutput:
			assert.NotNil(t, odds)
			assert.Equal(t, models.ProviderTrade360, odds.Provider)
			assert.Equal(t, "trade-event-123", odds.EventID)
			assert.Equal(t, "Team X vs Team Y", odds.EventName)
			assert.Equal(t, "football", odds.Sport)
			assert.Equal(t, "Championship", odds.Competition)
			assert.Equal(t, models.MarketTypeMatchOdds, odds.Market)
			assert.False(t, odds.BackPrice.IsZero())
			assert.False(t, odds.LayPrice.IsZero())
			receivedCount++
		case <-timeout:
			t.Fatalf("timeout waiting for odds, got %d/2", receivedCount)
		}
	}
}

// TestTrade360Adapter_ProcessMessage_InvalidJSON tests handling of invalid JSON
func TestTrade360Adapter_ProcessMessage_InvalidJSON(t *testing.T) {
	setup := setupTestTrade360Adapter(t)
	defer setup.cleanup()

	logger := zerolog.Nop()

	msg := amqp.Delivery{
		Body:      []byte("invalid json"),
		MessageId: "msg-invalid",
	}

	adapter := &Trade360Adapter{
		oddsOutput: setup.oddsOutput,
		logger:     logger.With().Str("component", "trade360_adapter").Logger(),
	}

	// Execute
	err := adapter.processMessage(msg)

	// Assert
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to unmarshal message")
}

// TestTrade360Adapter_ProcessMessage_InvalidTimestamp tests handling of invalid timestamp
func TestTrade360Adapter_ProcessMessage_InvalidTimestamp(t *testing.T) {
	setup := setupTestTrade360Adapter(t)
	defer setup.cleanup()

	logger := zerolog.Nop()

	data := models.Trade360OddsData{
		EventID:     "trade-event-time",
		EventName:   "Time Test Event",
		Sport:       "tennis",
		Competition: "Test Open",
		Timestamp:   "invalid-timestamp",
		Markets: []models.Trade360Market{
			{
				MarketType: "match_result",
				Selections: []models.Trade360Selection{
					{
						Name:      "Player A",
						BackPrice: 1.80,
						LayPrice:  1.85,
						BackSize:  500.00,
						LaySize:   450.00,
					},
				},
			},
		},
	}

	payload, err := json.Marshal(data)
	require.NoError(t, err)

	msg := amqp.Delivery{
		Body:      payload,
		MessageId: "msg-time",
	}

	adapter := &Trade360Adapter{
		oddsOutput: setup.oddsOutput,
		logger:     logger.With().Str("component", "trade360_adapter").Logger(),
	}

	// Execute - should not error, but use current time
	err = adapter.processMessage(msg)

	// Assert
	assert.NoError(t, err)

	// Verify odds were still sent
	select {
	case odds := <-setup.oddsOutput:
		assert.NotNil(t, odds)
		assert.Equal(t, "trade-event-time", odds.EventID)
	case <-time.After(1 * time.Second):
		t.Fatal("timeout waiting for odds")
	}
}

// TestTrade360Adapter_ProcessMessage_MultipleMarkets tests processing multiple markets
func TestTrade360Adapter_ProcessMessage_MultipleMarkets(t *testing.T) {
	setup := setupTestTrade360Adapter(t)
	defer setup.cleanup()

	logger := zerolog.Nop()

	data := models.Trade360OddsData{
		EventID:     "trade-multi",
		EventName:   "Multi Market Event",
		Sport:       "football",
		Competition: "Test League",
		Timestamp:   time.Now().UTC().Format(time.RFC3339),
		Markets: []models.Trade360Market{
			{
				MarketType: "match_result",
				Selections: []models.Trade360Selection{
					{Name: "Home", BackPrice: 2.00, LayPrice: 2.05, BackSize: 100.00, LaySize: 95.00},
				},
			},
			{
				MarketType: "over_under",
				Selections: []models.Trade360Selection{
					{Name: "Over 2.5", BackPrice: 1.90, LayPrice: 1.95, BackSize: 200.00, LaySize: 190.00},
					{Name: "Under 2.5", BackPrice: 2.10, LayPrice: 2.15, BackSize: 180.00, LaySize: 175.00},
				},
			},
		},
	}

	payload, err := json.Marshal(data)
	require.NoError(t, err)

	msg := amqp.Delivery{
		Body:      payload,
		MessageId: "msg-multi",
	}

	adapter := &Trade360Adapter{
		oddsOutput: setup.oddsOutput,
		logger:     logger.With().Str("component", "trade360_adapter").Logger(),
	}

	// Execute
	err = adapter.processMessage(msg)

	// Assert
	assert.NoError(t, err)

	// Verify we got 3 odds messages (1 from first market, 2 from second)
	receivedCount := 0
	timeout := time.After(1 * time.Second)

	for receivedCount < 3 {
		select {
		case odds := <-setup.oddsOutput:
			assert.NotNil(t, odds)
			assert.Equal(t, "trade-multi", odds.EventID)
			receivedCount++
		case <-timeout:
			t.Fatalf("timeout waiting for odds, got %d/3", receivedCount)
		}
	}
}

// TestTrade360Adapter_ProcessMessage_EmptyMarkets tests handling of empty markets
func TestTrade360Adapter_ProcessMessage_EmptyMarkets(t *testing.T) {
	setup := setupTestTrade360Adapter(t)
	defer setup.cleanup()

	logger := zerolog.Nop()

	data := models.Trade360OddsData{
		EventID:     "trade-empty",
		EventName:   "Empty Markets Event",
		Sport:       "football",
		Competition: "Test",
		Timestamp:   time.Now().UTC().Format(time.RFC3339),
		Markets:     []models.Trade360Market{},
	}

	payload, err := json.Marshal(data)
	require.NoError(t, err)

	msg := amqp.Delivery{
		Body:      payload,
		MessageId: "msg-empty",
	}

	adapter := &Trade360Adapter{
		oddsOutput: setup.oddsOutput,
		logger:     logger.With().Str("component", "trade360_adapter").Logger(),
	}

	// Execute
	err = adapter.processMessage(msg)

	// Assert
	assert.NoError(t, err)

	// Verify no odds were sent
	select {
	case <-setup.oddsOutput:
		t.Fatal("unexpected odds received")
	case <-time.After(100 * time.Millisecond):
		// Expected - no odds should be sent
	}
}

// TestTrade360Adapter_ProcessMessage_ChannelFull tests handling of full output channel
func TestTrade360Adapter_ProcessMessage_ChannelFull(t *testing.T) {
	// Create adapter with 0 capacity channel
	oddsOutput := make(chan *models.RawOdds)
	logger := zerolog.Nop()

	data := models.Trade360OddsData{
		EventID:     "trade-full",
		EventName:   "Full Channel Test",
		Sport:       "football",
		Competition: "Test",
		Timestamp:   time.Now().UTC().Format(time.RFC3339),
		Markets: []models.Trade360Market{
			{
				MarketType: "match_result",
				Selections: []models.Trade360Selection{
					{Name: "Team", BackPrice: 2.00, LayPrice: 2.05, BackSize: 100.00, LaySize: 95.00},
				},
			},
		},
	}

	payload, err := json.Marshal(data)
	require.NoError(t, err)

	msg := amqp.Delivery{
		Body:      payload,
		MessageId: "msg-full",
	}

	adapter := &Trade360Adapter{
		oddsOutput: oddsOutput,
		logger:     logger.With().Str("component", "trade360_adapter").Logger(),
	}

	// Execute - should not block even if channel is full
	err = adapter.processMessage(msg)

	// Assert - should succeed without blocking
	assert.NoError(t, err)
}

// TestTrade360Adapter_ProcessMessage_DecimalPrecision tests decimal precision handling
func TestTrade360Adapter_ProcessMessage_DecimalPrecision(t *testing.T) {
	setup := setupTestTrade360Adapter(t)
	defer setup.cleanup()

	logger := zerolog.Nop()

	data := models.Trade360OddsData{
		EventID:     "trade-decimal",
		EventName:   "Decimal Test",
		Sport:       "football",
		Competition: "Test",
		Timestamp:   time.Now().UTC().Format(time.RFC3339),
		Markets: []models.Trade360Market{
			{
				MarketType: "match_result",
				Selections: []models.Trade360Selection{
					{
						Name:      "Team A",
						BackPrice: 2.5555,
						LayPrice:  2.5666,
						BackSize:  1000.123,
						LaySize:   999.987,
					},
				},
			},
		},
	}

	payload, err := json.Marshal(data)
	require.NoError(t, err)

	msg := amqp.Delivery{
		Body:      payload,
		MessageId: "msg-decimal",
	}

	adapter := &Trade360Adapter{
		oddsOutput: setup.oddsOutput,
		logger:     logger.With().Str("component", "trade360_adapter").Logger(),
	}

	// Execute
	err = adapter.processMessage(msg)

	// Assert
	assert.NoError(t, err)

	// Verify decimal precision
	select {
	case odds := <-setup.oddsOutput:
		assert.NotNil(t, odds)
		assert.True(t, odds.BackPrice.Equal(decimal.NewFromFloat(2.5555)))
		assert.True(t, odds.LayPrice.Equal(decimal.NewFromFloat(2.5666)))
		assert.True(t, odds.BackSize.Equal(decimal.NewFromFloat(1000.123)))
		assert.True(t, odds.LaySize.Equal(decimal.NewFromFloat(999.987)))
	case <-time.After(1 * time.Second):
		t.Fatal("timeout waiting for odds")
	}
}

// TestMapTrade360MarketType tests market type mapping
func TestMapTrade360MarketType(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected models.MarketType
	}{
		{
			name:     "match_result maps to match_odds",
			input:    "match_result",
			expected: models.MarketTypeMatchOdds,
		},
		{
			name:     "match_odds maps to match_odds",
			input:    "match_odds",
			expected: models.MarketTypeMatchOdds,
		},
		{
			name:     "1x2 maps to match_odds",
			input:    "1x2",
			expected: models.MarketTypeMatchOdds,
		},
		{
			name:     "over_under maps to over_under",
			input:    "over_under",
			expected: models.MarketTypeOverUnder,
		},
		{
			name:     "totals maps to over_under",
			input:    "totals",
			expected: models.MarketTypeOverUnder,
		},
		{
			name:     "handicap maps to handicap",
			input:    "handicap",
			expected: models.MarketTypeHandicap,
		},
		{
			name:     "asian_handicap maps to handicap",
			input:    "asian_handicap",
			expected: models.MarketTypeHandicap,
		},
		{
			name:     "btts maps to both_teams",
			input:    "btts",
			expected: models.MarketTypeBothTeams,
		},
		{
			name:     "both_teams_to_score maps to both_teams",
			input:    "both_teams_to_score",
			expected: models.MarketTypeBothTeams,
		},
		{
			name:     "unknown market type passes through",
			input:    "custom_market",
			expected: models.MarketType("custom_market"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := mapTrade360MarketType(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestTrade360Adapter_ProcessMessage_RawPayload tests raw payload preservation
func TestTrade360Adapter_ProcessMessage_RawPayload(t *testing.T) {
	setup := setupTestTrade360Adapter(t)
	defer setup.cleanup()

	logger := zerolog.Nop()

	data := models.Trade360OddsData{
		EventID:     "trade-raw",
		EventName:   "Raw Payload Test",
		Sport:       "football",
		Competition: "Test",
		Timestamp:   time.Now().UTC().Format(time.RFC3339),
		Markets: []models.Trade360Market{
			{
				MarketType: "match_result",
				Selections: []models.Trade360Selection{
					{
						Name:      "Team A",
						BackPrice: 2.00,
						LayPrice:  2.05,
						BackSize:  100.00,
						LaySize:   95.00,
					},
				},
			},
		},
	}

	payload, err := json.Marshal(data)
	require.NoError(t, err)

	msg := amqp.Delivery{
		Body:      payload,
		MessageId: "msg-raw",
	}

	adapter := &Trade360Adapter{
		oddsOutput: setup.oddsOutput,
		logger:     logger.With().Str("component", "trade360_adapter").Logger(),
	}

	// Execute
	err = adapter.processMessage(msg)

	// Assert
	assert.NoError(t, err)

	// Verify raw payload was preserved
	select {
	case odds := <-setup.oddsOutput:
		assert.NotNil(t, odds)
		assert.NotNil(t, odds.RawPayload)
		assert.Contains(t, odds.RawPayload, "original")
		assert.Contains(t, odds.RawPayload, "market")
		assert.Contains(t, odds.RawPayload, "selection")
	case <-time.After(1 * time.Second):
		t.Fatal("timeout waiting for odds")
	}
}

// TestTrade360Adapter_ProcessMessage_ZeroPrices tests handling of zero prices
func TestTrade360Adapter_ProcessMessage_ZeroPrices(t *testing.T) {
	setup := setupTestTrade360Adapter(t)
	defer setup.cleanup()

	logger := zerolog.Nop()

	data := models.Trade360OddsData{
		EventID:     "trade-zero",
		EventName:   "Zero Price Test",
		Sport:       "football",
		Competition: "Test",
		Timestamp:   time.Now().UTC().Format(time.RFC3339),
		Markets: []models.Trade360Market{
			{
				MarketType: "match_result",
				Selections: []models.Trade360Selection{
					{
						Name:      "Team A",
						BackPrice: 0.00,
						LayPrice:  0.00,
						BackSize:  0.00,
						LaySize:   0.00,
					},
				},
			},
		},
	}

	payload, err := json.Marshal(data)
	require.NoError(t, err)

	msg := amqp.Delivery{
		Body:      payload,
		MessageId: "msg-zero",
	}

	adapter := &Trade360Adapter{
		oddsOutput: setup.oddsOutput,
		logger:     logger.With().Str("component", "trade360_adapter").Logger(),
	}

	// Execute
	err = adapter.processMessage(msg)

	// Assert
	assert.NoError(t, err)

	// Verify odds were sent with zero values
	select {
	case odds := <-setup.oddsOutput:
		assert.NotNil(t, odds)
		assert.True(t, odds.BackPrice.IsZero())
		assert.True(t, odds.LayPrice.IsZero())
		assert.True(t, odds.BackSize.IsZero())
		assert.True(t, odds.LaySize.IsZero())
	case <-time.After(1 * time.Second):
		t.Fatal("timeout waiting for odds")
	}
}

// TestTrade360Adapter_ProcessMessage_LargeBatch tests handling of large batch
func TestTrade360Adapter_ProcessMessage_LargeBatch(t *testing.T) {
	setup := setupTestTrade360Adapter(t)
	defer setup.cleanup()

	logger := zerolog.Nop()

	// Create data with many selections
	selections := make([]models.Trade360Selection, 100)
	for i := 0; i < 100; i++ {
		selections[i] = models.Trade360Selection{
			Name:      "Selection " + string(rune(i)),
			BackPrice: 2.00,
			LayPrice:  2.05,
			BackSize:  100.00,
			LaySize:   95.00,
		}
	}

	data := models.Trade360OddsData{
		EventID:     "trade-large",
		EventName:   "Large Batch Test",
		Sport:       "football",
		Competition: "Test",
		Timestamp:   time.Now().UTC().Format(time.RFC3339),
		Markets: []models.Trade360Market{
			{
				MarketType: "match_result",
				Selections: selections,
			},
		},
	}

	payload, err := json.Marshal(data)
	require.NoError(t, err)

	msg := amqp.Delivery{
		Body:      payload,
		MessageId: "msg-large",
	}

	adapter := &Trade360Adapter{
		oddsOutput: setup.oddsOutput,
		logger:     logger.With().Str("component", "trade360_adapter").Logger(),
	}

	// Execute
	err = adapter.processMessage(msg)

	// Assert
	assert.NoError(t, err)

	// Verify all 100 odds were sent
	receivedCount := 0
	timeout := time.After(2 * time.Second)

	for receivedCount < 100 {
		select {
		case odds := <-setup.oddsOutput:
			assert.NotNil(t, odds)
			assert.Equal(t, "trade-large", odds.EventID)
			receivedCount++
		case <-timeout:
			t.Fatalf("timeout waiting for odds, got %d/100", receivedCount)
		}
	}
}
