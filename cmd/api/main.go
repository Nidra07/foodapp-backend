// cmd/api is the composition root: the only place in the codebase that
// wires config -> infrastructure -> repositories -> services -> HTTP
// handlers together. Every module stays decoupled from every other
// module except through this file, so new domains (restaurants, orders,
// delivery, ...) plug in the same way without touching existing wiring.
package main

import (
	"context"
	"errors"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/foodapp/backend/internal/config"

	identityapp "github.com/foodapp/backend/internal/modules/identity/application"
	identitydomain "github.com/foodapp/backend/internal/modules/identity/domain"
	identityinfra "github.com/foodapp/backend/internal/modules/identity/infrastructure"
	identityhttp "github.com/foodapp/backend/internal/modules/identity/interfaces/http"

	usersapp "github.com/foodapp/backend/internal/modules/users/application"
	usersinfra "github.com/foodapp/backend/internal/modules/users/infrastructure"
	usershttp "github.com/foodapp/backend/internal/modules/users/interfaces/http"

	restaurantsapp "github.com/foodapp/backend/internal/modules/restaurants/application"
	restaurantsinfra "github.com/foodapp/backend/internal/modules/restaurants/infrastructure"
	restaurantshttp "github.com/foodapp/backend/internal/modules/restaurants/interfaces/http"

	menuapp "github.com/foodapp/backend/internal/modules/menu/application"
	menuinfra "github.com/foodapp/backend/internal/modules/menu/infrastructure"
	menuhttp "github.com/foodapp/backend/internal/modules/menu/interfaces/http"

	cartapp "github.com/foodapp/backend/internal/modules/cart/application"
	cartinfra "github.com/foodapp/backend/internal/modules/cart/infrastructure"
	carthttp "github.com/foodapp/backend/internal/modules/cart/interfaces/http"

	ordersapp "github.com/foodapp/backend/internal/modules/orders/application"
	ordersinfra "github.com/foodapp/backend/internal/modules/orders/infrastructure"
	ordershttp "github.com/foodapp/backend/internal/modules/orders/interfaces/http"

	paymentsapp "github.com/foodapp/backend/internal/modules/payments/application"
	paymentsinfra "github.com/foodapp/backend/internal/modules/payments/infrastructure"
	paymentshttp "github.com/foodapp/backend/internal/modules/payments/interfaces/http"

	deliveryapp "github.com/foodapp/backend/internal/modules/delivery/application"
	deliveryinfra "github.com/foodapp/backend/internal/modules/delivery/infrastructure"
	deliveryhttp "github.com/foodapp/backend/internal/modules/delivery/interfaces/http"

	notificationsapp "github.com/foodapp/backend/internal/modules/notifications/application"
	notificationsinfra "github.com/foodapp/backend/internal/modules/notifications/infrastructure"
	notificationshttp "github.com/foodapp/backend/internal/modules/notifications/interfaces/http"

	settlementsapp "github.com/foodapp/backend/internal/modules/settlements/application"
	settlementsinfra "github.com/foodapp/backend/internal/modules/settlements/infrastructure"
	settlementshttp "github.com/foodapp/backend/internal/modules/settlements/interfaces/http"

	adminrbacapp "github.com/foodapp/backend/internal/modules/adminrbac/application"
	adminrbacinfra "github.com/foodapp/backend/internal/modules/adminrbac/infrastructure"
	adminrbachttp "github.com/foodapp/backend/internal/modules/adminrbac/interfaces/http"

	searchapp "github.com/foodapp/backend/internal/modules/search/application"
	searchinfra "github.com/foodapp/backend/internal/modules/search/infrastructure"
	searchhttp "github.com/foodapp/backend/internal/modules/search/interfaces/http"

	"github.com/foodapp/backend/internal/platform/db"
	sqlcgen "github.com/foodapp/backend/internal/platform/db/sqlc"
	"github.com/foodapp/backend/internal/platform/httpserver"
	"github.com/foodapp/backend/internal/platform/logger"
	"github.com/foodapp/backend/internal/platform/middleware"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		panic("failed to load config: " + err.Error())
	}

	log := logger.New(cfg.App.LogLevel, cfg.App.LogFormat, cfg.App.Name, cfg.App.Version)
	log.Info().Str("env", cfg.App.Env).Msg("starting foodapp-backend")

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// --- infrastructure ---
	pg, err := db.NewPostgres(ctx, cfg.Postgres)
	if err != nil {
		log.Fatal().Err(err).Msg("failed to connect to postgres")
	}
	defer pg.Close()
	log.Info().Msg("connected to postgres")

	redisConn, err := db.NewRedis(ctx, cfg.Redis)
	if err != nil {
		log.Fatal().Err(err).Msg("failed to connect to redis")
	}
	defer redisConn.Close()
	log.Info().Msg("connected to redis")

	queries := sqlcgen.New(pg.Pool)

	// --- identity & auth module ---
	identityRepo := identityinfra.NewRepository(queries)
	usersRepo := usersinfra.NewRepository(queries)
	tokenIssuer := identityinfra.NewJWTTokenIssuer(cfg.JWT.AccessSecret, cfg.JWT.Issuer, cfg.JWT.AccessTokenTTL)

	var otpSender identitydomain.OTPSender
	if cfg.IsLocal() || cfg.SMS.Provider == "mock" {
		otpSender = identityinfra.NewMockOTPSender(log)
	} else {
		// TODO(hardening): wire a real MSG91/Twilio (SMS) or SES/SendGrid
		// (email) sender here behind the same domain.OTPSender interface.
		// Intentionally kept separate from the Notifications module's own
		// SMS/Email senders (Phase 6) — OTP delivery has different
		// urgency/reliability requirements, and Identity should not take a
		// dependency on Notifications. Falling back to mock keeps non-local
		// environments from panicking before a real provider is wired in,
		// but this must be replaced before production.
		log.Warn().Msg("no production OTP provider wired yet — falling back to mock sender")
		otpSender = identityinfra.NewMockOTPSender(log)
	}

	authService := identityapp.NewAuthService(identityRepo, usersRepo, otpSender, tokenIssuer, identityapp.AuthConfig{
		OTPLength:         cfg.OTP.Length,
		OTPTTL:            cfg.OTP.TTL,
		OTPMaxAttempts:    cfg.OTP.MaxAttempts,
		OTPResendCooldown: cfg.OTP.ResendCooldown,
		OTPMaxPerDay:      cfg.OTP.MaxRequestsPerDay,
		RefreshTokenTTL:   cfg.JWT.RefreshTokenTTL,
	})
	authHandler := identityhttp.NewAuthHandler(authService)

	// --- users module ---
	userService := usersapp.NewUserService(usersRepo)
	userHandler := usershttp.NewUserHandler(userService)

	// --- admin RBAC module ---
	// Wired before Restaurants/Payments/Settlements since they take it as
	// an optional AuditLogger dependency (see each module's AuditLogger
	// interface).
	adminrbacRepo := adminrbacinfra.NewRepository(pg.Pool, queries)
	rbacService := adminrbacapp.NewRBACService(adminrbacRepo)
	rbacHandler := adminrbachttp.NewRBACHandler(rbacService)

	// --- restaurants module ---
	restaurantsRepo := restaurantsinfra.NewRepository(queries)
	restaurantService := restaurantsapp.NewRestaurantService(restaurantsRepo, rbacService)
	restaurantHandler := restaurantshttp.NewRestaurantHandler(restaurantService)

	// --- menu module ---
	menuRepo := menuinfra.NewRepository(queries)
	menuService := menuapp.NewMenuService(menuRepo)
	menuHandler := menuhttp.NewMenuHandler(menuService)

	// --- cart module ---
	cartRepo := cartinfra.NewRepository(queries)
	cartService := cartapp.NewCartService(cartRepo)
	cartHandler := carthttp.NewCartHandler(cartService)

	// --- notifications module ---
	// Wired before Orders/Payments/Delivery since they take it as an
	// optional dependency (see each module's Notifier interface).
	notificationsRepo := notificationsinfra.NewRepository(queries)
	deviceLookup := notificationsinfra.NewDeviceLookupAdapter(queries)
	contactLookup := notificationsinfra.NewContactLookupAdapter(queries)
	notificationService := notificationsapp.NewNotificationService(
		notificationsRepo,
		notificationsinfra.NewMockPushSender(log),  // TODO(Phase 6 hardening): swap for real FCM before production — see docs/assumptions.md
		notificationsinfra.NewMockSMSSender(log),   // TODO(Phase 6 hardening): swap for real SMS provider (MSG91/Twilio)
		notificationsinfra.NewMockEmailSender(log), // TODO(Phase 6 hardening): swap for real email provider (SES/SendGrid)
		deviceLookup, contactLookup, log,
	)
	notificationHandler := notificationshttp.NewNotificationHandler(notificationService)

	// --- orders module ---
	ordersRepo := ordersinfra.NewRepository(pg.Pool, queries)
	orderService := ordersapp.NewOrderService(ordersRepo, cartService, restaurantService, notificationService, ordersapp.PricingConfig{
		TaxRatePct:      5.0, // flat 5% GST assumption — see docs/assumptions.md
		FlatDeliveryFee: 40.0,
	})
	orderHandler := ordershttp.NewOrderHandler(orderService)

	// --- payments module ---
	paymentsRepo := paymentsinfra.NewRepository(queries)
	paymentGateway := paymentsinfra.NewMockGateway("") // TODO(Phase 4 hardening): swap for a real gateway behind domain.PaymentGateway before production — see docs/assumptions.md
	paymentService := paymentsapp.NewPaymentService(paymentsRepo, paymentGateway, orderService, orderService, notificationService, rbacService)
	paymentHandler := paymentshttp.NewPaymentHandler(paymentService)

	// --- delivery module ---
	deliveryRepo := deliveryinfra.NewRepository(queries)
	deliveryService := deliveryapp.NewDeliveryService(deliveryRepo, orderService, orderService, notificationService, deliveryapp.DispatchConfig{
		MaxActiveDeliveries: 3,     // flat cap — see docs/assumptions.md
		SearchRadiusM:       8000,  // 8km search radius for nearest-partner dispatch
	})
	deliveryHandler := deliveryhttp.NewDeliveryHandler(deliveryService)

	// --- settlements module ---
	settlementsRepo := settlementsinfra.NewRepository(queries)
	settlementService := settlementsapp.NewSettlementService(settlementsRepo, orderService, deliveryService, restaurantService, deliveryService, rbacService, settlementsapp.PayoutConfig{
		PerDeliveryFee: 30.0, // flat per-delivery payout to partners — see docs/assumptions.md
	})
	settlementHandler := settlementshttp.NewSettlementHandler(settlementService)

	// --- HTTP server ---
	router, v1 := httpserver.NewRouter(cfg, log, redisConn.Client)

	httpserver.RegisterHealthRoutes(router, httpserver.Deps{Postgres: pg, Redis: redisConn})

	authHandler.RegisterRoutes(v1)

	authMW := middleware.RequireAuth(cfg.JWT.AccessSecret)
	adminOnlyMW := middleware.RequireRole("admin")
	ownerOnlyMW := middleware.RequireRole("restaurant_owner", "restaurant_staff", "admin")
	customerOnlyMW := middleware.RequireRole("customer", "admin")
	userHandler.RegisterRoutes(v1, authMW, adminOnlyMW)
	restaurantHandler.RegisterRoutes(v1, authMW, ownerOnlyMW, adminOnlyMW)
	menuHandler.RegisterRoutes(v1, authMW, ownerOnlyMW)
	cartHandler.RegisterRoutes(v1, authMW)
	orderHandler.RegisterRoutes(v1, authMW, customerOnlyMW, ownerOnlyMW)
	paymentHandler.RegisterRoutes(v1, authMW)
	paymentHandler.RegisterAdminRoutes(v1, authMW, adminOnlyMW)

	partnerOnlyMW := middleware.RequireRole("delivery_partner", "admin")
	deliveryHandler.RegisterRoutes(v1, authMW, partnerOnlyMW, ownerOnlyMW)

	notificationHandler.RegisterRoutes(v1, authMW)

	settlementHandler.RegisterRoutes(v1, authMW, adminOnlyMW, ownerOnlyMW, partnerOnlyMW)

	manageRolesMW := middleware.RequirePermission(rbacService, "admin.manage_roles")
	assignRolesMW := middleware.RequirePermission(rbacService, "admin.assign_roles")
	rbacHandler.RegisterRoutes(v1, authMW, adminOnlyMW, manageRolesMW, assignRolesMW)

	// --- search module ---
	searchRepo := searchinfra.NewRepository(queries)
	searchService := searchapp.NewSearchService(searchRepo)
	searchHandler := searchhttp.NewSearchHandler(searchService)
	optionalAuthMW := middleware.OptionalAuth(cfg.JWT.AccessSecret)
	searchHandler.RegisterRoutes(v1, optionalAuthMW, authMW, adminOnlyMW)

	srv := &http.Server{
		Addr:         ":" + cfg.HTTP.Port,
		Handler:      router,
		ReadTimeout:  cfg.HTTP.ReadTimeout,
		WriteTimeout: cfg.HTTP.WriteTimeout,
	}

	go func() {
		log.Info().Str("port", cfg.HTTP.Port).Msg("HTTP server listening")
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatal().Err(err).Msg("HTTP server failed")
		}
	}()

	<-ctx.Done()
	log.Info().Msg("shutdown signal received, draining in-flight requests")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.HTTP.ShutdownTimeout)
	defer cancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Error().Err(err).Msg("graceful shutdown failed")
		os.Exit(1)
	}
	log.Info().Msg("shutdown complete")
}
