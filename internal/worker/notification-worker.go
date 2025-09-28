package worker

import (
	"context"
	"go-sqs-worker/internal/worker/handlers"
	"go-sqs-worker/pkg/sqs"
)

type NotificationWorker struct {
}

func NewNotificationWorker() *NotificationWorker {
	return &NotificationWorker{}
}

func (n *NotificationWorker) Start(ctx context.Context) error {
	sqsConsumerConfig := &sqs.ConsumerConfig{
		SqsQueueName:             "localstack-queue",
		Concurrency:              2,
		VisibilityTimeoutSeconds: 60,
		WaitTimeSeconds:          10,
		TimeoutSeconds:           20,
		MaxNumberOfMessages:      1,
	}

	sqsClient := sqs.NewSqsClient(ctx)

	sqsConsumer := sqs.NewSqsConsumer(sqsConsumerConfig, sqsClient)
	sqsConsumer.RegisterHandler(handlers.NewHandler())

	return sqsConsumer.Start(ctx)
}
