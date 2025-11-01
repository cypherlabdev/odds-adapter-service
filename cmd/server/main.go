package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"

	"github.com/cypherlabdev/odds-adapter-service/internal/adapter"
	"github.com/cypherlabdev/odds-adapter-service/internal/aggregator"
	"github.com/cypherlabdev/odds-adapter-service/internal/config"
	"github.com/cypherlabdev/odds-adapter-service/internal/models"
)

func main() {
	// Load configuration
	cfg, err := config.LoadConfig("config/config.yaml")
	if err != nil {
		log.Fatal().Err(err).Msg("failed to load config")
	}

	// Setup logger
	logger := setupLogger(cfg.Logging)
	logger.Info().Msg("starting odds-adapter-service")

	// Create context with cancellation
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Create odds channel (buffered to handle bursts)
	oddsChannel := make(chan *models.RawOdds, 10000)

	// Start aggregator (consumes from channel, publishes to Kafka)
	agg := aggregator.NewOddsAggregator(
		aggregator.AggregatorConfig{
			KafkaBrokers: cfg.Kafka.Brokers,
			KafkaTopic:   cfg.Kafka.Topic,
			BatchSize:    cfg.Kafka.BatchSize,
			BatchTimeout: cfg.Kafka.BatchTimeout,
		},
		oddsChannel,
		logger,
	)

	// Start aggregator in goroutine
	go func() {
		if err := agg.Start(ctx); err != nil {
			logger.Error().Err(err).Msg("aggregator failed")
		}
	}()

	// Start Trade360 adapter if enabled
	if cfg.Trade360.Enabled {
		trade360Adapter, err := adapter.NewTrade360Adapter(
			adapter.Trade360Config{
				URL:        cfg.Trade360.URL,
				QueueName:  cfg.Trade360.QueueName,
				Exchange:   cfg.Trade360.Exchange,
				RoutingKey: cfg.Trade360.RoutingKey,
			},
			oddsChannel,
			logger,
		)
		if err != nil {
			logger.Error().Err(err).Msg("failed to create Trade360 adapter")
		} else {
			if err := trade360Adapter.Start(ctx); err != nil {
				logger.Error().Err(err).Msg("failed to start Trade360 adapter")
			}
			defer trade360Adapter.Close()
		}
	}

	// Start OptiOdds adapter if enabled
	if cfg.OptiOdds.Enabled {
		optioddsAdapter := adapter.NewOptiOddsAdapter(
			adapter.OptiOddsConfig{
				BaseURL:      cfg.OptiOdds.BaseURL,
				APIKey:       cfg.OptiOdds.APIKey,
				PollInterval: cfg.OptiOdds.PollInterval,
			},
			oddsChannel,
			logger,
		)
		go func() {
			if err := optioddsAdapter.Start(ctx); err != nil {
				logger.Error().Err(err).Msg("OptiOdds adapter failed")
			}
		}()
	}

	// Start HTTP server for health checks and metrics
	mux := http.NewServeMux()
	mux.HandleFunc("/health", healthHandler)
	mux.HandleFunc("/ready", readyHandler)
	mux.Handle("/metrics", promhttp.Handler())

	server := &http.Server{
		Addr:         fmt.Sprintf(":%d", cfg.Server.Port),
		Handler:      mux,
		ReadTimeout:  cfg.Server.ReadTimeout,
		WriteTimeout: cfg.Server.WriteTimeout,
	}

	// Start HTTP server in goroutine
	go func() {
		logger.Info().Int("port", cfg.Server.Port).Msg("starting HTTP server")
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error().Err(err).Msg("HTTP server failed")
		}
	}()

	// Wait for interrupt signal
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	<-sigChan

	logger.Info().Msg("shutting down gracefully...")

	// Cancel context to stop all adapters
	cancel()

	// Shutdown HTTP server
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		logger.Error().Err(err).Msg("HTTP server shutdown failed")
	}

	logger.Info().Msg("shutdown complete")
}

// setupLogger configures the logger based on config
func setupLogger(cfg config.LoggingConfig) zerolog.Logger {
	// Set log level
	level, err := zerolog.ParseLevel(cfg.Level)
	if err != nil {
		level = zerolog.InfoLevel
	}
	zerolog.SetGlobalLevel(level)

	// Set format
	if cfg.Format == "console" {
		log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stdout, TimeFormat: time.RFC3339})
	}

	return log.Logger.With().Str("service", "odds-adapter").Logger()
}

// healthHandler returns 200 if service is running
func healthHandler(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("OK"))
}

// readyHandler returns 200 if service is ready to accept traffic
func readyHandler(w http.ResponseWriter, r *http.Request) {
	// TODO: Add readiness checks (Kafka connection, etc.)
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("READY"))
}
