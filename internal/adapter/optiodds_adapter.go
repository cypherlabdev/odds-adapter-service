package adapter

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog"
	"github.com/shopspring/decimal"

	"github.com/cypherlabdev/odds-adapter-service/internal/models"
)

// OptiOddsAdapter polls odds data from OptiOdds REST API
type OptiOddsAdapter struct {
	client      *http.Client
	baseURL     string
	apiKey      string
	pollInterval time.Duration
	oddsOutput  chan<- *models.RawOdds
	logger      zerolog.Logger
}

// OptiOddsConfig holds configuration for OptiOdds REST API
type OptiOddsConfig struct {
	BaseURL      string        // e.g., "https://api.optiodds.com/v1"
	APIKey       string
	PollInterval time.Duration // e.g., 5 * time.Second
}

// NewOptiOddsAdapter creates a new OptiOdds REST API adapter
func NewOptiOddsAdapter(
	config OptiOddsConfig,
	oddsOutput chan<- *models.RawOdds,
	logger zerolog.Logger,
) *OptiOddsAdapter {
	return &OptiOddsAdapter{
		client: &http.Client{
			Timeout: 10 * time.Second,
		},
		baseURL:      config.BaseURL,
		apiKey:       config.APIKey,
		pollInterval: config.PollInterval,
		oddsOutput:   oddsOutput,
		logger:       logger.With().Str("component", "optiodds_adapter").Logger(),
	}
}

// Start begins polling OptiOdds REST API
func (a *OptiOddsAdapter) Start(ctx context.Context) error {
	a.logger.Info().
		Str("base_url", a.baseURL).
		Dur("poll_interval", a.pollInterval).
		Msg("started polling OptiOdds REST API")

	ticker := time.NewTicker(a.pollInterval)
	defer ticker.Stop()

	// Initial poll
	if err := a.poll(ctx); err != nil {
		a.logger.Error().Err(err).Msg("initial poll failed")
	}

	// Periodic polling
	for {
		select {
		case <-ctx.Done():
			a.logger.Info().Msg("stopping OptiOdds adapter")
			return nil

		case <-ticker.C:
			if err := a.poll(ctx); err != nil {
				a.logger.Error().Err(err).Msg("poll failed")
				// Continue polling even if one request fails
			}
		}
	}
}

// poll fetches odds from OptiOdds API
func (a *OptiOddsAdapter) poll(ctx context.Context) error {
	// Create request
	req, err := http.NewRequestWithContext(ctx, "GET", a.baseURL+"/odds", nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	// Add API key header
	req.Header.Set("Authorization", "Bearer "+a.apiKey)
	req.Header.Set("Content-Type", "application/json")

	// Execute request
	resp, err := a.client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to execute request: %w", err)
	}
	defer resp.Body.Close()

	// Check status code
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("unexpected status code %d: %s", resp.StatusCode, string(body))
	}

	// Parse response
	var optiResp models.OptiOddsResponse
	if err := json.NewDecoder(resp.Body).Decode(&optiResp); err != nil {
		return fmt.Errorf("failed to decode response: %w", err)
	}

	// Process events
	count := 0
	for _, event := range optiResp.Data {
		if err := a.processEvent(event); err != nil {
			a.logger.Error().
				Err(err).
				Str("event_id", event.EventID).
				Msg("failed to process event")
			continue
		}
		count++
	}

	a.logger.Info().
		Int("events_processed", count).
		Msg("completed OptiOdds poll")

	return nil
}

// processEvent processes a single event from OptiOdds
func (a *OptiOddsAdapter) processEvent(event models.OptiOddsEvent) error {
	// Parse start time
	startTime, err := time.Parse(time.RFC3339, event.StartTime)
	if err != nil {
		a.logger.Warn().
			Err(err).
			Str("start_time", event.StartTime).
			Msg("failed to parse start time")
		startTime = time.Now().UTC()
	}

	// Convert OptiOdds format to RawOdds
	for _, market := range event.Markets {
		for _, outcome := range market.Outcomes {
			rawOdds := &models.RawOdds{
				ID:          uuid.New(),
				Provider:    models.ProviderOptiOdds,
				EventID:     event.EventID,
				EventName:   event.EventName,
				Sport:       event.Sport,
				Competition: event.Competition,
				Market:      mapOptiOddsMarketType(market.MarketType),
				Selection:   outcome.Name,
				BackPrice:   decimal.NewFromFloat(outcome.Price),
				LayPrice:    decimal.Zero, // OptiOdds provides back odds only
				BackSize:    decimal.NewFromFloat(outcome.Size),
				LaySize:     decimal.Zero,
				Timestamp:   startTime,
				ReceivedAt:  time.Now().UTC(),
				RawPayload: map[string]interface{}{
					"original": event,
					"market":   market,
					"outcome":  outcome,
				},
			}

			// Send to aggregator
			select {
			case a.oddsOutput <- rawOdds:
				a.logger.Debug().
					Str("event_id", rawOdds.EventID).
					Str("market", string(rawOdds.Market)).
					Str("selection", rawOdds.Selection).
					Msg("sent odds to aggregator")
			default:
				a.logger.Warn().Msg("odds output channel full, dropping message")
			}
		}
	}

	return nil
}

// mapOptiOddsMarketType maps OptiOdds market type to internal MarketType
func mapOptiOddsMarketType(marketType string) models.MarketType {
	switch marketType {
	case "match_winner", "moneyline", "1x2":
		return models.MarketTypeMatchOdds
	case "total", "over_under":
		return models.MarketTypeOverUnder
	case "spread", "handicap":
		return models.MarketTypeHandicap
	case "btts":
		return models.MarketTypeBothTeams
	default:
		return models.MarketType(marketType)
	}
}
