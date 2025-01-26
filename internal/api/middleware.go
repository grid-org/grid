package api

import (
	"net/http"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/charmbracelet/log"
)

func echoLogger() echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			start := time.Now()

			err := next(c)

			log.Info("request",
				"method", c.Request().Method,
				"uri", c.Request().RequestURI,
				"status", c.Response().Status,
				"latency", time.Since(start),
			)

			return err
		}
	}
}

func tokenAuth() echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			token := c.Request().Header.Get("Authorization")
			if token != "Bearer my_secret_token" {
				return c.JSON(http.StatusUnauthorized, J{"error": "invalid token"})
			}
			return next(c)
		}
	}
}
