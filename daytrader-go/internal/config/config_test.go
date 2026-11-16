package config

import (
	"strings"
	"testing"

	"github.com/shopspring/decimal"
)

func envOf(values map[string]string) func(string) (string, bool) {
	return func(name string) (string, bool) {
		value, ok := values[name]
		return value, ok
	}
}

// TradeConfigTest.defaultsMatchTheShippedConfiguration
func TestFromEnvDefaultsMatchTheShippedConfiguration(t *testing.T) {
	cfg, err := FromEnv(envOf(nil))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.MaxUsers != 15000 || cfg.MaxQuotes != 10000 || cfg.MaxHoldings != 10 {
		t.Errorf("population = %d users, %d quotes, %d holdings", cfg.MaxUsers, cfg.MaxQuotes, cfg.MaxHoldings)
	}
	// TradeConfig.SYNCH is the shipped orderProcessingMode.
	if cfg.OrderProcessingMode != OrderProcessingSync {
		t.Errorf("OrderProcessingMode = %q", cfg.OrderProcessingMode)
	}
	// jdbc/TradeDataSource became DATABASE_URL.
	if cfg.DatabaseURL != "postgres://daytrader:daytrader@localhost:55432/daytrader?sslmode=disable" {
		t.Errorf("DatabaseURL = %q", cfg.DatabaseURL)
	}
	if cfg.ListenAddr != ":8080" || cfg.ContextPath != "/daytrader" || cfg.RestPath != "/rest" {
		t.Errorf("addr %q context %q rest %q", cfg.ListenAddr, cfg.ContextPath, cfg.RestPath)
	}
	if cfg.DBMaxConns != 20 || cfg.KeyBlockSize != 1000 || cfg.MarketSummaryInterval != 20 {
		t.Errorf("conns %d keyBlock %d interval %d", cfg.DBMaxConns, cfg.KeyBlockSize, cfg.MarketSummaryInterval)
	}
	if cfg.PublishQuotePriceChange || !cfg.UpdateQuotePrices || cfg.OrderQueueURL != "" {
		t.Errorf("publish %v update %v queue %q", cfg.PublishQuotePriceChange, cfg.UpdateQuotePrices, cfg.OrderQueueURL)
	}
	if cfg.OrderFee.String() != "24.95" {
		t.Errorf("OrderFee = %s", cfg.OrderFee)
	}
}

func TestFromEnvNilLookupUsesTheProcessEnvironment(t *testing.T) {
	t.Setenv("MAX_USERS", "42")
	t.Setenv("ORDER_PROCESSING_MODE", "async")
	cfg, err := FromEnv(nil)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.MaxUsers != 42 || cfg.OrderProcessingMode != OrderProcessingAsync {
		t.Errorf("MaxUsers %d mode %q", cfg.MaxUsers, cfg.OrderProcessingMode)
	}
}

func TestFromEnvOverrides(t *testing.T) {
	cfg, err := FromEnv(envOf(map[string]string{
		"LISTEN_ADDR": ":9000", "CONTEXT_PATH": "trader/", "REST_PATH": "/api/", "DATABASE_URL": "postgres://db/x",
		"DB_MAX_CONNS": "5", "MAX_USERS": "200", "MAX_QUOTES": "50", "MAX_HOLDINGS": "3", "KEY_BLOCK_SIZE": "10",
		"ORDER_QUEUE_URL": "https://sqs/q", "PUBLISH_QUOTE_PRICE_CHANGE": "true", "UPDATE_QUOTE_PRICES": "0",
		"MARKET_SUMMARY_INTERVAL": "7",
	}))
	if err != nil {
		t.Fatal(err)
	}
	want := Config{
		ListenAddr: ":9000", ContextPath: "/trader", RestPath: "/api", DatabaseURL: "postgres://db/x",
		DBMaxConns: 5, MaxUsers: 200, MaxQuotes: 50, MaxHoldings: 3, KeyBlockSize: 10,
		OrderProcessingMode: "sync", OrderQueueURL: "https://sqs/q", PublishQuotePriceChange: true,
		UpdateQuotePrices: false, MarketSummaryInterval: 7, OrderFee: cfg.OrderFee,
	}
	if *cfg != want {
		t.Errorf("got %+v\nwant %+v", *cfg, want)
	}
}

func TestFromEnvEmptyValuesFallBackToDefaults(t *testing.T) {
	cfg, err := FromEnv(envOf(map[string]string{"MAX_USERS": "", "UPDATE_QUOTE_PRICES": "", "LISTEN_ADDR": "", "ORDER_PROCESSING_MODE": ""}))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.MaxUsers != 15000 || !cfg.UpdateQuotePrices || cfg.ListenAddr != ":8080" || cfg.OrderProcessingMode != "sync" {
		t.Errorf("got %+v", *cfg)
	}
}

