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
	"github.com/gorilla/mux"
	"github.com/rs/zerolog/log"
)

// ContextKey is a custom type for context keys to avoid collisions.
type ContextKey string

const (
	// ClientIPKey is the context key for storing client IP.
	ClientIPKey ContextKey = "client_ip"
)

// Handler is the struct that implements the http.Handler interface.
type Handler struct {
	Clients *client.Clients
}

// Routes sets up the HTTP routes for the metadata service.
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

// loggingMiddleware logs incoming requests.
func (h *Handler) loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		// Create a response writer wrapper to capture status code
		wrapped := &responseWriter{ResponseWriter: w, statusCode: 200}

		next.ServeHTTP(wrapped, r)

		// Log with comprehensive information
		logEvent := log.Info()
		if wrapped.statusCode >= 400 {
			logEvent = log.Error()
		} else if wrapped.statusCode >= 300 {
			logEvent = log.Warn()
		}

		logEvent.
			Str("method", r.Method).
			Str("path", r.URL.Path).
			Str("query", r.URL.RawQuery).
			Str("remote_addr", r.RemoteAddr).
			Str("user_agent", r.UserAgent()).
			Str("x_forwarded_for", r.Header.Get("X-Forwarded-For")).
			Str("x_real_ip", r.Header.Get("X-Real-IP")).
			Int("status_code", wrapped.statusCode).
			Int64("response_size", wrapped.responseSize).
			Dur("duration", time.Since(start)).
			Msg("HTTP request")
	})
}

// responseWriter wraps http.ResponseWriter to capture status code and response size.
type responseWriter struct {
	http.ResponseWriter
	statusCode   int
	responseSize int64
}

func (rw *responseWriter) WriteHeader(statusCode int) {
	rw.statusCode = statusCode
	rw.ResponseWriter.WriteHeader(statusCode)
}

func (rw *responseWriter) Write(b []byte) (int, error) {
	size, err := rw.ResponseWriter.Write(b)
	rw.responseSize += int64(size)
	return size, err
}

// clientIPMiddleware extracts the client IP and stores it in the request context.
func (h *Handler) clientIPMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		clientIP := getClientIP(r)
		ctx := context.WithValue(r.Context(), ClientIPKey, clientIP)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// getClientIP extracts the real client IP from the request.
func getClientIP(r *http.Request) string {
	// Check X-Forwarded-For header first
	xff := r.Header.Get("X-Forwarded-For")
	if xff != "" {
		ips := strings.Split(xff, ",")
		clientIP := strings.TrimSpace(ips[0])
		log.Debug().
			Str("x_forwarded_for", xff).
			Str("extracted_ip", clientIP).
			Msg("Using IP from X-Forwarded-For header")
		return clientIP
	}

	// Check X-Real-IP header
	xri := r.Header.Get("X-Real-IP")
	if xri != "" {
		log.Debug().
			Str("x_real_ip", xri).
			Msg("Using IP from X-Real-IP header")
		return xri
	}

	// Fall back to remote address
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		log.Warn().
			Err(err).
			Str("remote_addr", r.RemoteAddr).
			Msg("Failed to split host:port from remote address, using as-is")
		return r.RemoteAddr
	}

	log.Debug().
		Str("remote_addr", r.RemoteAddr).
		Str("extracted_host", host).
		Msg("Using IP from remote address")
	return host
}

// getClientIPFromContext safely extracts the client IP from the request context.
func getClientIPFromContext(r *http.Request) (string, error) {
	clientIP, ok := r.Context().Value(ClientIPKey).(string)
	if !ok {
		return "", fmt.Errorf("client IP not found in context")
	}
	return clientIP, nil
}

// handleOpenStackRoot handles requests to /openstack.
func (h *Handler) handleOpenStackRoot(w http.ResponseWriter, r *http.Request) {
	versions := []string{"latest"}
	h.writeJSONResponse(w, versions)
}

// handleLatestRoot handles requests to /openstack/latest.
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

