package main

import (
	"log"
	"log/slog"
	"os"
	"sflowg/sflowg"

	"github.com/gin-gonic/gin"
)

func main() {
	app, err := sflowg.NewApp("flows")

	if err != nil {
		log.Fatalf("Error initializing app: %v", err)
	}

	g := gin.Default()

	flow := app.Flows["test_flow"]

	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	executor := sflowg.NewExecutor(logger)

	sflowg.NewHttpHandler(&flow, app.Container, executor, g)

	err = g.Run(":8080")

	if err != nil {
		log.Fatalf("Error running server: %v", err)
	}
}
