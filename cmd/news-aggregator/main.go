package main

import (
	"context"
	"errors"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"

	"github.com/infopek/news-aggregator/internal/httpapi"
	"github.com/infopek/news-aggregator/internal/platform"
	"github.com/infopek/news-aggregator/internal/webassets"
)

const applicationVersion = "0.1.0"

func main() {
	if err := run(); err != nil {
		slog.Error("application stopped", "error", err)
		os.Exit(1)
	}
}

func run() error {
	assets, err := fs.Sub(webassets.Files, "dist")
	if err != nil {
		return errors.New("compiled frontend assets are unavailable")
	}
	if _, err := fs.Stat(assets, "index.html"); err != nil {
		return errors.New("compiled frontend entrypoint is unavailable; build the web application first")
	}
	port, err := configuredPort(os.Getenv("NEWS_AGGREGATOR_PORT"))
	if err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	api := http.NewServeMux()
	api.Handle("GET /api/v1/health", httpapi.NewHealthHandler(applicationVersion))
	host := platform.Host{
		Address: "127.0.0.1:" + strconv.Itoa(port),
		Handler: httpapi.NewLocalHandler(api, assets),
		Browser: platform.SystemBrowser{},
	}
	return host.Run(ctx)
}

func configuredPort(value string) (int, error) {
	if value == "" {
		return 0, nil
	}
	port, err := strconv.Atoi(value)
	if err != nil || port < 1 || port > 65535 {
		return 0, errors.New("NEWS_AGGREGATOR_PORT must be an integer from 1 to 65535")
	}
	return port, nil
}
