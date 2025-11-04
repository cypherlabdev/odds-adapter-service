package adapter

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog"
	"github.com/shopspring/decimal"
	"github.com/streadway/amqp"

	"github.com/cypherlabdev/odds-adapter-service/internal/models"
)

// Trade360Adapter consumes odds data from Trade360 RabbitMQ feed
type Trade360Adapter struct {
	conn       *amqp.Connection
	channel    *amqp.Channel
	queueName  string
	oddsOutput chan<- *models.RawOdds
	logger     zerolog.Logger
}

// Trade360Config holds configuration for Trade360 RabbitMQ connection
type Trade360Config struct {
	URL       string // e.g., "amqp://guest:guest@localhost:5672/"
	QueueName string // e.g., "trade360_odds"
	Exchange  string
	RoutingKey string
}

// NewTrade360Adapter creates a new Trade360 RabbitMQ adapter
func NewTrade360Adapter(
	config Trade360Config,
	oddsOutput chan<- *models.RawOdds,
	logger zerolog.Logger,
) (*Trade360Adapter, error) {
	// Connect to RabbitMQ
	conn, err := amqp.Dial(config.URL)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to RabbitMQ: %w", err)
	}

	// Create channel
	channel, err := conn.Channel()
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("failed to open channel: %w", err)
	}

	// Declare queue (create if not exists)
	_, err = channel.QueueDeclare(
		config.QueueName, // name
		true,             // durable
		false,            // delete when unused
		false,            // exclusive
		false,            // no-wait
		nil,              // arguments
	)
	if err != nil {
		channel.Close()
		conn.Close()
		return nil, fmt.Errorf("failed to declare queue: %w", err)
	}

	return &Trade360Adapter{
		conn:       conn,
		channel:    channel,
		queueName:  config.QueueName,
		oddsOutput: oddsOutput,
		logger:     logger.With().Str("component", "trade360_adapter").Logger(),
	}, nil
}

// Start begins consuming from Trade360 RabbitMQ
func (a *Trade360Adapter) Start(ctx context.Context) error {
	// Set QoS to limit concurrent messages
	if err := a.channel.Qos(
		10,    // prefetch count
		0,     // prefetch size
		false, // global
	); err != nil {
		return fmt.Errorf("failed to set QoS: %w", err)
	}

	// Start consuming
	messages, err := a.channel.Consume(
		a.queueName, // queue
		"",          // consumer tag (auto-generated)
		false,       // auto-ack (we'll ack manually)
		false,       // exclusive
		false,       // no-local
		false,       // no-wait
		nil,         // arguments
	)
	if err != nil {
		return fmt.Errorf("failed to register consumer: %w", err)
	}

	a.logger.Info().
		Str("queue", a.queueName).
		Msg("started consuming from Trade360 RabbitMQ")

	// Process messages
	go func() {
		for {
			select {
			case <-ctx.Done():
				a.logger.Info().Msg("stopping Trade360 adapter")
				return

			case msg, ok := <-messages:
				if !ok {
					a.logger.Warn().Msg("message channel closed")
					return
				}

				if err := a.processMessage(msg); err != nil {
					a.logger.Error().
						Err(err).
						Str("message_id", msg.MessageId).
						Msg("failed to process message")

					// Reject message (requeue if transient error)
					msg.Nack(false, true)
				} else {
					// Acknowledge message
					msg.Ack(false)
				}
			}
		}
	}()

	return nil
}

// processMessage processes a single RabbitMQ message
func (a *Trade360Adapter) processMessage(msg amqp.Delivery) error {
	// Parse JSON payload
	var data models.Trade360OddsData
	if err := json.Unmarshal(msg.Body, &data); err != nil {
		return fmt.Errorf("failed to unmarshal message: %w", err)
	}

	// Parse timestamp
	timestamp, err := time.Parse(time.RFC3339, data.Timestamp)
	if err != nil {
		a.logger.Warn().
			Err(err).
			Str("timestamp", data.Timestamp).
			Msg("failed to parse timestamp, using current time")
		timestamp = time.Now().UTC()
	}

	// Convert Trade360 format to RawOdds
	for _, market := range data.Markets {
		for _, selection := range market.Selections {
			rawOdds := &models.RawOdds{
				ID:          uuid.New(),
				Provider:    models.ProviderTrade360,
				EventID:     data.EventID,
				EventName:   data.EventName,
				Sport:       data.Sport,
				Competition: data.Competition,
				Market:      mapTrade360MarketType(market.MarketType),
				Selection:   selection.Name,
				BackPrice:   decimal.NewFromFloat(selection.BackPrice),
				LayPrice:    decimal.NewFromFloat(selection.LayPrice),
				BackSize:    decimal.NewFromFloat(selection.BackSize),
				LaySize:     decimal.NewFromFloat(selection.LaySize),
				Timestamp:   timestamp,
				ReceivedAt:  time.Now().UTC(),
				RawPayload: map[string]interface{}{
					"original": data,
					"market":   market,
					"selection": selection,
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

// Close closes the RabbitMQ connection
func (a *Trade360Adapter) Close() error {
	if err := a.channel.Close(); err != nil {
		a.logger.Error().Err(err).Msg("failed to close channel")
	}
	if err := a.conn.Close(); err != nil {
		a.logger.Error().Err(err).Msg("failed to close connection")
	}
	return nil
}

// mapTrade360MarketType maps Trade360 market type to internal MarketType
func mapTrade360MarketType(marketType string) models.MarketType {
	switch marketType {
	case "match_result", "match_odds", "1x2":
		return models.MarketTypeMatchOdds
	case "over_under", "totals":
		return models.MarketTypeOverUnder
	case "handicap", "asian_handicap":
		return models.MarketTypeHandicap
	case "btts", "both_teams_to_score":
		return models.MarketTypeBothTeams
	default:
		return models.MarketType(marketType) // Use as-is
	}
}
