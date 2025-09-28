package handlers

import (
	"fmt"
	"go-sqs-worker/pkg/sqs"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/sqs/types"
)

type Handler struct{}

func NewHandler() *Handler {
	return &Handler{}
}

func (h *Handler) Handle(message types.Message, option *sqs.HandlerOptions) error {
	fmt.Println(message)
	time.Sleep(10 * time.Second)
	return nil
}
