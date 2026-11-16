package trading

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
	sqstypes "github.com/aws/aws-sdk-go-v2/service/sqs/types"
	"github.com/shopspring/decimal"

	"github.com/Cognition-Partner-Workshops/uc-java-ee-6-modernization/daytrader-go/internal/store"
)

func dec(text string) decimal.Decimal { return decimal.RequireFromString(text) }

func quietLog() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

func TestTotals(t *testing.T) {
	fee := dec("24.95")
	if got := buyTotal(14, dec("81.37"), fee); !got.Equal(dec("1164.13")) {
		t.Errorf("buyTotal = %s, want 1164.13", got)
	}
	if got := sellTotal(14, dec("81.37"), fee); !got.Equal(dec("1114.23")) {
		t.Errorf("sellTotal = %s, want 1114.23", got)
	}
	// 0.1 x 3 style float error must not appear: 3 x 0.10 + 24.95 is exactly 25.25.
	if got := buyTotal(3, dec("0.10"), fee); got.String() != "25.25" {
		t.Errorf("buyTotal = %s, want 25.25", got)
	}
}

func TestNewQuotePrice(t *testing.T) {
	cases := []struct{ old, factor, want string }{
		{"100.00", "1.10", "110.00"},
		{"10.05", "0.90", "9.05"}, // 9.045 rounds half up
		{"0.01", "0.90", "6.00"},  // penny stock recovery x600
		{"0.02", "0.90", "0.02"},
	}
	for _, c := range cases {
		if got := newQuotePrice(dec(c.old), dec(c.factor)); !got.Equal(dec(c.want)) {
			t.Errorf("newQuotePrice(%s, %s) = %s, want %s", c.old, c.factor, got, c.want)
		}
	}
}

func TestJavaDecimal(t *testing.T) {
	basis := newJavaDecimal(dec("9.00")).mul(newJavaDecimal(decimal.NewFromFloat(10)))
	if got := basis.String(); got != "90.00" {
		t.Errorf("basis = %q, want 90.00", got)
	}
	if got := (javaDecimal{}).String(); got != "0" {
		t.Errorf("empty total = %q, want 0", got)
	}
	if got := (javaDecimal{}).add(basis).sub(newJavaDecimal(dec("100.5"))).String(); got != "-10.50" {
		t.Errorf("total = %q, want -10.50", got)
	}
}

func TestMemoryQueue(t *testing.T) {
	queue := newMemoryQueue(2)
	ctx := context.Background()
	if err := queue.send(ctx, 7); err != nil {
		t.Fatal(err)
	}
	messages, err := queue.receive(ctx)
	if err != nil || len(messages) != 1 || messages[0].orderID != 7 {
		t.Fatalf("receive = %v, %v", messages, err)
	}
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := queue.receive(cancelled); !errors.Is(err, context.Canceled) {
		t.Errorf("receive on a cancelled context = %v", err)
	}
}

// The consumer completes every queued order id, retries an order that is not visible yet, and
// keeps going after a failed completion.
func TestOrderConsumer(t *testing.T) {
	queue := newMemoryQueue(8)
	var mutex sync.Mutex
	attempts := map[int]int{}
	completed := make(chan int, 8)
	complete := func(_ context.Context, orderID int) (*store.Order, error) {
		mutex.Lock()
		defer mutex.Unlock()
		attempts[orderID]++
		switch {
		case orderID == 2 && attempts[orderID] < 3:
			return nil, store.ErrNotFound // the trade has not committed yet
		case orderID == 3:
			return nil, errOrderAlreadyCompleted
		}
		completed <- orderID
		return &store.Order{OrderID: orderID, OrderStatus: statusClosed}, nil
	}
	consumer := newOrderConsumer(queue, complete, quietLog())
	consumer.retryDelay = time.Millisecond
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	consumer.start(ctx)
	consumer.start(ctx) // once only

	for _, orderID := range []int{1, 2, 3, 4} {
		if err := queue.send(ctx, orderID); err != nil {
			t.Fatal(err)
		}
	}
	got := []int{}
	for len(got) < 3 {
		select {
		case orderID := <-completed:
			got = append(got, orderID)
		case <-time.After(5 * time.Second):
			t.Fatalf("timed out, completed so far: %v", got)
		}
	}
	if got[0] != 1 || got[1] != 2 || got[2] != 4 {
		t.Errorf("completed %v, want [1 2 4]", got)
	}
	mutex.Lock()
	defer mutex.Unlock()
	if attempts[2] != 3 || attempts[3] != 1 {
		t.Errorf("attempts = %v", attempts)
	}
}

// fakeSQS is an in-process sqsAPI: the tests never reach AWS.
type fakeSQS struct {
	bodies  []string
	deleted []string
}

func (f *fakeSQS) SendMessage(_ context.Context, in *sqs.SendMessageInput, _ ...func(*sqs.Options)) (*sqs.SendMessageOutput, error) {
	f.bodies = append(f.bodies, aws.ToString(in.MessageBody))
	return &sqs.SendMessageOutput{}, nil
}

func (f *fakeSQS) ReceiveMessage(context.Context, *sqs.ReceiveMessageInput, ...func(*sqs.Options)) (*sqs.ReceiveMessageOutput, error) {
	output := &sqs.ReceiveMessageOutput{}
	for index, body := range f.bodies {
		output.Messages = append(output.Messages, sqstypes.Message{Body: aws.String(body), ReceiptHandle: aws.String("handle-" + string(rune('a'+index)))})
	}
	f.bodies = nil
	return output, nil
}

func (f *fakeSQS) DeleteMessage(_ context.Context, in *sqs.DeleteMessageInput, _ ...func(*sqs.Options)) (*sqs.DeleteMessageOutput, error) {
	f.deleted = append(f.deleted, aws.ToString(in.ReceiptHandle))
	return &sqs.DeleteMessageOutput{}, nil
}

func TestSQSQueue(t *testing.T) {
	fake := &fakeSQS{}
	queue := &sqsQueue{client: fake, queueURL: "https://sqs.invalid/orders"}
	ctx := context.Background()
	if err := queue.send(ctx, 42); err != nil {
		t.Fatal(err)
	}
	fake.bodies = append(fake.bodies, `{"command":"ping"}`, "not json")
	messages, err := queue.receive(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(messages) != 1 || messages[0].orderID != 42 || messages[0].receiptHandle != "handle-a" {
		t.Fatalf("messages = %+v", messages)
	}
	// Anything that is not a neworder command is dropped, as the MDB did.
	if len(fake.deleted) != 2 {
		t.Errorf("deleted = %v, want the two unknown messages", fake.deleted)
	}
	if err := queue.remove(ctx, messages[0]); err != nil {
		t.Fatal(err)
	}
	if len(fake.deleted) != 3 || fake.deleted[2] != "handle-a" {
		t.Errorf("deleted = %v", fake.deleted)
	}
}