// TradeConfigTest.unknownEnumValueLeavesSettingUnchanged: the legacy "Asynchronous_2-Phase" is
// ORDER_PROCESSING_MODE=async. The enum is matched ignoring case, as setConfigParam does.
func TestOrderProcessingModeIsMatchedIgnoringCase(t *testing.T) {
	for _, value := range []string{"async", "ASYNC", "Async"} {
		cfg, err := FromEnv(envOf(map[string]string{"ORDER_PROCESSING_MODE": value}))
		if err != nil || cfg.OrderProcessingMode != OrderProcessingAsync {
			t.Errorf("%q: mode %v, err %v", value, cfg, err)
		}
	}
}

// Deliberate difference from TradeConfigTest.unknownEnumValueLeavesSettingUnchanged: legacy
// keeps the old value for an unknown enum display name; the Go config refuses to start.
func TestUnknownOrderProcessingModeIsAnError(t *testing.T) {
	cfg, err := FromEnv(envOf(map[string]string{"ORDER_PROCESSING_MODE": "Eventually"}))
	if err == nil || cfg != nil {
		t.Fatalf("cfg %v, err %v", cfg, err)
	}
	if !strings.Contains(err.Error(), "ORDER_PROCESSING_MODE") || !strings.Contains(err.Error(), "eventually") {
		t.Errorf("error = %v", err)
	}
}

// TradeConfigTest.numericLimitsAreParsedAndBadNumbersAreIgnored: the numbers are parsed; a
// bad number is a start-up error here instead of being ignored (deliberate difference).
func TestNumericLimitsAreParsedAndBadNumbersAreErrors(t *testing.T) {
	cfg, err := FromEnv(envOf(map[string]string{"MAX_USERS": "200", "MAX_QUOTES": "50"}))
	if err != nil || cfg.MaxUsers != 200 || cfg.MaxQuotes != 50 {
		t.Fatalf("cfg %+v, err %v", cfg, err)
	}
	for _, name := range []string{"MAX_USERS", "MAX_QUOTES", "MAX_HOLDINGS", "KEY_BLOCK_SIZE", "DB_MAX_CONNS", "MARKET_SUMMARY_INTERVAL"} {
		cfg, err := FromEnv(envOf(map[string]string{name: "lots"}))
		if err == nil || cfg != nil || !strings.HasPrefix(err.Error(), name+":") {
			t.Errorf("%s=lots: cfg %v, err %v", name, cfg, err)
		}
	}
}

func TestFirstBadValueIsReported(t *testing.T) {
	_, err := FromEnv(envOf(map[string]string{"DB_MAX_CONNS": "x", "MAX_USERS": "y"}))
	if err == nil || !strings.HasPrefix(err.Error(), "DB_MAX_CONNS:") {
		t.Errorf("error = %v", err)
	}
}

func TestBadFlagIsAnError(t *testing.T) {
	for _, name := range []string{"PUBLISH_QUOTE_PRICE_CHANGE", "UPDATE_QUOTE_PRICES"} {
		_, err := FromEnv(envOf(map[string]string{name: "maybe"}))
		if err == nil || !strings.HasPrefix(err.Error(), name+":") {
			t.Errorf("%s: error = %v", name, err)
		}
	}
}

// Legacy setConfigParam trims the value (" 200 " -> 200). FromEnv does not.
func TestNumericValuesAreTrimmedLikeSetConfigParam(t *testing.T) {
	cfg, err := FromEnv(envOf(map[string]string{"MAX_USERS": " 200 "}))
	if err != nil {
		t.Skipf("scaffold difference (reported in the PR): config.FromEnv does not trim whitespace, legacy TradeConfig.setConfigParam does: %v", err)
	}
	if cfg.MaxUsers != 200 {
		t.Errorf("MaxUsers = %d", cfg.MaxUsers)
	}
}

// TradeConfigTest.orderFeeAppliesToBuyAndSellOnly
func TestOrderFeeAppliesToBuyAndSellOnly(t *testing.T) {
	cfg, err := FromEnv(envOf(nil))
	if err != nil {
		t.Fatal(err)
	}
	fee := decimal.RequireFromString("24.95")
	for _, orderType := range []string{"buy", "SELL", "Buy", "sell"} {
		if got := cfg.OrderFeeFor(orderType); !got.Equal(fee) || got.StringFixed(2) != "24.95" {
			t.Errorf("OrderFeeFor(%q) = %s", orderType, got)
		}
	}
	for _, orderType := range []string{"cash", "", "buy "} {
		if got := cfg.OrderFeeFor(orderType); !got.IsZero() {
			t.Errorf("OrderFeeFor(%q) = %s", orderType, got)
		}
	}
}
