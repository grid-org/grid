package api

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"net"
	"net/http"
	"strconv"
	"time"

	"github.com/grid-org/grid/internal/client"
	"github.com/grid-org/grid/internal/config"
	"github.com/grid-org/grid/internal/models"
	"github.com/grid-org/grid/internal/scheduler"

	"github.com/charmbracelet/log"
	"github.com/labstack/echo/v4"
)

type J map[string]any

type API struct {
	config    *config.Config
	client    *client.Client
	scheduler *scheduler.Scheduler
	echo      *echo.Echo
}

// JobRequest is the HTTP request body for creating a job.
type JobRequest struct {
	Target models.Target `json:"target"`
	Tasks  []models.Task `json:"tasks"`
}

func New(cfg *config.Config, c *client.Client, sched *scheduler.Scheduler) *API {
	return &API{
		config:    cfg,
		client:    c,
		scheduler: sched,
		echo:      echo.New(),
	}
}

func (a *API) Start() error {
	a.echo.HideBanner = true
	a.echo.HidePort = true

	a.echo.Use(echoLogger())

	a.echo.GET("/status", a.getStatus)
	a.echo.POST("/job", a.postJob)
	a.echo.GET("/job/:id", a.getJob)
	a.echo.GET("/jobs", a.listJobs)
	a.echo.GET("/nodes", a.listNodes)
	a.echo.GET("/node/:id", a.getNode)

	addr := net.JoinHostPort(a.config.API.Host, strconv.Itoa(a.config.API.Port))

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
		return ctx.JSON(http.StatusInternalServerError, J{"error": err.Error()})
	}
	return ctx.JSON(http.StatusOK, J{"status": string(status.Value())})
}

func (a *API) postJob(ctx echo.Context) error {
	var req JobRequest
	if err := ctx.Bind(&req); err != nil {
		return ctx.JSON(http.StatusBadRequest, J{"error": err.Error()})
	}

	if len(req.Tasks) == 0 {
		return ctx.JSON(http.StatusBadRequest, J{"error": "at least one task is required"})
	}

	if req.Target.Scope == "" {
		req.Target.Scope = "all"
	}

	job := models.Job{
		ID:     generateID(),
		Target: req.Target,
		Tasks:  req.Tasks,
	}

	job, err := a.scheduler.Submit(job)
	if err != nil {
		return ctx.JSON(http.StatusInternalServerError, J{"error": err.Error()})
	}

	return ctx.JSON(http.StatusAccepted, job)
}

func (a *API) getJob(ctx echo.Context) error {
	id := ctx.Param("id")

	job, err := a.client.GetJob(id)
	if err != nil {
		return ctx.JSON(http.StatusNotFound, J{"error": err.Error()})
	}

	return ctx.JSON(http.StatusOK, job)
}

func (a *API) listJobs(ctx echo.Context) error {
	jobs, err := a.client.ListJobs()
	if err != nil {
		return ctx.JSON(http.StatusInternalServerError, J{"error": err.Error()})
	}

	return ctx.JSON(http.StatusOK, jobs)
}

func (a *API) listNodes(ctx echo.Context) error {
	nodes, err := a.client.ListNodes()
	if err != nil {
		return ctx.JSON(http.StatusInternalServerError, J{"error": err.Error()})
	}

	return ctx.JSON(http.StatusOK, nodes)
}

func (a *API) getNode(ctx echo.Context) error {
	id := ctx.Param("id")

	node, err := a.client.GetNode(id)
	if err != nil {
		return ctx.JSON(http.StatusNotFound, J{"error": err.Error()})
	}

	return ctx.JSON(http.StatusOK, node)
}

func generateID() string {
	b := make([]byte, 8)
	rand.Read(b)
	return hex.EncodeToString(b)
}
