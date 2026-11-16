// Package config is the Go replacement for the legacy TradeConfig statics: every value comes
// from an environment variable (ECS fills them from SSM Parameter Store and Secrets Manager).
package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/shopspring/decimal"
)

const (
	OrderProcessingSync  = "sync"
	OrderProcessingAsync = "async"
)

// Config mirrors the TradeConfig values the migrated code uses.
type Config struct {
	ListenAddr   string // LISTEN_ADDR, default :8080
	ContextPath  string // CONTEXT_PATH, default /daytrader
	RestPath     string // REST_PATH, default /rest
	DatabaseURL  string // DATABASE_URL (Secrets Manager in ECS)
	DBMaxConns   int    // DB_MAX_CONNS
	MaxUsers     int    // MAX_USERS, TradeConfig.MAX_USERS (15000)
	MaxQuotes    int    // MAX_QUOTES, TradeConfig.MAX_QUOTES (10000)
	MaxHoldings  int    // MAX_HOLDINGS, TradeConfig.MAX_HOLDINGS (10)
	KeyBlockSize int    // KEY_BLOCK_SIZE, TradeConfig.KEYBLOCKSIZE (1000)
	// ORDER_PROCESSING_MODE: sync (default, what parity records) or async (SQS consumer replaces DTBroker3MDB).
	OrderProcessingMode string
	OrderQueueURL       string // ORDER_QUEUE_URL, SQS queue for async mode
	// PUBLISH_QUOTE_PRICE_CHANGE: legacy publishQuotePriceChange (DTStreamer3MDB topic). Off by default here.
	PublishQuotePriceChange bool
	UpdateQuotePrices       bool // UPDATE_QUOTE_PRICES, TradeConfig.updateQuotePrices (true)
	MarketSummaryInterval   int  // MARKET_SUMMARY_INTERVAL seconds, TradeConfig.marketSummaryInterval (20)
	OrderFee                decimal.Decimal
}

// OrderFeeFor is TradeConfig.getOrderFee: 24.95 for BUY and SELL, otherwise 0.
func (c *Config) OrderFeeFor(orderType string) decimal.Decimal {
	if strings.EqualFold(orderType, "buy") || strings.EqualFold(orderType, "sell") {
		return c.OrderFee
	}
	return decimal.Zero
}

// FromEnv reads the configuration from the environment, using lookup (os.LookupEnv in production).
func FromEnv(lookup func(string) (string, bool)) (*Config, error) {
	if lookup == nil {
		lookup = os.LookupEnv
	}
	e := envReader{lookup: lookup}
	c := &Config{
		ListenAddr:              e.str("LISTEN_ADDR", ":8080"),
		ContextPath:             "/" + strings.Trim(e.str("CONTEXT_PATH", "/daytrader"), "/"),
		RestPath:                "/" + strings.Trim(e.str("REST_PATH", "/rest"), "/"),
		DatabaseURL:             e.str("DATABASE_URL", "postgres://daytrader:daytrader@localhost:55432/daytrader?sslmode=disable"),
		DBMaxConns:              e.num("DB_MAX_CONNS", 20),
		MaxUsers:                e.num("MAX_USERS", 15000),
		MaxQuotes:               e.num("MAX_QUOTES", 10000),
		MaxHoldings:             e.num("MAX_HOLDINGS", 10),
		KeyBlockSize:            e.num("KEY_BLOCK_SIZE", 1000),
		OrderProcessingMode:     strings.ToLower(e.str("ORDER_PROCESSING_MODE", OrderProcessingSync)),
		OrderQueueURL:           e.str("ORDER_QUEUE_URL", ""),
		PublishQuotePriceChange: e.flag("PUBLISH_QUOTE_PRICE_CHANGE", false),
		UpdateQuotePrices:       e.flag("UPDATE_QUOTE_PRICES", true),
		MarketSummaryInterval:   e.num("MARKET_SUMMARY_INTERVAL", 20),
		OrderFee:                decimal.RequireFromString("24.95"),
	}
	if e.err != nil {
		return nil, e.err
	}
	if c.OrderProcessingMode != OrderProcessingSync && c.OrderProcessingMode != OrderProcessingAsync {
		return nil, fmt.Errorf("ORDER_PROCESSING_MODE must be sync or async, got %q", c.OrderProcessingMode)
	}
	return c, nil
}

type envReader struct {
	lookup func(string) (string, bool)
	err    error
}

func (e *envReader) str(name, fallback string) string {
	if value, ok := e.lookup(name); ok && value != "" {
		return value
	}
	return fallback
}

func (e *envReader) num(name string, fallback int) int {
	value, ok := e.lookup(name)
	if !ok || value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil && e.err == nil {
		e.err = fmt.Errorf("%s: %w", name, err)
	}
	return parsed
}

func (e *envReader) flag(name string, fallback bool) bool {
	value, ok := e.lookup(name)
	if !ok || value == "" {
		return fallback
	}
	parsed, err := strconv.ParseBool(value)
	if err != nil && e.err == nil {
		e.err = fmt.Errorf("%s: %w", name, err)
	}
	return parsed
}
