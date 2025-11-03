package adapter

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/cypherlabdev/odds-adapter-service/internal/models"
	"github.com/rs/zerolog"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// testOptiOddsSetup is a helper struct to hold test dependencies
type testOptiOddsSetup struct {
	adapter    *OptiOddsAdapter
	oddsOutput chan *models.RawOdds
	server     *httptest.Server
}

// setupTestOptiOddsAdapter creates a test adapter with mock HTTP server
func setupTestOptiOddsAdapter(t *testing.T, handler http.HandlerFunc) *testOptiOddsSetup {
	server := httptest.NewServer(handler)

	oddsOutput := make(chan *models.RawOdds, 100)
	logger := zerolog.Nop()

	config := OptiOddsConfig{
		BaseURL:      server.URL,
		APIKey:       "test-api-key",
		PollInterval: 100 * time.Millisecond,
	}

	adapter := NewOptiOddsAdapter(config, oddsOutput, logger)

	return &testOptiOddsSetup{
		adapter:    adapter,
		oddsOutput: oddsOutput,
		server:     server,
	}
}

// cleanup cleans up test resources
func (s *testOptiOddsSetup) cleanup() {
	s.server.Close()
	close(s.oddsOutput)
}

// TestNewOptiOddsAdapter_Success tests successful adapter creation
func TestNewOptiOddsAdapter_Success(t *testing.T) {
	oddsOutput := make(chan *models.RawOdds, 100)
	logger := zerolog.Nop()

	config := OptiOddsConfig{
		BaseURL:      "https://api.optiodds.com/v1",
		APIKey:       "test-key",
		PollInterval: 5 * time.Second,
	}

	adapter := NewOptiOddsAdapter(config, oddsOutput, logger)

	assert.NotNil(t, adapter)
	assert.Equal(t, config.BaseURL, adapter.baseURL)
	assert.Equal(t, config.APIKey, adapter.apiKey)
	assert.Equal(t, config.PollInterval, adapter.pollInterval)
	assert.NotNil(t, adapter.client)
}

// TestOptiOddsAdapter_Poll_Success tests successful API polling
func TestOptiOddsAdapter_Poll_Success(t *testing.T) {
	// Create mock response
	mockResponse := models.OptiOddsResponse{
		Status: "success",
		Data: []models.OptiOddsEvent{
			{
				EventID:     "event-123",
				EventName:   "Team A vs Team B",
				Sport:       "football",
				Competition: "Premier League",
				StartTime:   time.Now().UTC().Format(time.RFC3339),
				Markets: []models.OptiOddsMarket{
					{
						MarketType: "match_winner",
						Outcomes: []models.OptiOddsOutcome{
							{
								Name:  "Team A",
								Price: 2.50,
								Size:  1000.00,
							},
							{
								Name:  "Team B",
								Price: 3.20,
								Size:  800.00,
							},
						},
					},
				},
			},
		},
	}

	// Setup mock server
	handler := func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "GET", r.Method)
		assert.Equal(t, "/odds", r.URL.Path)
		assert.Equal(t, "Bearer test-api-key", r.Header.Get("Authorization"))
		assert.Equal(t, "application/json", r.Header.Get("Content-Type"))

		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(mockResponse)
	}

	setup := setupTestOptiOddsAdapter(t, handler)
	defer setup.cleanup()

	ctx := context.Background()

	// Execute poll
	err := setup.adapter.poll(ctx)

	// Assert
	assert.NoError(t, err)

	// Check odds were sent to channel
	timeout := time.After(1 * time.Second)
	receivedCount := 0

	for {
		select {
		case odds := <-setup.oddsOutput:
			assert.NotNil(t, odds)
			assert.Equal(t, models.ProviderOptiOdds, odds.Provider)
			assert.Equal(t, "event-123", odds.EventID)
			assert.Equal(t, "Team A vs Team B", odds.EventName)
			assert.Equal(t, "football", odds.Sport)
			assert.Equal(t, "Premier League", odds.Competition)
			assert.Equal(t, models.MarketTypeMatchOdds, odds.Market)
			receivedCount++
			if receivedCount == 2 { // We expect 2 outcomes
				return
			}
		case <-timeout:
			t.Fatal("timeout waiting for odds")
		}
	}
}

