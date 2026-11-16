// Package trading holds the trading feature area: buy, sell, portfolio and order completion
// (TradeServletAction.doBuy/doSell/doPortfolio, the trading half of TradeSLSBBean, order.jsp,
// portfolio.jsp and DTBroker3MDB). Its handlers register themselves here from init(); see
// daytrader-go/README.md for the registry API. Only the trading workstream edits this package.
package trading

import (
	"context"
	"encoding/json"
	"net/http"
	"sync"

	"github.com/Cognition-Partner-Workshops/uc-java-ee-6-modernization/daytrader-go/internal/config"
	"github.com/Cognition-Partner-Workshops/uc-java-ee-6-modernization/daytrader-go/internal/web"
)

func init() {
	web.RegisterAction("buy", buy)
	web.RegisterAction("sell", sell)
	web.RegisterAction("portfolio", portfolio)
	web.RegisterAction("portfolioNoEdge", portfolio)
	// The build function runs once at start-up with the App: that is where the order consumer
	// (DTBroker3MDB replacement) starts in async mode. The route itself reports its state.
	web.RegisterRoute("GET /trading/orderQueue", orderQueueStatus)
}

// runtime is the per-App trading state: the service and, in async mode, the queue consumer.
type runtime struct {
	service   *service
	consumer  *orderConsumer // nil in sync mode
	queueKind string         // none | memory | sqs
}

var (
	runtimeMutex sync.Mutex
	runtimes     = map[*web.App]*runtime{}
	// buildQueue is replaced in tests so no AWS client is ever built there.
	buildQueue = defaultQueue
)

func defaultQueue(ctx context.Context, app *web.App) (orderQueue, string, error) {
	if app.Config.OrderQueueURL == "" {
		app.Log.Warn("ORDER_PROCESSING_MODE=async without ORDER_QUEUE_URL: using the in-memory order queue")
		return newMemoryQueue(1024), "memory", nil
	}
	queue, err := newSQSQueue(ctx, app.Config.OrderQueueURL)
	return queue, "sqs", err
}

// runtimeFor returns the trading runtime of app, building it (and starting the consumer in
// async mode) on first use. Called from the route build function at start-up and from every
// handler, so the consumer runs whichever comes first.
func runtimeFor(app *web.App) (*runtime, error) {
	runtimeMutex.Lock()
	defer runtimeMutex.Unlock()
	if existing, ok := runtimes[app]; ok {
		return existing, nil
	}
	created := &runtime{queueKind: "none"}
	var queue orderQueue
	if app.Config.OrderProcessingMode == config.OrderProcessingAsync {
		var err error
		queue, created.queueKind, err = buildQueue(context.Background(), app)
		if err != nil {
			return nil, err
		}
	}
	created.service = newService(app.Store, app.Config, queue)
	if queue != nil {
		created.consumer = newOrderConsumer(queue, created.service.completeOrder, app.Log)
		created.consumer.start(context.Background())
	}
	runtimes[app] = created
	return created, nil
}

func orderQueueStatus(app *web.App) http.Handler {
	if _, err := runtimeFor(app); err != nil {
		app.Log.Error("trading: order queue not available", "error", err)
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body := map[string]any{"orderProcessingMode": app.Config.OrderProcessingMode, "queue": "none", "consumerRunning": false}
		status := http.StatusOK
		if current, err := runtimeFor(app); err != nil {
			body["error"] = "order queue not available"
			status = http.StatusServiceUnavailable
		} else {
			body["queue"] = current.queueKind
			body["consumerRunning"] = current.consumer != nil
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_ = json.NewEncoder(w).Encode(body)
	})
}
