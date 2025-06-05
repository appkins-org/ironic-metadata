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
	log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr})

	// Get configuration from environment variables
	ironicURL := getEnvOrDefault("IRONIC_URL", "http://localhost:6385")
	bindAddr := getEnvOrDefault("BIND_ADDR", "169.254.169.254")
	bindPort := getEnvOrDefault("BIND_PORT", "80")

	log.Info().Str("ironic_url", ironicURL).Str("bind_addr", bindAddr).Str("bind_port", bindPort).Msg("Starting ironic-metadata service")

	// Initialize Ironic client
	ironicClient, err := createIronicClient(ironicURL)
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to create Ironic client")
	}

	clients := &client.Clients{}
	clients.SetIronicClient(ironicClient)

	// Create metadata handler
	handler := &metadata.Handler{
		Clients: clients,
	}

	// Parse bind address
	addr, err := netip.ParseAddrPort(fmt.Sprintf("%s:%s", bindAddr, bindPort))
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to parse bind address")
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
		if err := metadata.ListenAndServe(context.Background(), addr, server); err != nil && err != http.ErrServerClosed {
			log.Fatal().Err(err).Msg("Failed to start server")
		}
	}()

	// Wait for interrupt signal to gracefully shutdown the server
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Info().Msg("Shutting down server...")

	// Give outstanding requests a deadline for completion
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := server.Shutdown(ctx); err != nil {
		log.Fatal().Err(err).Msg("Server forced to shutdown")
	}

	log.Info().Msg("Server exited")
}

func getEnvOrDefault(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func createIronicClient(ironicURL string) (*gophercloud.ServiceClient, error) {
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
		// For standalone Ironic, we might not need authentication
		provider := &gophercloud.ProviderClient{
			IdentityBase: ironicURL,
		}

		client := &gophercloud.ServiceClient{
			ProviderClient: provider,
			Endpoint:       ironicURL + "/v1/",
		}

		return client, nil
	}

	// Use regular authentication
	provider, err := openstack.AuthenticatedClient(authOpts)
	if err != nil {
		return nil, fmt.Errorf("failed to create authenticated client: %w", err)
	}

	client, err := openstack.NewBareMetalV1(provider, gophercloud.EndpointOpts{
		Region: getEnvOrDefault("OS_REGION_NAME", ""),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create baremetal client: %w", err)
	}

	return client, nil
}
