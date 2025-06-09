package main

import (
	"context"
	"fmt"
	"net/http"
	"net/netip"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/appkins-org/ironic-metadata/api/metadata"
	"github.com/appkins-org/ironic-metadata/pkg/client"
	"github.com/gophercloud/gophercloud"
	"github.com/gophercloud/gophercloud/openstack"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

func main() {
	// Configure logging
	zerolog.TimeFieldFormat = zerolog.TimeFormatUnix

	// Configure log level from environment
	logLevel := getEnvOrDefault("LOG_LEVEL", "info")
	level, err := zerolog.ParseLevel(logLevel)
	if err != nil {
		level = zerolog.InfoLevel
		log.Warn().
			Str("invalid_level", logLevel).
			Str("default_level", "info").
			Msg("Invalid log level, using default")
	}
	zerolog.SetGlobalLevel(level)

	// Configure log output based on environment
	// In Docker/production, use JSON format to stdout
	// In development, use console format to stderr
	logFormat := getEnvOrDefault("LOG_FORMAT", "auto")

	switch logFormat {
	case "json":
		// JSON format for structured logging (good for production)
		log.Logger = log.Output(os.Stdout)
	case "console":
		// Console format for human-readable output (good for development)
		log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stdout})
	case "auto":
		// Auto-detect: use JSON in Docker, console otherwise
		if os.Getenv("DOCKER_CONTAINER") == "true" || os.Getenv("KUBERNETES_SERVICE_HOST") != "" {
			log.Logger = log.Output(os.Stdout)
		} else {
			log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stdout})
		}
	default:
		// Default to console format to stdout
		log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stdout})
	}

	// Get configuration from environment variables
	ironicURL := getEnvOrDefault("IRONIC_URL", "http://localhost:6385")
	bindAddr := getEnvOrDefault("BIND_ADDR", "0.0.0.0")
	bindPort := getEnvOrDefault("BIND_PORT", "80")

	log.Info().
		Str("ironic_url", ironicURL).
		Str("bind_addr", bindAddr).
		Str("bind_port", bindPort).
		Str("log_level", logLevel).
		Msg("Starting ironic-metadata service")

	// Initialize Ironic client
	ironicClient, err := createIronicClient(ironicURL)
	if err != nil {
		log.Fatal().
			Err(err).
			Str("ironic_url", ironicURL).
			Msg("Failed to create Ironic client")
	}

	log.Info().
		Str("ironic_endpoint", ironicClient.Endpoint).
		Msg("Successfully initialized Ironic client")

	clients := &client.Clients{}
	clients.SetIronicClient(ironicClient)

	// Create metadata handler
	handler := &metadata.Handler{
		Clients: clients,
	}

	// Parse bind address
	addr, err := netip.ParseAddrPort(fmt.Sprintf("%s:%s", bindAddr, bindPort))
	if err != nil {
		log.Fatal().
			Err(err).
			Str("bind_addr", bindAddr).
			Str("bind_port", bindPort).
			Msg("Failed to parse bind address")
	}

	// Create HTTP server
	server := &http.Server{
		Handler:      handler.Routes(),
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	// Start server in a goroutine
	go func() {
		log.Info().Str("address", addr.String()).Msg("Starting HTTP server")
		if err := metadata.ListenAndServe(context.Background(), addr, server); err != nil &&
			err != http.ErrServerClosed {
			log.Fatal().
				Err(err).
				Str("address", addr.String()).
				Msg("Failed to start server")
		}
	}()

	// Wait for interrupt signal to gracefully shutdown the server
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	sig := <-quit
	log.Info().
		Str("signal", sig.String()).
		Msg("Received shutdown signal, shutting down server...")

	// Give outstanding requests a deadline for completion
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := server.Shutdown(ctx); err != nil {
		log.Fatal().
			Err(err).
			Dur("timeout", 30*time.Second).
			Msg("Server forced to shutdown")
	}

	log.Info().Msg("Server exited gracefully")
}

func getEnvOrDefault(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func createIronicClient(ironicURL string) (*gophercloud.ServiceClient, error) {
	log.Debug().
		Str("ironic_url", ironicURL).
		Msg("Creating Ironic client")

	// Create authentication options
	authOpts := gophercloud.AuthOptions{
		IdentityEndpoint: ironicURL,
		Username:         getEnvOrDefault("OS_USERNAME", ""),
		Password:         getEnvOrDefault("OS_PASSWORD", ""),
		TenantName:       getEnvOrDefault("OS_PROJECT_NAME", ""),
		DomainName:       getEnvOrDefault("OS_USER_DOMAIN_NAME", "default"),
	}

	// If no credentials provided, try to use no-auth mode
	if authOpts.Username == "" {
		log.Info().
			Str("ironic_url", ironicURL).
			Msg("No authentication credentials provided, using no-auth mode for standalone Ironic")

		// For standalone Ironic, we might not need authentication
		provider := &gophercloud.ProviderClient{
			IdentityBase: ironicURL,
		}

		client := &gophercloud.ServiceClient{
			ProviderClient: provider,
			Endpoint:       ironicURL + "/v1/",
		}

		log.Debug().
			Str("endpoint", client.Endpoint).
			Msg("Created no-auth Ironic client")

		return client, nil
	}

	log.Info().
		Str("username", authOpts.Username).
		Str("tenant_name", authOpts.TenantName).
		Str("domain_name", authOpts.DomainName).
		Msg("Using authentication for Ironic client")

	// Use regular authentication
	provider, err := openstack.AuthenticatedClient(authOpts)
	if err != nil {
		log.Error().
			Err(err).
			Str("identity_endpoint", authOpts.IdentityEndpoint).
			Str("username", authOpts.Username).
			Msg("Failed to create authenticated OpenStack client")
		return nil, fmt.Errorf("failed to create authenticated client: %w", err)
	}

	client, err := openstack.NewBareMetalV1(provider, gophercloud.EndpointOpts{
		Region: getEnvOrDefault("OS_REGION_NAME", ""),
	})
	if err != nil {
		log.Error().
			Err(err).
			Str("region", getEnvOrDefault("OS_REGION_NAME", "")).
			Msg("Failed to create baremetal service client")
		return nil, fmt.Errorf("failed to create baremetal client: %w", err)
	}

	log.Debug().
		Str("endpoint", client.Endpoint).
		Msg("Created authenticated Ironic client")

	return client, nil
}
