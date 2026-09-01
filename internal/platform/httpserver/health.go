package httpserver

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"
)

type HealthChecker interface {
	HealthCheck(ctx context.Context) error
}

// Deps bundles the dependencies that must be healthy for the service to
// be considered "ready" (distinct from "alive" — liveness just means the
// process is running and able to respond at all).
type Deps struct {
	Postgres HealthChecker
	Redis    HealthChecker
}

// RegisterHealthRoutes wires k8s-style liveness/readiness probes.
// Liveness never touches dependencies (a slow DB shouldn't cause the
// orchestrator to kill and restart a healthy process). Readiness checks
// dependencies so the load balancer stops sending traffic during a DB
// blip instead of serving 500s.
func RegisterHealthRoutes(r gin.IRouter, deps Deps) {
	r.GET("/healthz", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "alive"})
	})

	r.GET("/readyz", func(c *gin.Context) {
		checks := gin.H{}
		healthy := true

		if err := deps.Postgres.HealthCheck(c.Request.Context()); err != nil {
			checks["postgres"] = "unhealthy: " + err.Error()
			healthy = false
		} else {
			checks["postgres"] = "healthy"
		}

		if err := deps.Redis.HealthCheck(c.Request.Context()); err != nil {
			checks["redis"] = "unhealthy: " + err.Error()
			healthy = false
		} else {
			checks["redis"] = "healthy"
		}

		status := http.StatusOK
		if !healthy {
			status = http.StatusServiceUnavailable
		}
		c.JSON(status, gin.H{"status": map[bool]string{true: "ready", false: "not_ready"}[healthy], "checks": checks})
	})
}
