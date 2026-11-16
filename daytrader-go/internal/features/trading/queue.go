package trading

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"time"

	"github.com/Cognition-Partner-Workshops/uc-java-ee-6-modernization/daytrader-go/internal/store"
)

// queueMessage is one "neworder" message: the order to complete, plus whatever the queue needs
// to acknowledge it (the SQS receipt handle).
type queueMessage struct {
	orderID       int
	receiptHandle string
}

// orderQueue replaces jms/TradeBrokerQueue. send is the legacy queueOrder; receive and remove
// are what the container did for DTBroker3MDB. receive blocks until there is a message, the
// queue's wait time passes (empty result) or ctx ends.
type orderQueue interface {
	send(ctx context.Context, orderID int) error
	receive(ctx context.Context) ([]queueMessage, error)
	remove(ctx context.Context, message queueMessage) error
}

// memoryQueue is the in-process orderQueue used by tests, and by async mode when no
// ORDER_QUEUE_URL is set (one process only; nothing survives a restart).
type memoryQueue struct {
	messages chan queueMessage
}

func newMemoryQueue(capacity int) *memoryQueue {
	return &memoryQueue{messages: make(chan queueMessage, capacity)}
}

func (q *memoryQueue) send(ctx context.Context, orderID int) error {
	select {
	case q.messages <- queueMessage{orderID: orderID}:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (q *memoryQueue) receive(ctx context.Context) ([]queueMessage, error) {
	select {
	case message := <-q.messages:
		return []queueMessage{message}, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (q *memoryQueue) remove(context.Context, queueMessage) error { return nil }

// completeFunc completes one order (service.completeOrder).
type completeFunc func(ctx context.Context, orderID int) (*store.Order, error)

// orderConsumer replaces DTBroker3MDB.onMessage: it takes order ids off the queue and
// completes each order in its own transaction.
type orderConsumer struct {
	queue    orderQueue
	complete completeFunc
	log      *slog.Logger
	// An order id can arrive just before the trade that queued it commits; the lookup is
	// retried this many times, retryDelay apart, before the message is left for redelivery.
	retries    int
	retryDelay time.Duration
	startOnce  sync.Once
}

func newOrderConsumer(queue orderQueue, complete completeFunc, logger *slog.Logger) *orderConsumer {
	return &orderConsumer{queue: queue, complete: complete, log: logger, retries: 5, retryDelay: 200 * time.Millisecond}
}

// start runs the consumer loop in the background, once.
func (c *orderConsumer) start(ctx context.Context) {
	c.startOnce.Do(func() {
		c.log.Info("order consumer started (DTBroker3MDB replacement)")
		go c.run(ctx)
	})
}

// run processes messages until ctx ends.
func (c *orderConsumer) run(ctx context.Context) {
	for ctx.Err() == nil {
		messages, err := c.queue.receive(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			c.log.Error("order consumer: receive failed", "error", err)
			select {
			case <-time.After(time.Second):
			case <-ctx.Done():
				return
			}
			continue
		}
		for _, message := range messages {
			c.handle(ctx, message)
		}
	}
}

// handle completes one order. Like the MDB, a failed completion is logged and not retried (the
// legacy bean ignored redelivered messages); only "order not visible yet" is left on the queue.
func (c *orderConsumer) handle(ctx context.Context, message queueMessage) {
	var err error
	for attempt := 0; ; attempt++ {
		_, err = c.complete(ctx, message.orderID)
		if !errors.Is(err, store.ErrNotFound) || attempt >= c.retries {
			break
		}
		select {
		case <-time.After(c.retryDelay):
		case <-ctx.Done():
			return
		}
	}
	if errors.Is(err, store.ErrNotFound) {
		c.log.Error("order consumer: order not found, leaving the message for redelivery", "orderID", message.orderID)
		return
	}
	if err != nil {
		c.log.Error("order consumer: completing order failed", "orderID", message.orderID, "error", err)
	}
	if err := c.queue.remove(ctx, message); err != nil {
		c.log.Error("order consumer: removing message failed", "orderID", message.orderID, "error", err)
	}
}
