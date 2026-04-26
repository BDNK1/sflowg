package httptransport

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/BDNK1/sflowg/core"
	"github.com/gin-gonic/gin"
)

type Config struct {
	Addr string
}

type Transport struct {
	cfg    Config
	server *http.Server
}

func New(cfg Config) *Transport {
	return &Transport{cfg: cfg}
}

func (t *Transport) Type() string {
	return "http"
}

func (t *Transport) ResponseSubtypes() []string {
	return []string{"json", "text", "redirect"}
}

func (t *Transport) ValidateFlow(flow runtime.Flow) error {
	method, ok := flow.Entrypoint.Config["method"].(string)
	if !ok || method == "" {
		return fmt.Errorf("method must be a non-empty string")
	}
	switch strings.ToLower(method) {
	case "get", "post":
	default:
		return fmt.Errorf("unsupported method %q", method)
	}

	path, ok := flow.Entrypoint.Config["path"].(string)
	if !ok || path == "" {
		return fmt.Errorf("path must be a non-empty string")
	}
	return nil
}

func (t *Transport) Start(ctx context.Context, rt runtime.TransportRuntime) error {
	gin.SetMode(gin.ReleaseMode)
	router := gin.Default()

	for i := range rt.Flows {
		flow := rt.Flows[i]
		registerFlowRoute(&flow, rt.Container, rt.Executor, rt.GlobalProperties, rt.NewValueStore, router)
	}

	t.server = &http.Server{
		Addr:    t.cfg.Addr,
		Handler: router,
	}

	rt.Container.Logger().Info("HTTP server listening", "addr", t.cfg.Addr)

	errChan := make(chan error, 1)
	go func() {
		errChan <- t.server.ListenAndServe()
	}()

	select {
	case <-ctx.Done():
		return nil
	case err := <-errChan:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	}
}

func (t *Transport) Shutdown(ctx context.Context) error {
	if t.server == nil {
		return nil
	}
	err := t.server.Shutdown(ctx)
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}
