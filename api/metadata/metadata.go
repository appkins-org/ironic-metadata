package metadata

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"strings"
	"time"

	"github.com/appkins-org/ironic-metadata/pkg/client"
	"github.com/appkins-org/ironic-metadata/pkg/metadata"
	"github.com/gophercloud/gophercloud/openstack/baremetal/v1/nodes"
	"github.com/gophercloud/gophercloud/pagination"
	"github.com/gorilla/mux"
	"github.com/rs/zerolog/log"
)

// Handler is the struct that implements the http.Handler interface.
type Handler struct {
	Clients *client.Clients
}

// Routes sets up the HTTP routes for the metadata service
func (h *Handler) Routes() http.Handler {
	r := mux.NewRouter()

	// OpenStack metadata service routes
	r.HandleFunc("/openstack", h.handleOpenStackRoot).Methods("GET")
	r.HandleFunc("/openstack/", h.handleOpenStackRoot).Methods("GET")
	r.HandleFunc("/openstack/latest", h.handleLatestRoot).Methods("GET")
	r.HandleFunc("/openstack/latest/", h.handleLatestRoot).Methods("GET")
	r.HandleFunc("/openstack/latest/meta_data.json", h.handleMetaData).Methods("GET")
	r.HandleFunc("/openstack/latest/network_data.json", h.handleNetworkData).Methods("GET")
	r.HandleFunc("/openstack/latest/user_data", h.handleUserData).Methods("GET")
	r.HandleFunc("/openstack/latest/vendor_data.json", h.handleVendorData).Methods("GET")
	r.HandleFunc("/openstack/latest/vendor_data2.json", h.handleVendorData2).Methods("GET")

	// EC2-compatible routes for compatibility
	r.HandleFunc("/", h.handleEC2Root).Methods("GET")
	r.HandleFunc("/latest", h.handleEC2Latest).Methods("GET")
	r.HandleFunc("/latest/", h.handleEC2Latest).Methods("GET")
	r.HandleFunc("/latest/meta-data", h.handleEC2MetaData).Methods("GET")
	r.HandleFunc("/latest/meta-data/", h.handleEC2MetaData).Methods("GET")
	r.HandleFunc("/latest/user-data", h.handleUserData).Methods("GET")

	// Add middleware for logging and client IP detection
	r.Use(h.loggingMiddleware)
	r.Use(h.clientIPMiddleware)

	return r
}

// loggingMiddleware logs incoming requests
func (h *Handler) loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		log.Info().
			Str("method", r.Method).
			Str("path", r.URL.Path).
			Str("remote_addr", r.RemoteAddr).
			Str("user_agent", r.UserAgent()).
			Dur("duration", time.Since(start)).
			Msg("HTTP request")
	})
}

// clientIPMiddleware extracts the client IP and stores it in the request context
func (h *Handler) clientIPMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		clientIP := getClientIP(r)
		ctx := context.WithValue(r.Context(), "client_ip", clientIP)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// getClientIP extracts the real client IP from the request