// TestOptiOddsAdapter_Poll_APIError tests handling of API error
func TestOptiOddsAdapter_Poll_APIError(t *testing.T) {
	handler := func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("Internal server error"))
	}

	setup := setupTestOptiOddsAdapter(t, handler)
	defer setup.cleanup()

	ctx := context.Background()

	// Execute poll
	err := setup.adapter.poll(ctx)

	// Assert
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unexpected status code 500")
}

// TestOptiOddsAdapter_Poll_Unauthorized tests handling of unauthorized access
func TestOptiOddsAdapter_Poll_Unauthorized(t *testing.T) {
	handler := func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte("Unauthorized"))
	}

	setup := setupTestOptiOddsAdapter(t, handler)
	defer setup.cleanup()

	ctx := context.Background()

	// Execute poll
	err := setup.adapter.poll(ctx)

	// Assert
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unexpected status code 401")
}

// TestOptiOddsAdapter_Poll_InvalidJSON tests handling of invalid JSON response
func TestOptiOddsAdapter_Poll_InvalidJSON(t *testing.T) {
	handler := func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("invalid json"))
	}

	setup := setupTestOptiOddsAdapter(t, handler)
	defer setup.cleanup()

	ctx := context.Background()

	// Execute poll
	err := setup.adapter.poll(ctx)

	// Assert
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to decode response")
}

// TestOptiOddsAdapter_Poll_EmptyResponse tests handling of empty response
func TestOptiOddsAdapter_Poll_EmptyResponse(t *testing.T) {
	mockResponse := models.OptiOddsResponse{
		Status: "success",
		Data:   []models.OptiOddsEvent{},
	}

	handler := func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(mockResponse)
	}

	setup := setupTestOptiOddsAdapter(t, handler)
	defer setup.cleanup()

	ctx := context.Background()

	// Execute poll
	err := setup.adapter.poll(ctx)

	// Assert
	assert.NoError(t, err)
	assert.Equal(t, 0, len(setup.oddsOutput))
}

// TestOptiOddsAdapter_Poll_ContextCancellation tests handling of context cancellation
func TestOptiOddsAdapter_Poll_ContextCancellation(t *testing.T) {
	handler := func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(100 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}

	setup := setupTestOptiOddsAdapter(t, handler)
	defer setup.cleanup()

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	// Execute poll
	err := setup.adapter.poll(ctx)

	// Assert
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "context canceled")
}

// TestOptiOddsAdapter_ProcessEvent_Success tests successful event processing
func TestOptiOddsAdapter_ProcessEvent_Success(t *testing.T) {
	setup := setupTestOptiOddsAdapter(t, nil)
	defer setup.cleanup()

	event := models.OptiOddsEvent{
		EventID:     "event-456",
		EventName:   "Match X vs Y",
		Sport:       "tennis",
		Competition: "Grand Slam",
		StartTime:   time.Now().UTC().Format(time.RFC3339),
		Markets: []models.OptiOddsMarket{
			{
				MarketType: "match_winner",
				Outcomes: []models.OptiOddsOutcome{
					{
						Name:  "Player X",
						Price: 1.80,
						Size:  500.00,
					},
				},
			},
		},
	}

	// Execute
	err := setup.adapter.processEvent(event)

	// Assert
	assert.NoError(t, err)

	// Verify odds were sent
	select {
	case odds := <-setup.oddsOutput:
		assert.NotNil(t, odds)
		assert.Equal(t, models.ProviderOptiOdds, odds.Provider)
		assert.Equal(t, "event-456", odds.EventID)
		assert.Equal(t, "Player X", odds.Selection)
		assert.True(t, odds.BackPrice.Equal(decimal.NewFromFloat(1.80)))
		assert.True(t, odds.LayPrice.IsZero())
	case <-time.After(1 * time.Second):
		t.Fatal("timeout waiting for odds")
	}
}

