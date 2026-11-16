package trading

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
)

// sqsAPI is the part of the SQS client the queue uses, so tests can fake it.
type sqsAPI interface {
	SendMessage(ctx context.Context, params *sqs.SendMessageInput, optFns ...func(*sqs.Options)) (*sqs.SendMessageOutput, error)
	ReceiveMessage(ctx context.Context, params *sqs.ReceiveMessageInput, optFns ...func(*sqs.Options)) (*sqs.ReceiveMessageOutput, error)
	DeleteMessage(ctx context.Context, params *sqs.DeleteMessageInput, optFns ...func(*sqs.Options)) (*sqs.DeleteMessageOutput, error)
}

// sqsMessageBody carries what the legacy JMS message properties did.
type sqsMessageBody struct {
	Command     string `json:"command"`
	OrderID     int    `json:"orderID"`
	PublishTime int64  `json:"publishTime"`
}

const commandNewOrder = "neworder"

// sqsQueue is the orderQueue on Amazon SQS (queue URL from ORDER_QUEUE_URL).
type sqsQueue struct {
	client   sqsAPI
	queueURL string
}

// newSQSQueue builds the client from the default AWS configuration chain (the ECS task role).
// Nothing is sent to AWS until the first send or receive.
func newSQSQueue(ctx context.Context, queueURL string) (*sqsQueue, error) {
	awsConfig, err := awsconfig.LoadDefaultConfig(ctx)
	if err != nil {
		return nil, fmt.Errorf("aws config: %w", err)
	}
	return &sqsQueue{client: sqs.NewFromConfig(awsConfig), queueURL: queueURL}, nil
}

func (q *sqsQueue) send(ctx context.Context, orderID int) error {
	body, err := json.Marshal(sqsMessageBody{Command: commandNewOrder, OrderID: orderID, PublishTime: time.Now().UnixMilli()})
	if err != nil {
		return err
	}
	_, err = q.client.SendMessage(ctx, &sqs.SendMessageInput{QueueUrl: aws.String(q.queueURL), MessageBody: aws.String(string(body))})
	if err != nil {
		return fmt.Errorf("sqs send order %d: %w", orderID, err)
	}
	return nil
}

func (q *sqsQueue) receive(ctx context.Context) ([]queueMessage, error) {
	output, err := q.client.ReceiveMessage(ctx, &sqs.ReceiveMessageInput{
		QueueUrl:            aws.String(q.queueURL),
		MaxNumberOfMessages: 10,
		WaitTimeSeconds:     20,
	})
	if err != nil {
		return nil, fmt.Errorf("sqs receive: %w", err)
	}
	messages := []queueMessage{}
	for _, received := range output.Messages {
		message := queueMessage{receiptHandle: aws.ToString(received.ReceiptHandle)}
		var body sqsMessageBody
		if err := json.Unmarshal([]byte(aws.ToString(received.Body)), &body); err != nil || body.Command != commandNewOrder {
			// The MDB logged and dropped anything that was not a "neworder" command.
			if err := q.remove(ctx, message); err != nil {
				return nil, err
			}
			continue
		}
		message.orderID = body.OrderID
		messages = append(messages, message)
	}
	return messages, nil
}

func (q *sqsQueue) remove(ctx context.Context, message queueMessage) error {
	_, err := q.client.DeleteMessage(ctx, &sqs.DeleteMessageInput{QueueUrl: aws.String(q.queueURL), ReceiptHandle: aws.String(message.receiptHandle)})
	if err != nil {
		return fmt.Errorf("sqs delete: %w", err)
	}
	return nil
}
