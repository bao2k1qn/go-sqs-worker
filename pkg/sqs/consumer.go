package sqs

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/sqs"
	"github.com/aws/aws-sdk-go-v2/service/sqs/types"
)

type ConsumerConfig struct {
	// The SQS queue from which we're processing messages. This will be used
	// to determine the queue url.
	SqsQueueName string

	// Concurrency represents the number of goroutines that will be
	// processing messages for this worker.
	Concurrency int

	// VisibilityTimeoutSeconds is the duration to wait before a message can be received
	// again by the client. It should be much higher than the processing time of a message to
	// avoid processing the same message at the same time.
	VisibilityTimeoutSeconds int

	// WaitTimeSeconds is the long-poll duration when receiving
	// messages via the SQS client.
	WaitTimeSeconds int

	// TimeoutSeconds is the max time the message handler will process
	// before the context deadline is exceeded.
	TimeoutSeconds int

	// The maximum number of messages to return. Amazon SQS never returns more
	// messages than this value (however, fewer messages might be returned). Valid values: 1 to 10. Default: 1.
	MaxNumberOfMessages int
}

type HandlerOptions struct {
	config    *ConsumerConfig
	sqsClient *sqs.Client
	ctx       *context.Context
}

// Handler implements the Handle method, which handles the logic for processing an SQS message.
// If no error is returned, the message is considered successfully processed, and it will be deleted
// from the queue. If an error is returned, the message will be retried after VisibilityTimeoutSeconds.
type Handler interface {
	Handle(message types.Message, option *HandlerOptions) error
}

// Retry defines the interface for strategies that manage retrying a failed operation.
// Implementations of this interface determine the conditions under which a retry is warranted,
// such as checking for specific error types or respecting a maximum number of attempts.
type Retries interface {
	Retry(retryFunc func() error) error
}

// DeadLetterQueue defines the interface for publishing messages that could not be processed
// successfully by the main consumer.
//
// A DLQ acts as a storage location for failed messages, allowing for later analysis,
// re-processing, or archiving without blocking the main queue.
type DLQ interface {
	Pulish(message types.Message) error
}

type SqsConsumer struct {
	config    *ConsumerConfig
	sqsClient *sqs.Client

	messages chan types.Message
	handler  Handler
	retries  Retries
	dlq      DLQ
}

func NewSqsConsumer(config *ConsumerConfig, sqsClient *sqs.Client) *SqsConsumer {
	return &SqsConsumer{
		sqsClient: sqsClient,
		config:    config,
		messages:  make(chan types.Message, config.Concurrency),
	}
}

func (s *SqsConsumer) RegisterHandler(handler Handler) {
	s.handler = handler
}

func (s *SqsConsumer) RegisterRetry(retries Retries) {
	s.retries = retries
}

func (s *SqsConsumer) RegisterDLQ(dlq DLQ) {
	s.dlq = dlq
}

func (s *SqsConsumer) Start(ctx context.Context) error {
	// 1. Retrieve the SQS Queue URL
	output, err := s.sqsClient.GetQueueUrl(ctx, &sqs.GetQueueUrlInput{
		QueueName: &s.config.SqsQueueName,
	})
	if err != nil {
		return fmt.Errorf("[SQS-Consumber] Failed to get queue url for %s: %w", s.config.SqsQueueName, err)
	}

	// 2. Start Consumer Goroutines.
	var wg sync.WaitGroup
	wg.Add(s.config.Concurrency)
	for i := 0; i < s.config.Concurrency; i++ {
		go func() {
			for message := range s.messages {
				s.processMessage(message)
			}
			wg.Done()
			slog.Error("[SQS-Consumber] Stopped worker")
		}()
	}

	// 3. Main Loop for Receiving Messages (Producer)
	for {
		// Use select to listen for context cancellation (shutdown signal). If there is a
		// context cancellation, we should close message channel to stop reciving message
		// from workers and wait for workers handling last messages successfully.
		select {
		case <-ctx.Done():
			close(s.messages)
			wg.Wait()
			return ctx.Err()
		default:
			// Continue to receive messages
		}

		receiveOutput, err := s.sqsClient.ReceiveMessage(ctx, &sqs.ReceiveMessageInput{
			QueueUrl:            output.QueueUrl,
			VisibilityTimeout:   int32(s.config.VisibilityTimeoutSeconds),
			WaitTimeSeconds:     int32(s.config.WaitTimeSeconds),
			MaxNumberOfMessages: int32(s.config.MaxNumberOfMessages),
		})

		if err != nil {
			slog.Error("[SQS-Consumber] Failed to receive messages from SQS")
			time.Sleep(1 * time.Second)
			continue
		}

		// Send the received messages to the internal channel (s.messages) for the consumers to pick up
		for _, message := range receiveOutput.Messages {
			// This is a blocking send. It will block until a consumer goroutine is ready to receive the message.
			s.messages <- message
		}
	}
}

func (s *SqsConsumer) processMessage(message types.Message) {
	defer func() {
		if r := recover(); r != nil {
			slog.Error("[SQS-Consumber] Failed to process sqs message due to panic", "message_id", *message.MessageId, "error", r)
		}
	}()

	// Create a new context (`handleCtx`) with a timeout for the user's message handler.
	// This context is derived from the main worker context (`ctx`) and uses the configured TimeoutSeconds.
	handleCtx, handleCancel := context.WithTimeout(context.Background(), time.Duration(s.config.TimeoutSeconds)*time.Second)
	defer handleCancel()

	// Binding the handle function with current data
	var bindingHandleFunc = func() error {
		return s.handler.Handle(message, &HandlerOptions{
			ctx:       &handleCtx,
			sqsClient: s.sqsClient,
			config:    s.config,
		})
	}

	var err error

	// Hanle message with retries if retry exist
	if s.retries != nil {
		err = s.retries.Retry(bindingHandleFunc)
	} else {
		err = bindingHandleFunc()
	}

	// Push the failed message to dlq if message processing fails
	if err != nil {
		if s.dlq != nil {
			s.dlq.Pulish(message)
		}

		slog.Error("[SQS-Consumber] Failed to process sqs message", "message_id", *message.MessageId, "error", err)
	}

	// Message processing was successful, so remove the message from the queue.
	_, err = s.sqsClient.DeleteMessage(context.Background(), &sqs.DeleteMessageInput{
		QueueUrl:      &s.config.SqsQueueName,
		ReceiptHandle: message.ReceiptHandle,
	})
	if err != nil {
		slog.Error("[SQS-Consumber] Failed to delete sqs message", "message_id", *message.MessageId, "error", err)
	}
}