// TestOptiOddsAdapter_ProcessEvent_InvalidStartTime tests handling of invalid start time
func TestOptiOddsAdapter_ProcessEvent_InvalidStartTime(t *testing.T) {
	setup := setupTestOptiOddsAdapter(t, nil)
	defer setup.cleanup()

	event := models.OptiOddsEvent{
		EventID:     "event-789",
		EventName:   "Test Event",
		Sport:       "football",
		Competition: "Test League",
		StartTime:   "invalid-time-format",
		Markets: []models.OptiOddsMarket{
			{
				MarketType: "match_winner",
				Outcomes: []models.OptiOddsOutcome{
					{
						Name:  "Team A",
						Price: 2.00,
						Size:  100.00,
					},
				},
			},
		},
	}

	// Execute - should not error, but use current time
	err := setup.adapter.processEvent(event)

	// Assert
	assert.NoError(t, err)

	// Verify odds were still sent
	select {
	case odds := <-setup.oddsOutput:
		assert.NotNil(t, odds)
		assert.Equal(t, "event-789", odds.EventID)
	case <-time.After(1 * time.Second):
		t.Fatal("timeout waiting for odds")
	}
}

// TestOptiOddsAdapter_ProcessEvent_MultipleMarkets tests processing multiple markets
func TestOptiOddsAdapter_ProcessEvent_MultipleMarkets(t *testing.T) {
	setup := setupTestOptiOddsAdapter(t, nil)
	defer setup.cleanup()

	event := models.OptiOddsEvent{
		EventID:     "event-multi",
		EventName:   "Multi Market Event",
		Sport:       "football",
		Competition: "Test League",
		StartTime:   time.Now().UTC().Format(time.RFC3339),
		Markets: []models.OptiOddsMarket{
			{
				MarketType: "match_winner",
				Outcomes: []models.OptiOddsOutcome{
					{Name: "Home", Price: 2.00, Size: 100.00},
				},
			},
			{
				MarketType: "over_under",
				Outcomes: []models.OptiOddsOutcome{
					{Name: "Over 2.5", Price: 1.90, Size: 200.00},
				},
			},
		},
	}

	// Execute
	err := setup.adapter.processEvent(event)

	// Assert
	assert.NoError(t, err)

	// Verify we got 2 odds messages
	receivedCount := 0
	timeout := time.After(1 * time.Second)

	for receivedCount < 2 {
		select {
		case odds := <-setup.oddsOutput:
			assert.NotNil(t, odds)
			assert.Equal(t, "event-multi", odds.EventID)
			receivedCount++
		case <-timeout:
			t.Fatalf("timeout waiting for odds, got %d/2", receivedCount)
		}
	}
}

