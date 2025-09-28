package sqs

import (
	"context"
	"log"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
)

func NewSqsClient(ctx context.Context) *sqs.Client {
	// Load the AWS configuration. The SDK automatically looks for the following information:
	// 1. Environment variables (AWS_ACCESS_KEY_ID, AWS_SECRET_ACCESS_KEY, AWS_REGION).
	// 2. Shared credentials file (~/.aws/credentials).
	// 3. IAM role (if running on EC2/ECS/EKS).
	awsConfig, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		log.Fatalf("[AWS-SQS] Can't load default config: %v", err)
	}

	return sqs.NewFromConfig(awsConfig)
}
