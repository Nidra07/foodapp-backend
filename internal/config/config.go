// Package config centralizes all environment-based configuration for the
// platform. No other package should read os.Getenv directly — everything
// flows through the Config struct so behavior is explicit and testable.
package config

import (
	"fmt"
	"strings"
	"time"

	"github.com/spf13/viper"
)

type Config struct {
	App      AppConfig
	HTTP     HTTPConfig
	Postgres PostgresConfig
	Redis    RedisConfig
	JWT      JWTConfig
	OTP      OTPConfig
	S3       S3Config
	SMS      SMSConfig
	Email    EmailConfig
	RateLimit RateLimitConfig
}

type AppConfig struct {
	Env         string // local | staging | production
	Name        string
	Version     string
	LogLevel    string // debug | info | warn | error
	LogFormat   string // json | console
}

type HTTPConfig struct {
	Port            string
	ReadTimeout     time.Duration
	WriteTimeout    time.Duration
	ShutdownTimeout time.Duration
	AllowedOrigins  []string
}

type PostgresConfig struct {
	Host            string
	Port            string
	User            string
	Password        string
	DBName          string
	SSLMode         string
	MaxOpenConns    int32
	MaxIdleConns    int32
	ConnMaxLifetime time.Duration
}

func (p PostgresConfig) DSN() string {
	return fmt.Sprintf(
		"postgres://%s:%s@%s:%s/%s?sslmode=%s",
		p.User, p.Password, p.Host, p.Port, p.DBName, p.SSLMode,
	)
}

type RedisConfig struct {
	Addr     string
	Password string
	DB       int
}

type JWTConfig struct {
	AccessSecret         string
	RefreshSecret        string
	AccessTokenTTL       time.Duration
	RefreshTokenTTL      time.Duration
	Issuer               string
}

type OTPConfig struct {
	Length            int
	TTL               time.Duration
	MaxAttempts       int
	ResendCooldown    time.Duration
	MaxRequestsPerDay int
}

type S3Config struct {
	Endpoint        string
	Region          string
	Bucket          string
	AccessKeyID     string
	SecretAccessKey string
	UseSSL          bool
	PresignTTL      time.Duration
}

type SMSConfig struct {
	Provider string // "msg91" | "twilio" | "mock"
	APIKey   string
	SenderID string
}

type EmailConfig struct {
	Provider string // "ses" | "sendgrid" | "mock"
	APIKey   string
	FromAddr string
}

type RateLimitConfig struct {
	OTPRequestsPerHour int
	LoginAttemptsPerHour int
	GlobalRPS          int
}

// Load reads configuration from environment variables (and an optional
// .env file for local development). It applies safe defaults for local
// dev but requires secrets to be explicitly set outside of "local" env.
func Load() (*Config, error) {
	v := viper.New()
	v.SetConfigFile(".env")
	v.SetConfigType("env")
	v.AutomaticEnv()
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	_ = v.ReadInConfig() // .env is optional; real deployments use real env vars

	setDefaults(v)

	cfg := &Config{
		App: AppConfig{
			Env:       v.GetString("APP_ENV"),
			Name:      v.GetString("APP_NAME"),
			Version:   v.GetString("APP_VERSION"),
			LogLevel:  v.GetString("LOG_LEVEL"),
			LogFormat: v.GetString("LOG_FORMAT"),
		},
		HTTP: HTTPConfig{
			Port:            v.GetString("HTTP_PORT"),
			ReadTimeout:     v.GetDuration("HTTP_READ_TIMEOUT"),
			WriteTimeout:    v.GetDuration("HTTP_WRITE_TIMEOUT"),
			ShutdownTimeout: v.GetDuration("HTTP_SHUTDOWN_TIMEOUT"),
			AllowedOrigins:  strings.Split(v.GetString("HTTP_ALLOWED_ORIGINS"), ","),
		},
		Postgres: PostgresConfig{
			Host:            v.GetString("POSTGRES_HOST"),
			Port:            v.GetString("POSTGRES_PORT"),
			User:            v.GetString("POSTGRES_USER"),
			Password:        v.GetString("POSTGRES_PASSWORD"),
			DBName:          v.GetString("POSTGRES_DB"),
			SSLMode:         v.GetString("POSTGRES_SSLMODE"),
			MaxOpenConns:    v.GetInt32("POSTGRES_MAX_OPEN_CONNS"),
			MaxIdleConns:    v.GetInt32("POSTGRES_MAX_IDLE_CONNS"),
			ConnMaxLifetime: v.GetDuration("POSTGRES_CONN_MAX_LIFETIME"),
		},
		Redis: RedisConfig{
			Addr:     v.GetString("REDIS_ADDR"),
			Password: v.GetString("REDIS_PASSWORD"),
			DB:       v.GetInt("REDIS_DB"),
		},
		JWT: JWTConfig{
			AccessSecret:    v.GetString("JWT_ACCESS_SECRET"),
			RefreshSecret:   v.GetString("JWT_REFRESH_SECRET"),
			AccessTokenTTL:  v.GetDuration("JWT_ACCESS_TOKEN_TTL"),
			RefreshTokenTTL: v.GetDuration("JWT_REFRESH_TOKEN_TTL"),
			Issuer:          v.GetString("JWT_ISSUER"),
		},
		OTP: OTPConfig{
			Length:            v.GetInt("OTP_LENGTH"),
			TTL:               v.GetDuration("OTP_TTL"),
			MaxAttempts:       v.GetInt("OTP_MAX_ATTEMPTS"),
			ResendCooldown:    v.GetDuration("OTP_RESEND_COOLDOWN"),
			MaxRequestsPerDay: v.GetInt("OTP_MAX_REQUESTS_PER_DAY"),
		},
		S3: S3Config{
			Endpoint:        v.GetString("S3_ENDPOINT"),
			Region:          v.GetString("S3_REGION"),
			Bucket:          v.GetString("S3_BUCKET"),
			AccessKeyID:     v.GetString("S3_ACCESS_KEY_ID"),
			SecretAccessKey: v.GetString("S3_SECRET_ACCESS_KEY"),
			UseSSL:          v.GetBool("S3_USE_SSL"),
			PresignTTL:      v.GetDuration("S3_PRESIGN_TTL"),
		},
		SMS: SMSConfig{
			Provider: v.GetString("SMS_PROVIDER"),
			APIKey:   v.GetString("SMS_API_KEY"),
			SenderID: v.GetString("SMS_SENDER_ID"),
		},
		Email: EmailConfig{
			Provider: v.GetString("EMAIL_PROVIDER"),
			APIKey:   v.GetString("EMAIL_API_KEY"),
			FromAddr: v.GetString("EMAIL_FROM_ADDR"),
		},
		RateLimit: RateLimitConfig{
			OTPRequestsPerHour:   v.GetInt("RATE_LIMIT_OTP_PER_HOUR"),
			LoginAttemptsPerHour: v.GetInt("RATE_LIMIT_LOGIN_PER_HOUR"),
			GlobalRPS:            v.GetInt("RATE_LIMIT_GLOBAL_RPS"),
		},
	}

	if err := cfg.validate(); err != nil {
		return nil, fmt.Errorf("config validation failed: %w", err)
	}

	return cfg, nil
}

