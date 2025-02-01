package api

import (
	"encoding/json"
	"net"
	"net/http"
	"strconv"

	"github.com/grid-org/grid/internal/client"
	"github.com/grid-org/grid/internal/config"

	"github.com/charmbracelet/log"
	"github.com/labstack/echo/v4"
)

// Easy JSON type
type J map[string]any

type API struct {
	config *config.Config
	client *client.Client
}

func New(cfg *config.Config, c *client.Client) *API {
	return &API{
		config: cfg,
		client: c,
	}
}

func (a *API) Start() (*echo.Echo, error) {
	e := echo.New()
	e.HideBanner = true
	e.HidePort = true

	// Middleware
	e.Use(echoLogger())

	// Add client to handler context
	// e.Use(func(next echo.HandlerFunc) echo.HandlerFunc {
	// 	return func(ectx echo.Context) error {
	// 		ectx.Set("client", c)
	// 		return next(ectx)
	// 	}
	// })

	// Public routes
	e.GET("/status", a.getStatus)
	e.GET("/job", a.getJob)
	e.POST("/job", a.postJob)

	// Secure routes
	secure := e.Group("/api")
	secure.Use(tokenAuth())

	addr := net.JoinHostPort(a.config.API.Host, strconv.Itoa(a.config.API.Port))

	// Start server in the background
	go func() {
		if err := e.Start(addr); err != nil && err != http.ErrServerClosed {
			log.Error("API server aborted", "error", err)
		}
	}()
	log.Info("API server started", "address", addr)

	return e, nil
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
	action := ctx.Param("action")
	payload := ctx.Param("payload")

	req := client.Request{
		Action: action,
	}

	if err := json.Unmarshal([]byte(payload), &req.Payload); err != nil {
		return err
	}
	return ctx.JSON(http.StatusAccepted, a.client.NewJob(req))
}
