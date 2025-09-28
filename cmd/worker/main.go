package main

import (
	"context"
	"fmt"
	"go-sqs-worker/internal/worker"
	"os"
	"os/signal"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, os.Kill)
	defer stop()

	worker := worker.NewNotificationWorker()
	err := worker.Start(ctx)

	if err != nil {
		fmt.Println("Something went wrong %s", err.Error())
	}
}
