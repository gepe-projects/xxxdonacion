package server

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"xxxdonacion/platform/config"

	"github.com/labstack/echo/v5"
	"github.com/labstack/echo/v5/middleware"
	emw "github.com/labstack/echo/v5/middleware"
)

const (
	readHeaderTimeout = 10 * time.Second
	readTimeout       = 15 * time.Second
	writeTimeout      = 30 * time.Second
	idleTimeout       = 60 * time.Second
	// Harus > requestTimeout: setiap handler (ctx-nya dibatalkan ContextTimeout)
	// selesai lebih dulu sebelum dependency ditutup di shutdown.
	gracefulTimeout = requestTimeout + 5*time.Second

	requestTimeout = 15 * time.Second
	bodyLimitBytes = 1 << 20 // 1 MiB
)

type Server struct {
	Echo *echo.Echo
	cfg  *config.App
}

func New(cfg *config.App) *Server {
	e := echo.NewWithConfig(echo.Config{})
	e.Use(emw.Recover())
	e.Use(middleware.BodyLimit(bodyLimitBytes))
	// set global middleware

	return &Server{Echo: e, cfg: cfg}
}

func (s *Server) Start() error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	config := echo.StartConfig{
		Address:         fmt.Sprintf(":%d", s.cfg.Port),
		GracefulTimeout: gracefulTimeout,
		BeforeServeFunc: func(s *http.Server) error {
			s.ReadHeaderTimeout = readHeaderTimeout
			s.ReadTimeout = readTimeout
			s.WriteTimeout = writeTimeout
			s.IdleTimeout = idleTimeout
			return nil
		},
	}

	return config.Start(ctx, s.Echo)
}
