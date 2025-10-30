package models

import (
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// Provider represents an odds data provider
type Provider string

const (
	ProviderTrade360  Provider = "trade360"
	ProviderOptiOdds  Provider = "optiodds"
	ProviderWebSocket Provider = "websocket_provider"
)

// MarketType represents the type of betting market
type MarketType string

const (
	MarketTypeMatchOdds MarketType = "match_odds"
	MarketTypeOverUnder MarketType = "over_under"
	MarketTypeHandicap  MarketType = "handicap"
	MarketTypeBothTeams MarketType = "both_teams_to_score"
)

// RawOdds represents odds data from a provider before normalization
type RawOdds struct {
	ID           uuid.UUID       `json:"id"`
	Provider     Provider        `json:"provider"`
	EventID      string          `json:"event_id"`        // Provider's event identifier
	EventName    string          `json:"event_name"`
	Sport        string          `json:"sport"`
	Competition  string          `json:"competition"`
	Market       MarketType      `json:"market"`
	Selection    string          `json:"selection"`       // e.g., "Home Win", "Over 2.5", etc.
	BackPrice    decimal.Decimal `json:"back_price"`      // Decimal odds (e.g., 2.50)
	LayPrice     decimal.Decimal `json:"lay_price,omitempty"`
	BackSize     decimal.Decimal `json:"back_size"`       // Available liquidity
	LaySize      decimal.Decimal `json:"lay_size,omitempty"`
	Timestamp    time.Time       `json:"timestamp"`
	ReceivedAt   time.Time       `json:"received_at"`
	RawPayload   map[string]interface{} `json:"raw_payload"` // Original provider payload
}

// Trade360OddsData represents the RabbitMQ payload from Trade360
type Trade360OddsData struct {
	EventID     string  `json:"event_id"`
	EventName   string  `json:"event_name"`
	Sport       string  `json:"sport"`
	Competition string  `json:"competition"`
	Markets     []Trade360Market `json:"markets"`
	Timestamp   string  `json:"timestamp"`
}

// Trade360Market represents a market in Trade360 format
type Trade360Market struct {
	MarketType  string             `json:"market_type"`
	Selections  []Trade360Selection `json:"selections"`
}

// Trade360Selection represents a selection in Trade360 format
type Trade360Selection struct {
	Name      string  `json:"name"`
	BackPrice float64 `json:"back_price"`
	LayPrice  float64 `json:"lay_price"`
	BackSize  float64 `json:"back_size"`
	LaySize   float64 `json:"lay_size"`
}

// OptiOddsResponse represents the REST API response from OptiOdds
type OptiOddsResponse struct {
	Status string         `json:"status"`
	Data   []OptiOddsEvent `json:"data"`
}

// OptiOddsEvent represents an event in OptiOdds format
type OptiOddsEvent struct {
	EventID     string         `json:"event_id"`
	EventName   string         `json:"event_name"`
	Sport       string         `json:"sport"`
	Competition string         `json:"competition"`
	StartTime   string         `json:"start_time"`
	Markets     []OptiOddsMarket `json:"markets"`
}

// OptiOddsMarket represents a market in OptiOdds format
type OptiOddsMarket struct {
	MarketType  string              `json:"market_type"`
	Outcomes    []OptiOddsOutcome   `json:"outcomes"`
}

// OptiOddsOutcome represents an outcome in OptiOdds format
type OptiOddsOutcome struct {
	Name  string  `json:"name"`
	Price float64 `json:"price"` // Decimal odds
	Size  float64 `json:"size"`  // Available stake
}

// KafkaOddsMessage represents the message sent to Kafka
type KafkaOddsMessage struct {
	OddsData  []RawOdds `json:"odds_data"`
	Provider  Provider  `json:"provider"`
	Timestamp time.Time `json:"timestamp"`
	BatchID   string    `json:"batch_id"`
}
