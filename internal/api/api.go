package api

import (
	"context"
	"net"
	"net/http"
	"strconv"
	"time"

	"github.com/grid-org/grid/internal/client"
	"github.com/grid-org/grid/internal/config"

	"github.com/charmbracelet/log"
	"github.com/labstack/echo/v4"
)

type J map[string]any

type API struct {
	config *config.Config
	client *client.Client
	echo   *echo.Echo
}

type Request struct {
	ID        uint64    `json:"id"`
	Action    string    `json:"action"`
	Payload   string    `json:"payload"`
	Timestamp time.Time `json:"timestamp"`
}

func New(cfg *config.Config, c *client.Client) *API {
	return &API{
		config: cfg,
		client: c,
		echo:   echo.New(),
	}
}

func (a *API) Start() error {
	a.echo.HideBanner = true
	a.echo.HidePort = true

	// Middleware
	a.echo.Use(echoLogger())

	// Public routes
	a.echo.GET("/status", a.getStatus)
	a.echo.GET("/job", a.getJob)
	a.echo.POST("/job", a.postJob)

	// Secure routes
	secure := a.echo.Group("/api")
	secure.Use(tokenAuth())

	addr := net.JoinHostPort(a.config.API.Host, strconv.Itoa(a.config.API.Port))

	// Start server in the background
	go func() {
		if err := a.echo.Start(addr); err != nil && err != http.ErrServerClosed {
			log.Error("API server aborted", "error", err)
		}
	}()
	log.Info("API server started", "address", addr)

	return nil
}

func (a *API) Stop() error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	return a.echo.Shutdown(ctx)
}

func (a *API) getStatus(ctx echo.Context) error {
	status, err := a.client.GetClusterStatus()
	if err != nil {
		return ctx.JSON(http.StatusInternalServerError, err)
	}
	return ctx.JSON(http.StatusOK, status)
}

func (a *API) getJob(ctx echo.Context) error {
	id := ctx.QueryParam("id")

	job, err := a.client.GetJob(id)
	if err != nil {
		return ctx.JSON(http.StatusNotFound, err)
	}

	return ctx.JSON(http.StatusOK, job)
}

func (a *API) postJob(ctx echo.Context) error {
	var job client.Job
	ctx.Bind(&job)
	return ctx.JSON(http.StatusAccepted, a.client.NewJob(job))
}
