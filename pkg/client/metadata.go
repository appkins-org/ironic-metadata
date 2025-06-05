package client

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gophercloud/gophercloud"
	"github.com/gophercloud/gophercloud/openstack/baremetal/v1/drivers"
	"github.com/gophercloud/gophercloud/openstack/baremetal/v1/nodes"
	"github.com/gophercloud/gophercloud/pagination"
)

// Clients stores the client connection information for Ironic and Inspector
type Clients struct {
	ironic    *gophercloud.ServiceClient
	inspector *gophercloud.ServiceClient

	// Boolean that determines if Ironic API was previously determined to be available, we don't need to try every time.
	ironicUp bool

	// Boolean that determines we've already waited and the API never came up, we don't need to wait again.
	ironicFailed bool

	// Mutex so that only one resource being created by terraform checks at a time. There's no reason to have multiple
	// resources calling out to the API.
	ironicMux sync.Mutex

	// Boolean that determines if Inspector API was previously determined to be available, we don't need to try every time.
	inspectorUp bool

	// Boolean that determines that we've already waited, and inspector API did not come up.
	inspectorFailed bool

	// Mutex so that only one resource being created by terraform checks at a time. There's no reason to have multiple
	// resources calling out to the API.
	inspectorMux sync.Mutex

	timeout int
}

// GetIronicClient returns the API client for Ironic, optionally retrying to reach the API if timeout is set.
func (c *Clients) GetIronicClient() (*gophercloud.ServiceClient, error) {
	// Terraform concurrently creates some resources which means multiple callers can request an Ironic client. We
	// only need to check if the API is available once, so we use a mux to restrict one caller to polling the API.
	// When the mux is released, the other callers will fall through to the check for ironicUp.
	c.ironicMux.Lock()
	defer c.ironicMux.Unlock()

	// Ironic is UP, or user didn't ask us to check
	if c.ironicUp || c.timeout == 0 {
		return c.ironic, nil
	}

	// We previously tried and it failed.
	if c.ironicFailed {
		return nil, fmt.Errorf("could not contact Ironic API: timeout reached")
	}

	// Let's poll the API until it's up, or times out.
	duration := time.Duration(c.timeout) * time.Second
	ctx, cancel := context.WithTimeout(context.Background(), duration)
	defer cancel()

	done := make(chan struct{})
	go func() {
		log.Printf("[INFO] Waiting for Ironic API...")
		waitForAPI(ctx, c.ironic)
		log.Printf("[INFO] API successfully connected, waiting for conductor...")
		waitForConductor(ctx, c.ironic)
		close(done)
	}()

	// Wait for done or time out
	select {
	case <-ctx.Done():
		if err := ctx.Err(); err != nil {
			c.ironicFailed = true
			return nil, fmt.Errorf("could not contact Ironic API: %w", err)
		}
	case <-done:
	}

	c.ironicUp = true
	return c.ironic, ctx.Err()
}

// Retries an API forever until it responds.
func waitForAPI(ctx context.Context, client *gophercloud.ServiceClient) {
	httpClient := &http.Client{
		Timeout: 5 * time.Second,
	}

	// NOTE: Some versions of Ironic inspector returns 404 for /v1/ but 200 for /v1,
	// which seems to be the default behavior for Flask. Remove the trailing slash
	// from the client endpoint.
	endpoint := strings.TrimSuffix(client.Endpoint, "/")

	for {
		select {
		case <-ctx.Done():
			return
		default:
			log.Printf("[DEBUG] Waiting for API to become available...")

			r, err := httpClient.Get(endpoint)
			if err == nil {
				statusCode := r.StatusCode
				r.Body.Close()
				if statusCode == http.StatusOK {
					return
				}
			}

			time.Sleep(5 * time.Second)
		}
	}
}

// Ironic conductor can be considered up when the driver count returns non-zero.
func waitForConductor(ctx context.Context, client *gophercloud.ServiceClient) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
			log.Printf("[DEBUG] Waiting for conductor API to become available...")
			driverCount := 0

			err := drivers.ListDrivers(client, drivers.ListDriversOpts{
				Detail: false,
			}).EachPage(func(page pagination.Page) (bool, error) {
				actual, err := drivers.ExtractDrivers(page)
				if err != nil {
					return false, err
				}
				driverCount += len(actual)
				return true, nil
			})
			// If we have any drivers, conductor is up.
			if err == nil && driverCount > 0 {
				return
			}

			time.Sleep(5 * time.Second)
		}
	}
}

func (c *Clients) GetNodes() error {
	client, err := c.GetIronicClient()
	if err != nil {
		return err
	}
	err = nodes.List(client, nodes.ListOpts{}).EachPage(func(page pagination.Page) (bool, error) {
		results, err := nodes.ExtractNodes(page)
		if err != nil {
			return false, fmt.Errorf("could not list nodes: %s", err)
		}

		for _, node := range results {
			log.Printf("[DEBUG] Found node: %s", node.UUID)
		}
		return true, nil
	})
	if err != nil {
		return fmt.Errorf("could not list nodes: %s", err)
	}
	return nil
}

// SetIronicClient sets the Ironic client
func (c *Clients) SetIronicClient(client *gophercloud.ServiceClient) {
	c.ironic = client
	c.ironicUp = true
}