// TestMapOptiOddsMarketType tests market type mapping
func TestMapOptiOddsMarketType(t *testing.T) {
	tests := []struct {
		name       string
		input      string
		expected   models.MarketType
	}{
		{
			name:     "match_winner maps to match_odds",
			input:    "match_winner",
			expected: models.MarketTypeMatchOdds,
		},
		{
			name:     "moneyline maps to match_odds",
			input:    "moneyline",
			expected: models.MarketTypeMatchOdds,
		},
		{
			name:     "1x2 maps to match_odds",
			input:    "1x2",
			expected: models.MarketTypeMatchOdds,
		},
		{
			name:     "total maps to over_under",
			input:    "total",
			expected: models.MarketTypeOverUnder,
		},
		{
			name:     "over_under maps to over_under",
			input:    "over_under",
			expected: models.MarketTypeOverUnder,
		},
		{
			name:     "spread maps to handicap",
			input:    "spread",
			expected: models.MarketTypeHandicap,
		},
		{
			name:     "handicap maps to handicap",
			input:    "handicap",
			expected: models.MarketTypeHandicap,
		},
		{
			name:     "btts maps to both_teams",
			input:    "btts",
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
			result := mapOptiOddsMarketType(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestOptiOddsAdapter_Start_Success tests successful adapter start and polling
func TestOptiOddsAdapter_Start_Success(t *testing.T) {
	var callCount int32
	mockResponse := models.OptiOddsResponse{
		Status: "success",
		Data: []models.OptiOddsEvent{
			{
				EventID:     "event-start",
				EventName:   "Start Test",
				Sport:       "football",
				Competition: "Test League",
				StartTime:   time.Now().UTC().Format(time.RFC3339),
				Markets: []models.OptiOddsMarket{
					{
						MarketType: "match_winner",
						Outcomes: []models.OptiOddsOutcome{
							{Name: "Team A", Price: 2.00, Size: 100.00},
						},
					},
				},
			},
		},
	}

	handler := func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&callCount, 1)
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(mockResponse)
	}

	setup := setupTestOptiOddsAdapter(t, handler)
	defer setup.cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()

	// Start adapter in goroutine
	done := make(chan struct{})
	go func() {
		defer close(done)
		setup.adapter.Start(ctx)
	}()

	// Wait for context cancellation
	<-ctx.Done()
	<-done

	// Verify multiple polls occurred
	assert.GreaterOrEqual(t, atomic.LoadInt32(&callCount), int32(2))
}

// TestOptiOddsAdapter_Start_ContextCancellation tests graceful shutdown
func TestOptiOddsAdapter_Start_ContextCancellation(t *testing.T) {
	handler := func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(models.OptiOddsResponse{Status: "success", Data: []models.OptiOddsEvent{}})
	}

	setup := setupTestOptiOddsAdapter(t, handler)
	defer setup.cleanup()

	ctx, cancel := context.WithCancel(context.Background())

	// Start adapter in goroutine
	done := make(chan error)
	go func() {
		done <- setup.adapter.Start(ctx)
	}()

	// Cancel after short delay
	time.Sleep(50 * time.Millisecond)
	cancel()

	// Wait for graceful shutdown
	select {
	case err := <-done:
		assert.NoError(t, err)
	case <-time.After(1 * time.Second):
		t.Fatal("adapter did not stop gracefully")
	}
}

// TestOptiOddsAdapter_ChannelFull tests handling of full output channel
func TestOptiOddsAdapter_ChannelFull(t *testing.T) {
	// Create adapter with small channel
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		response := models.OptiOddsResponse{
			Status: "success",
			Data: []models.OptiOddsEvent{
				{
					EventID:     "event-full",
					EventName:   "Full Channel Test",
					Sport:       "football",
					Competition: "Test",
					StartTime:   time.Now().UTC().Format(time.RFC3339),
					Markets: []models.OptiOddsMarket{
						{
							MarketType: "match_winner",
							Outcomes:   []models.OptiOddsOutcome{{Name: "Team", Price: 2.00, Size: 100.00}},
						},
					},
				},
			},
		}
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	// Create channel with 0 capacity
	oddsOutput := make(chan *models.RawOdds)
	logger := zerolog.Nop()

	adapter := NewOptiOddsAdapter(
		OptiOddsConfig{
			BaseURL:      server.URL,
			APIKey:       "test-key",
			PollInterval: 100 * time.Millisecond,
		},
		oddsOutput,
		logger,
	)

	ctx := context.Background()

	// Execute poll - should not block even if channel is full
	err := adapter.poll(ctx)

	// Assert - should succeed without blocking
	assert.NoError(t, err)
}

// TestOptiOddsAdapter_HTTPClientTimeout tests HTTP client timeout handling
func TestOptiOddsAdapter_HTTPClientTimeout(t *testing.T) {
	handler := func(w http.ResponseWriter, r *http.Request) {
		// Sleep longer than client timeout
		time.Sleep(15 * time.Second)
		w.WriteHeader(http.StatusOK)
	}

	setup := setupTestOptiOddsAdapter(t, handler)
	defer setup.cleanup()

	ctx := context.Background()

	// Execute poll
	err := setup.adapter.poll(ctx)

	// Assert - should timeout
	assert.Error(t, err)
	require.Contains(t, err.Error(), "failed to execute request")
}
