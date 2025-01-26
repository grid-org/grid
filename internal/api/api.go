package api

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/grid-org/grid/internal/client"
	"github.com/grid-org/grid/internal/config"

	"github.com/charmbracelet/log"
	"github.com/labstack/echo/v4"
)

func Run(cfg *config.Config, c *client.Client) error {
	e := echo.New()
	e.HideBanner = true
	e.HidePort = true

	// Middleware
	e.Use(echoLogger())

	// Add client to handler context
	e.Use(func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(ectx echo.Context) error {
			ectx.Set("client", c)
			return next(ectx)
		}
	})

	// Public routes
	e.GET("/status", getStatus)
	e.GET("/job", getJob)
	e.POST("/job", postJob)

	// Secure routes
	secure := e.Group("/api")
	secure.Use(tokenAuth())

	addr := net.JoinHostPort(cfg.API.Host, strconv.Itoa(cfg.API.Port))

	// Start server in the background
	go func() {
		if err := e.Start(addr); err != nil && err != http.ErrServerClosed {
			log.Error("API server aborted", "error", err)
		}
	}()
	log.Info("API server started", "address", addr)

	// Wait for signal
	sigch := make(chan os.Signal, 1)
	signal.Notify(sigch, os.Interrupt, syscall.SIGTERM)
	<-sigch

	// Shutdown context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Shutdown server
	log.Info("Shutting down API server")
	if err := e.Shutdown(ctx); err != nil {
		return err
	}

	return nil
}

func getStatus(ectx echo.Context) error {
	c := ectx.Get("client").(*client.Client)
	return ectx.JSON(http.StatusOK, c.GetStatus())
}

func getJob(ectx echo.Context) error {
	c := ectx.Get("client").(*client.Client)
	id := ectx.QueryParam("id")
	return ectx.JSON(http.StatusOK, c.GetJob(id))
}

func postJob(ectx echo.Context) error {
	c := ectx.Get("client").(*client.Client)
	action := ectx.Param("action")
	payload := ectx.Param("payload")

	req := client.Request{
		Action: action,
	}

	if err := json.Unmarshal([]byte(payload), &req.Payload); err != nil {
		return err
	}
	return ectx.JSON(http.StatusAccepted, c.NewJob(req))
}