func (c *Config) validate() error {
	if c.App.Env != "local" {
		if c.JWT.AccessSecret == "" || c.JWT.RefreshSecret == "" {
			return fmt.Errorf("JWT secrets must be explicitly set outside local env")
		}
		if c.Postgres.Password == "" {
			return fmt.Errorf("POSTGRES_PASSWORD must be set outside local env")
		}
	}
	return nil
}

func (c *Config) IsProduction() bool { return c.App.Env == "production" }
func (c *Config) IsLocal() bool      { return c.App.Env == "local" }

func setDefaults(v *viper.Viper) {
	v.SetDefault("APP_ENV", "local")
	v.SetDefault("APP_NAME", "foodapp-backend")
	v.SetDefault("APP_VERSION", "0.1.0")
	v.SetDefault("LOG_LEVEL", "debug")
	v.SetDefault("LOG_FORMAT", "console")

	v.SetDefault("HTTP_PORT", "8080")
	v.SetDefault("HTTP_READ_TIMEOUT", "15s")
	v.SetDefault("HTTP_WRITE_TIMEOUT", "15s")
	v.SetDefault("HTTP_SHUTDOWN_TIMEOUT", "10s")
	v.SetDefault("HTTP_ALLOWED_ORIGINS", "http://localhost:3000")

	v.SetDefault("POSTGRES_HOST", "localhost")
	v.SetDefault("POSTGRES_PORT", "5432")
	v.SetDefault("POSTGRES_USER", "foodapp")
	v.SetDefault("POSTGRES_PASSWORD", "foodapp_local_pw")
	v.SetDefault("POSTGRES_DB", "foodapp")
	v.SetDefault("POSTGRES_SSLMODE", "disable")
	v.SetDefault("POSTGRES_MAX_OPEN_CONNS", 25)
	v.SetDefault("POSTGRES_MAX_IDLE_CONNS", 10)
	v.SetDefault("POSTGRES_CONN_MAX_LIFETIME", "30m")

	v.SetDefault("REDIS_ADDR", "localhost:6379")
	v.SetDefault("REDIS_PASSWORD", "")
	v.SetDefault("REDIS_DB", 0)

	v.SetDefault("JWT_ACCESS_SECRET", "local_dev_access_secret_change_me")
	v.SetDefault("JWT_REFRESH_SECRET", "local_dev_refresh_secret_change_me")
	v.SetDefault("JWT_ACCESS_TOKEN_TTL", "15m")
	v.SetDefault("JWT_REFRESH_TOKEN_TTL", "720h") // 30 days
	v.SetDefault("JWT_ISSUER", "foodapp")

	v.SetDefault("OTP_LENGTH", 6)
	v.SetDefault("OTP_TTL", "5m")
	v.SetDefault("OTP_MAX_ATTEMPTS", 5)
	v.SetDefault("OTP_RESEND_COOLDOWN", "30s")
	v.SetDefault("OTP_MAX_REQUESTS_PER_DAY", 10)

	v.SetDefault("S3_ENDPOINT", "localhost:9000")
	v.SetDefault("S3_REGION", "us-east-1")
	v.SetDefault("S3_BUCKET", "foodapp-media")
	v.SetDefault("S3_USE_SSL", false)
	v.SetDefault("S3_PRESIGN_TTL", "15m")

	v.SetDefault("SMS_PROVIDER", "mock")
	v.SetDefault("EMAIL_PROVIDER", "mock")

	v.SetDefault("RATE_LIMIT_OTP_PER_HOUR", 5)
	v.SetDefault("RATE_LIMIT_LOGIN_PER_HOUR", 10)
	v.SetDefault("RATE_LIMIT_GLOBAL_RPS", 50)
}