func getClientIP(r *http.Request) string {
	// Check X-Forwarded-For header first
	xff := r.Header.Get("X-Forwarded-For")
	if xff != "" {
		ips := strings.Split(xff, ",")
		return strings.TrimSpace(ips[0])
	}

	// Check X-Real-IP header
	xri := r.Header.Get("X-Real-IP")
	if xri != "" {
		return xri
	}

	// Fall back to remote address
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// handleOpenStackRoot handles requests to /openstack
func (h *Handler) handleOpenStackRoot(w http.ResponseWriter, r *http.Request) {
	versions := []string{"latest"}
	h.writeJSONResponse(w, versions)
}

// handleLatestRoot handles requests to /openstack/latest
func (h *Handler) handleLatestRoot(w http.ResponseWriter, r *http.Request) {
	endpoints := []string{
		"meta_data.json",
		"network_data.json",
		"user_data",
		"vendor_data.json",
		"vendor_data2.json",
	}
	h.writeJSONResponse(w, endpoints)
}

// handleMetaData handles requests to /openstack/latest/meta_data.json
func (h *Handler) handleMetaData(w http.ResponseWriter, r *http.Request) {
	clientIP := r.Context().Value("client_ip").(string)

	node, err := h.getNodeByIP(clientIP)
	if err != nil {
		log.Error().Err(err).Str("client_ip", clientIP).Msg("Failed to find node for client IP")
		http.Error(w, "Node not found", http.StatusNotFound)
		return
	}

	metaData := h.buildMetaData(node)
	h.writeJSONResponse(w, metaData)
}

// handleNetworkData handles requests to /openstack/latest/network_data.json
func (h *Handler) handleNetworkData(w http.ResponseWriter, r *http.Request) {
	clientIP := r.Context().Value("client_ip").(string)

	node, err := h.getNodeByIP(clientIP)
	if err != nil {
		log.Error().Err(err).Str("client_ip", clientIP).Msg("Failed to find node for client IP")
		http.Error(w, "Node not found", http.StatusNotFound)
		return
	}

	networkData := h.buildNetworkData(node)
	h.writeJSONResponse(w, networkData)
}

// handleUserData handles requests to /openstack/latest/user_data
func (h *Handler) handleUserData(w http.ResponseWriter, r *http.Request) {
	clientIP := r.Context().Value("client_ip").(string)

	node, err := h.getNodeByIP(clientIP)
	if err != nil {
		log.Error().Err(err).Str("client_ip", clientIP).Msg("Failed to find node for client IP")
		http.Error(w, "Node not found", http.StatusNotFound)
		return
	}

	userData := h.getUserData(node)
	if userData == "" {
		http.Error(w, "User data not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "text/plain")
	w.Write([]byte(userData))
}

// handleVendorData handles requests to /openstack/latest/vendor_data.json
func (h *Handler) handleVendorData(w http.ResponseWriter, r *http.Request) {
	vendorData := map[string]interface{}{
		"ironic": map[string]interface{}{
			"version": "1.0",
		},
	}
	h.writeJSONResponse(w, vendorData)
}

// handleVendorData2 handles requests to /openstack/latest/vendor_data2.json
func (h *Handler) handleVendorData2(w http.ResponseWriter, r *http.Request) {
	vendorData := map[string]interface{}{
		"static": map[string]interface{}{
			"ironic-metadata": map[string]interface{}{
				"version": "1.0",
			},
		},
	}
	h.writeJSONResponse(w, vendorData)
}

// handleEC2Root handles EC2-compatible root requests
func (h *Handler) handleEC2Root(w http.ResponseWriter, r *http.Request) {
	versions := []string{"latest"}
	h.writeTextResponse(w, strings.Join(versions, "\n"))
}

// handleEC2Latest handles EC2-compatible latest requests
func (h *Handler) handleEC2Latest(w http.ResponseWriter, r *http.Request) {
	endpoints := []string{
		"meta-data/",
		"user-data",
	}
	h.writeTextResponse(w, strings.Join(endpoints, "\n"))
}

// handleEC2MetaData handles EC2-compatible meta-data requests
func (h *Handler) handleEC2MetaData(w http.ResponseWriter, r *http.Request) {
	clientIP := r.Context().Value("client_ip").(string)

	node, err := h.getNodeByIP(clientIP)
	if err != nil {
		log.Error().Err(err).Str("client_ip", clientIP).Msg("Failed to find node for client IP")
		http.Error(w, "Node not found", http.StatusNotFound)
		return
	}

	// EC2-style metadata
	ec2Data := []string{
		fmt.Sprintf("instance-id\n%s", node.UUID),
		fmt.Sprintf("hostname\n%s", getNodeHostname(node)),
		fmt.Sprintf("local-ipv4\n%s", clientIP),
	}

	h.writeTextResponse(w, strings.Join(ec2Data, "\n"))
}

// getNodeByIP finds a node based on the client IP address
func (h *Handler) getNodeByIP(clientIP string) (*nodes.Node, error) {
	ironicClient, err := h.Clients.GetIronicClient()
	if err != nil {
		return nil, fmt.Errorf("failed to get Ironic client: %w", err)
	}

	var foundNode *nodes.Node
	err = nodes.List(ironicClient, nodes.ListOpts{}).EachPage(func(page pagination.Page) (bool, error) {
		nodeList, err := nodes.ExtractNodes(page)
		if err != nil {
			return false, err
		}

		for _, node := range nodeList {
			// Check provisioning network IP
			if provisioningIP, ok := node.DriverInfo["deploy_ramdisk_address"]; ok {
				if provisioningIP == clientIP {
					foundNode = &node
					return false, nil // Stop iteration
				}
			}

			// Check if the IP matches any of the node's ports
			if nodeMatches, _ := h.nodeMatchesIP(&node, clientIP); nodeMatches {
				foundNode = &node
				return false, nil // Stop iteration
			}
		}
		return true, nil
	})

	if err != nil {
		return nil, fmt.Errorf("failed to list nodes: %w", err)
	}

	if foundNode == nil {
		return nil, fmt.Errorf("no node found for IP %s", clientIP)
	}

	return foundNode, nil
}

// nodeMatchesIP checks if a node matches the given IP address
func (h *Handler) nodeMatchesIP(node *nodes.Node, clientIP string) (bool, error) {
	// This is a simplified implementation - in a real deployment,
	// you would need to check the node's port information via the
	// Ironic API to see if any ports have the matching IP

	// For now, we'll check some common fields where IP might be stored
	if instanceInfo, ok := node.InstanceInfo["configdrive"]; ok {
		if configMap, ok := instanceInfo.(map[string]interface{}); ok {
			if networkData, ok := configMap["network_data"]; ok {
				// Parse network data and check IPs
				_ = networkData // TODO: Implement proper network data parsing
			}
		}
	}

	return false, nil
}

// buildMetaData constructs the metadata response for a node
func (h *Handler) buildMetaData(node *nodes.Node) *metadata.MetaData {
	metaData := &metadata.MetaData{
		UUID:         node.UUID,
		Name:         node.Name,
		Hostname:     getNodeHostname(node),
		LaunchIndex:  0,
		PublicKeys:   make(map[string]string),
		Meta:         make(map[string]string),
		Keys:         []metadata.Key{},
		ProjectID:    getProjectID(node),
		CreationTime: &node.CreatedAt,
	}

	// Extract public keys from instance info
	if instanceInfo, ok := node.InstanceInfo["public_keys"]; ok {
		if keys, ok := instanceInfo.(map[string]interface{}); ok {
			for name, key := range keys {
				if keyStr, ok := key.(string); ok {
					metaData.PublicKeys[name] = keyStr
				}
			}
		}
	}

	// Extract metadata from node properties
	for key, value := range node.Properties {
		if strValue, ok := value.(string); ok {
			metaData.Meta[key] = strValue
		}
	}

	return metaData
}

// buildNetworkData constructs the network data response for a node
func (h *Handler) buildNetworkData(node *nodes.Node) *metadata.NetworkData {
	networkData := &metadata.NetworkData{
		Links:    []metadata.Link{},
		Networks: []metadata.Network{},
		Services: []metadata.Service{},
	}

	// Extract network configuration from instance info
	if instanceInfo, ok := node.InstanceInfo["network_data"]; ok {
		if netData, ok := instanceInfo.(map[string]interface{}); ok {
			// Parse the network data - this is a simplified version
			// In a real implementation, you'd need to parse the full network_data structure
			_ = netData // TODO: Implement proper network data parsing
		}
	}

	// For now, create a basic network configuration
	networkData.Links = append(networkData.Links, metadata.Link{
		ID:   "eth0",
		Type: "physical",
		MTU:  1500,
	})

	networkData.Networks = append(networkData.Networks, metadata.Network{
		ID:   "network0",
		Type: "ipv4",
		Link: "eth0",
	})

	return networkData
}

// getUserData extracts user data from the node
func (h *Handler) getUserData(node *nodes.Node) string {
	if instanceInfo, ok := node.InstanceInfo["user_data"]; ok {
		if userData, ok := instanceInfo.(string); ok {
			return userData
		}
	}
	return ""
}

// Helper functions
func getNodeHostname(node *nodes.Node) string {
	if node.Name != "" {
		return node.Name
	}
	return node.UUID
}

func getProjectID(node *nodes.Node) string {
	return node.Owner
}

// writeJSONResponse writes a JSON response
func (h *Handler) writeJSONResponse(w http.ResponseWriter, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(data); err != nil {
		log.Error().Err(err).Msg("Failed to encode JSON response")
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
	}
}

// writeTextResponse writes a plain text response
func (h *Handler) writeTextResponse(w http.ResponseWriter, data string) {
	w.Header().Set("Content-Type", "text/plain")
	w.Write([]byte(data))
}

// ListenAndServe is a patterned after http.ListenAndServe.
// It listens on the TCP network address srv.Addr and then
// calls ServeHTTP to handle requests on incoming connections.
//
// ListenAndServe always returns a non-nil error. After Shutdown or Close,
// the returned error is http.ErrServerClosed.
func ListenAndServe(ctx context.Context, addr netip.AddrPort, h *http.Server) error {
	conn, err := net.Listen("tcp", addr.String())
	if err != nil {
		return err
	}
	return Serve(ctx, conn, h)
}

// Serve is patterned after http.Serve.
// It accepts incoming connections on the Listener conn and serves them
// using the Server h.
//
// Serve always returns a non-nil error and closes conn.
// After Shutdown or Close, the returned error is http.ErrServerClosed.
func Serve(_ context.Context, conn net.Listener, h *http.Server) error {
	return h.Serve(conn)
}