// handleMetaData handles requests to /openstack/latest/meta_data.json.
func (h *Handler) handleMetaData(w http.ResponseWriter, r *http.Request) {
	clientIP, err := getClientIPFromContext(r)
	if err != nil {
		log.Error().
			Err(err).
			Str("request_path", r.URL.Path).
			Str("method", r.Method).
			Msg("Failed to get client IP from context")
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	log.Debug().
		Str("client_ip", clientIP).
		Str("endpoint", "meta_data.json").
		Msg("Processing metadata request")

	node, err := h.getNodeByIP(clientIP)
	if err != nil {
		log.Error().
			Err(err).
			Str("client_ip", clientIP).
			Str("endpoint", "meta_data.json").
			Msg("Failed to find node for client IP")
		http.Error(w, "Node not found", http.StatusNotFound)
		return
	}

	log.Info().
		Str("client_ip", clientIP).
		Str("node_uuid", node.UUID).
		Str("node_name", node.Name).
		Str("endpoint", "meta_data.json").
		Msg("Successfully matched client IP to node")

	metaData := h.buildMetaData(node)
	h.writeJSONResponse(w, metaData)
}

// handleNetworkData handles requests to /openstack/latest/network_data.json.
func (h *Handler) handleNetworkData(w http.ResponseWriter, r *http.Request) {
	clientIP, err := getClientIPFromContext(r)
	if err != nil {
		log.Error().
			Err(err).
			Str("request_path", r.URL.Path).
			Str("method", r.Method).
			Msg("Failed to get client IP from context")
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	log.Debug().
		Str("client_ip", clientIP).
		Str("endpoint", "network_data.json").
		Msg("Processing network data request")

	node, err := h.getNodeByIP(clientIP)
	if err != nil {
		log.Error().
			Err(err).
			Str("client_ip", clientIP).
			Str("endpoint", "network_data.json").
			Msg("Failed to find node for client IP")
		http.Error(w, "Node not found", http.StatusNotFound)
		return
	}

	log.Info().
		Str("client_ip", clientIP).
		Str("node_uuid", node.UUID).
		Str("node_name", node.Name).
		Str("endpoint", "network_data.json").
		Msg("Successfully matched client IP to node")

	networkData := h.buildNetworkData(node)
	h.writeJSONResponse(w, networkData)
}

// handleUserData handles requests to /openstack/latest/user_data.
func (h *Handler) handleUserData(w http.ResponseWriter, r *http.Request) {
	clientIP, err := getClientIPFromContext(r)
	if err != nil {
		log.Error().
			Err(err).
			Str("request_path", r.URL.Path).
			Str("method", r.Method).
			Msg("Failed to get client IP from context")
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	log.Debug().
		Str("client_ip", clientIP).
		Str("endpoint", "user_data").
		Msg("Processing user data request")

	node, err := h.getNodeByIP(clientIP)
	if err != nil {
		log.Error().
			Err(err).
			Str("client_ip", clientIP).
			Str("endpoint", "user_data").
			Msg("Failed to find node for client IP")
		http.Error(w, "Node not found", http.StatusNotFound)
		return
	}

	log.Info().
		Str("client_ip", clientIP).
		Str("node_uuid", node.UUID).
		Str("node_name", node.Name).
		Str("endpoint", "user_data").
		Msg("Successfully matched client IP to node")

	userData := h.getUserData(node)
	if userData == "" {
		log.Warn().
			Str("client_ip", clientIP).
			Str("node_uuid", node.UUID).
			Str("node_name", node.Name).
			Msg("No user data found for node")
		http.Error(w, "User data not found", http.StatusNotFound)
		return
	}

	log.Debug().
		Str("client_ip", clientIP).
		Str("node_uuid", node.UUID).
		Int("user_data_length", len(userData)).
		Msg("Returning user data")

	w.Header().Set("Content-Type", "text/plain")
	if _, err := w.Write([]byte(userData)); err != nil {
		log.Error().
			Err(err).
			Str("client_ip", clientIP).
			Str("node_uuid", node.UUID).
			Msg("Failed to write user data response")
	}
}

// handleVendorData handles requests to /openstack/latest/vendor_data.json.
func (h *Handler) handleVendorData(w http.ResponseWriter, r *http.Request) {
	vendorData := map[string]any{
		"ironic": map[string]any{
			"version": "1.0",
		},
	}
	h.writeJSONResponse(w, vendorData)
}

// handleVendorData2 handles requests to /openstack/latest/vendor_data2.json.
func (h *Handler) handleVendorData2(w http.ResponseWriter, r *http.Request) {
	vendorData := map[string]any{
		"static": map[string]any{
			"ironic-metadata": map[string]any{
				"version": "1.0",
			},
		},
	}
	h.writeJSONResponse(w, vendorData)
}

// handleEC2Root handles EC2-compatible root requests.
func (h *Handler) handleEC2Root(w http.ResponseWriter, r *http.Request) {
	versions := []string{"latest"}
	h.writeTextResponse(w, strings.Join(versions, "\n"))
}

// handleEC2Latest handles EC2-compatible latest requests.
func (h *Handler) handleEC2Latest(w http.ResponseWriter, r *http.Request) {
	endpoints := []string{
		"meta-data/",
		"user-data",
	}
	h.writeTextResponse(w, strings.Join(endpoints, "\n"))
}

// handleEC2MetaData handles EC2-compatible meta-data requests.
func (h *Handler) handleEC2MetaData(w http.ResponseWriter, r *http.Request) {
	clientIP, err := getClientIPFromContext(r)
	if err != nil {
		log.Error().
			Err(err).
			Str("request_path", r.URL.Path).
			Str("method", r.Method).
			Msg("Failed to get client IP from context")
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	log.Debug().
		Str("client_ip", clientIP).
		Str("endpoint", "ec2_meta_data").
		Msg("Processing EC2-compatible metadata request")

	node, err := h.getNodeByIP(clientIP)
	if err != nil {
		log.Error().
			Err(err).
			Str("client_ip", clientIP).
			Str("endpoint", "ec2_meta_data").
			Msg("Failed to find node for client IP")
		http.Error(w, "Node not found", http.StatusNotFound)
		return
	}

	log.Info().
		Str("client_ip", clientIP).
		Str("node_uuid", node.UUID).
		Str("node_name", node.Name).
		Str("endpoint", "ec2_meta_data").
		Msg("Successfully matched client IP to node")

	// EC2-style metadata
	ec2Data := []string{
		fmt.Sprintf("instance-id\n%s", node.UUID),
		fmt.Sprintf("hostname\n%s", getNodeHostname(node)),
		fmt.Sprintf("local-ipv4\n%s", clientIP),
	}

	h.writeTextResponse(w, strings.Join(ec2Data, "\n"))
}

// extractFromConfigDrive attempts to extract data from a node's configdrive.
func (h *Handler) extractFromConfigDrive(node *nodes.Node) (*configDriveData, error) {
	configDriveInfo, exists := node.InstanceInfo["configdrive"]
	if !exists {
		log.Debug().
			Str("node_uuid", node.UUID).
			Str("node_name", node.Name).
			Msg("No configdrive found in instance_info")
		return nil, fmt.Errorf("no configdrive found")
	}

	// Try to parse as configdrive URL or path first
	if configDriveStr, ok := configDriveInfo.(string); ok {
		log.Debug().
			Str("configdrive", configDriveStr).
			Str("node_uuid", node.UUID).
			Msg("Found configdrive string")

		// If it's an ISO file path or URL, we would need to download and parse it
		// For now, we'll assume it's a JSON string or try to parse it as such
		if strings.HasPrefix(configDriveStr, "{") {
			// Try to parse as JSON
			var configData configDriveData
			if err := json.Unmarshal([]byte(configDriveStr), &configData); err == nil {
				log.Debug().
					Str("node_uuid", node.UUID).
					Msg("Successfully parsed configdrive as JSON string")
				return nil, fmt.Errorf("configdrive is a JSON string, not a file path or URL")
			} else {
				log.Error().
					Err(err).
					Str("node_uuid", node.UUID).
					Str("configdrive_content", configDriveStr).
					Msg("Failed to parse configdrive JSON string")
			}
		}

		// For ISO files, we would use utils.ConfigDrive to parse
		// This is a placeholder for ISO parsing functionality
		log.Warn().
			Str("node_uuid", node.UUID).
			Str("configdrive", configDriveStr).
			Msg("ISO configdrive parsing not yet implemented")
		return nil, fmt.Errorf("ISO configdrive parsing not yet implemented")
	}

	dataBytes, err := json.Marshal(configDriveInfo)
	if err != nil {
		log.Error().
			Err(err).
			Str("node_uuid", node.UUID).
			Msg("Failed to marshal configdrive info")
		return nil, fmt.Errorf("failed to marshal configdrive info: %w", err)
	}
	resData := configDriveData{}
	err = json.Unmarshal(dataBytes, &resData)
	if err != nil {
		log.Error().
			Err(err).
			Str("node_uuid", node.UUID).
			Msg("Failed to unmarshal configdrive data")
		return nil, fmt.Errorf("failed to unmarshal configdrive data: %w", err)
	}
	return &resData, nil
}

// configDriveData holds extracted configdrive information.
type configDriveData struct {
	MetaData    *metadata.MetaData    `json:"meta_data,omitempty"`
	UserData    string                `json:"user_data,omitempty"`
	NetworkData *metadata.NetworkData `json:"network_data,omitempty"`
	VendorData  map[string]any        `json:"vendor_data,omitempty"`
	PublicKeys  map[string]string     `json:"public_keys,omitempty"`
}

// buildMetaData constructs the metadata response for a node.
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

	// Try to extract from configdrive first
	if configDriveData, err := h.extractFromConfigDrive(node); err == nil {
		log.Debug().Str("node_uuid", node.UUID).Msg("Using configdrive metadata")

		// Use configdrive metadata if available
		if configDriveData.MetaData != nil {
			metaData.InstanceType = configDriveData.MetaData.InstanceType
			if configDriveData.MetaData.Hostname != "" {
				metaData.Hostname = configDriveData.MetaData.Hostname
			}
		}

		// Use configdrive public keys if available
		if len(configDriveData.PublicKeys) > 0 {
			metaData.PublicKeys = configDriveData.PublicKeys
		}

		return metaData
	}

	// Fallback to dynamic config from instance info
	log.Debug().Str("node_uuid", node.UUID).Msg("Using dynamic metadata")

	// Extract public keys from instance info
	if instanceInfo, ok := node.InstanceInfo["public_keys"]; ok {
		if keys, ok := instanceInfo.(map[string]any); ok {
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

// buildNetworkData constructs the network data response for a node.
func (h *Handler) buildNetworkData(node *nodes.Node) *metadata.NetworkData {
	networkData := &metadata.NetworkData{
		Links:    []metadata.Link{},
		Networks: []metadata.Network{},
		Services: []metadata.Service{},
	}

	// Try to extract from configdrive first
	if configDriveData, err := h.extractFromConfigDrive(node); err == nil &&
		configDriveData.NetworkData != nil {
		log.Debug().Str("node_uuid", node.UUID).Msg("Using configdrive network data")
		return configDriveData.NetworkData
	}

	// Fallback to dynamic config from instance info
	log.Debug().Str("node_uuid", node.UUID).Msg("Using dynamic network data")

	// Extract network configuration from instance info
	if instanceInfo, ok := node.InstanceInfo["network_data"]; ok {
		if netData, ok := instanceInfo.(map[string]any); ok {
			// Parse the network data - simplified version
			_ = netData // TODO: Implement proper network data parsing
		}
	}

	// For now, create a basic network configuration as fallback
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

// getUserData extracts user data from the node.
func (h *Handler) getUserData(node *nodes.Node) string {
	// Try to extract from configdrive first
	if configDriveData, err := h.extractFromConfigDrive(node); err == nil &&
		configDriveData.UserData != "" {
		log.Debug().Str("node_uuid", node.UUID).Msg("Using configdrive user data")
		return configDriveData.UserData
	}

	// Fallback to instance info
	log.Debug().Str("node_uuid", node.UUID).Msg("Using dynamic user data")
	if instanceInfo, ok := node.InstanceInfo["user_data"]; ok {
		if userData, ok := instanceInfo.(string); ok {
			return userData
		}
	}

	return ""
}

// getNodeByIP finds a node by its IP address.
func (h *Handler) getNodeByIP(clientIP string) (*nodes.Node, error) {
	// Get the Ironic client
	ironicClient, err := h.Clients.GetIronicClient()
	if err != nil {
		log.Error().
			Err(err).
			Str("client_ip", clientIP).
			Msg("Failed to get ironic client")
		return nil, fmt.Errorf("failed to get ironic client: %w", err)
	}

	// Log the endpoint being used for debugging
	log.Debug().
		Str("client_ip", clientIP).
		Str("ironic_endpoint", ironicClient.Endpoint).
		Msg("Attempting to list nodes from Ironic")

	allPages, err := nodes.List(ironicClient, nodes.ListOpts{}).AllPages()
	if err != nil {
		log.Error().
			Err(err).
			Str("client_ip", clientIP).
			Str("ironic_endpoint", ironicClient.Endpoint).
			Msg("Failed to list nodes from Ironic API")
		return nil, fmt.Errorf("failed to list nodes: %w", err)
	}

	allNodes, err := nodes.ExtractNodes(allPages)
	if err != nil {
		log.Error().
			Err(err).
			Str("client_ip", clientIP).
			Msg("Failed to extract nodes from API response")
		return nil, fmt.Errorf("failed to extract nodes: %w", err)
	}

	log.Debug().
		Str("client_ip", clientIP).
		Int("total_nodes", len(allNodes)).
		Msg("Successfully retrieved nodes from Ironic")

	// Look for node with matching IP
	for _, node := range allNodes {
		getRes := nodes.Get(ironicClient, node.UUID)
		if getRes.Err != nil {
			log.Error().
				Err(getRes.Err).
				Str("client_ip", clientIP).
				Str("node_uuid", node.UUID).
				Str("node_name", node.Name).
				Msg("Failed to get node details from Ironic API")
			continue // Skip this node if we can't get its details
		}
		log.Debug().
			Str("client_ip", clientIP).
			Str("node_uuid", node.UUID).
			Str("node_name", node.Name).
			Msg("Checking node for matching IP")

		if err = getRes.ExtractInto(&node); err != nil {
			log.Error().
				Err(err).
				Str("client_ip", clientIP).
				Str("node_uuid", node.UUID).
				Str("node_name", node.Name).
				Msg("Failed to extract node details")
			continue // Skip this node if we can't extract its details
		}
		// Check if the node has this IP in its port information
		if h.nodeHasIP(&node, clientIP) {
			log.Info().
				Str("client_ip", clientIP).
				Str("node_uuid", node.UUID).
				Str("node_name", node.Name).
				Msg("Found matching node for client IP")
			return &node, nil
		}
	}

	log.Warn().
		Str("client_ip", clientIP).
		Int("nodes_checked", len(allNodes)).
		Msg("No node found matching client IP")
	return nil, fmt.Errorf("no node found for IP %s", clientIP)
}

// nodeHasIP checks if a node has the specified IP address.
func (h *Handler) nodeHasIP(node *nodes.Node, targetIP string) bool {
	log.Debug().
		Str("node_uuid", node.UUID).
		Str("node_name", node.Name).
		Str("target_ip", targetIP).
		Msg("Checking if node has target IP")

	if configDrive, err := h.extractFromConfigDrive(node); err == nil {
		if configDrive.NetworkData != nil {
			// Check if the target IP is in the network data
			for _, net := range configDrive.NetworkData.Networks {
				if net.IPAddress == targetIP {
					log.Debug().
						Str("node_uuid", node.UUID).
						Str("target_ip", targetIP).
						Str("network_id", net.ID).
						Msg("Found target IP in configdrive network data")
					return true
				}
			}
		}
	} else {
		log.Debug().
			Err(err).
			Str("node_uuid", node.UUID).
			Msg("Could not extract configdrive for IP matching")
	}

	// Check instance_info for IP addresses
	if instanceInfo, exists := node.InstanceInfo["fixed_ips"]; exists {
		if fixedIPs, ok := instanceInfo.([]any); ok {
			log.Debug().
				Str("node_uuid", node.UUID).
				Int("fixed_ips_count", len(fixedIPs)).
				Msg("Checking fixed_ips in instance_info")

			for i, ip := range fixedIPs {
				if ipMap, ok := ip.(map[string]any); ok {
					if ipAddr, exists := ipMap["ip_address"]; exists {
						if ipStr, ok := ipAddr.(string); ok && ipStr == targetIP {
							log.Debug().
								Str("node_uuid", node.UUID).
								Str("target_ip", targetIP).
								Int("fixed_ip_index", i).
								Msg("Found target IP in fixed_ips")
							return true
						}
					}
				}
			}
		}
	}

	// Check driver_info for IP information
	if driverInfo, exists := node.DriverInfo["deploy_ramdisk_options"]; exists {
		if options, ok := driverInfo.(map[string]any); ok {
			if ip, exists := options["ipa-api-url"]; exists {
				if ipStr, ok := ip.(string); ok && strings.Contains(ipStr, targetIP) {
					log.Debug().
						Str("node_uuid", node.UUID).
						Str("target_ip", targetIP).
						Str("ipa_api_url", ipStr).
						Msg("Found target IP in IPA API URL")
					return true
				}
			}
		}
	}

	// For testing purposes, if node name contains the IP
	if strings.Contains(node.Name, targetIP) {
		log.Debug().
			Str("node_uuid", node.UUID).
			Str("node_name", node.Name).
			Str("target_ip", targetIP).
			Msg("Found target IP in node name (testing mode)")
		return true
	}

	log.Debug().
		Str("node_uuid", node.UUID).
		Str("target_ip", targetIP).
		Msg("Target IP not found in node")
	return false
}

// Helper functions.
func getNodeHostname(node *nodes.Node) string {
	if node.Name != "" {
		return node.Name
	}
	return node.UUID
}

func getProjectID(node *nodes.Node) string {
	return node.Owner
}

// writeJSONResponse writes a JSON response.
func (h *Handler) writeJSONResponse(w http.ResponseWriter, data any) {
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(data); err != nil {
		log.Error().
			Err(err).
			Interface("data_type", fmt.Sprintf("%T", data)).
			Msg("Failed to encode JSON response")
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
	}
}

// writeTextResponse writes a plain text response.
func (h *Handler) writeTextResponse(w http.ResponseWriter, data string) {
	w.Header().Set("Content-Type", "text/plain")
	if _, err := w.Write([]byte(data)); err != nil {
		log.Error().
			Err(err).
			Int("data_length", len(data)).
			Msg("Failed to write text response")
	}
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
