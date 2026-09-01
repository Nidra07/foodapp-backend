package httpserver

import (
	"time"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"

	"github.com/foodapp/backend/internal/config"
	"github.com/foodapp/backend/internal/platform/logger"
	"github.com/foodapp/backend/internal/platform/middleware"
)

// NewRouter builds the base Gin engine with global middleware applied.
// Module route registration happens in main.go after this returns, via
// each module's RegisterRoutes(rg) on the returned /api/v1 group — this
// keeps httpserver ignorant of specific modules, avoiding an import cycle
// as more of the 60+ domains get their own HTTP package.
func NewRouter(cfg *config.Config, log *logger.Logger, redisClient *redis.Client) (*gin.Engine, *gin.RouterGroup) {
	if cfg.IsProduction() {
		gin.SetMode(gin.ReleaseMode)
	}

	r := gin.New()
	r.Use(middleware.RequestID())
	r.Use(middleware.RequestLogging(log))
	r.Use(middleware.Recovery(log))
	r.Use(middleware.ErrorHandler())
	r.Use(middleware.CORS(cfg.HTTP.AllowedOrigins))
	r.Use(middleware.RateLimit(redisClient, "global", cfg.RateLimit.GlobalRPS, time.Second, middleware.ByIP))

	r.NoRoute(middleware.NotFoundHandler)

	v1 := r.Group("/api/v1")
	return r, v1
}
