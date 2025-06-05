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
		log.Error().Err(err).Msg("Failed to get client IP from context")
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	node, err := h.getNodeByIP(clientIP)
	if err != nil {
		log.Error().Err(err).Str("client_ip", clientIP).Msg("Failed to find node for client IP")
		http.Error(w, "Node not found", http.StatusNotFound)
		return
	}

	metaData := h.buildMetaData(node)
	h.writeJSONResponse(w, metaData)
}

// handleNetworkData handles requests to /openstack/latest/network_data.json.
func (h *Handler) handleNetworkData(w http.ResponseWriter, r *http.Request) {
	clientIP, err := getClientIPFromContext(r)
	if err != nil {
		log.Error().Err(err).Msg("Failed to get client IP from context")
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	node, err := h.getNodeByIP(clientIP)
	if err != nil {
		log.Error().Err(err).Str("client_ip", clientIP).Msg("Failed to find node for client IP")
		http.Error(w, "Node not found", http.StatusNotFound)
		return
	}

	networkData := h.buildNetworkData(node)
	h.writeJSONResponse(w, networkData)
}

// handleUserData handles requests to /openstack/latest/user_data.
func (h *Handler) handleUserData(w http.ResponseWriter, r *http.Request) {
	clientIP, err := getClientIPFromContext(r)
	if err != nil {
		log.Error().Err(err).Msg("Failed to get client IP from context")
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

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
	if _, err := w.Write([]byte(userData)); err != nil {
		log.Error().Err(err).Msg("Failed to write user data response")
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
		log.Error().Err(err).Msg("Failed to get client IP from context")
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

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

// extractFromConfigDrive attempts to extract data from a node's configdrive.
func (h *Handler) extractFromConfigDrive(node *nodes.Node) (*configDriveData, error) {
	configDriveInfo, exists := node.InstanceInfo["configdrive"]
	if !exists {
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
			var configMap map[string]any
			if err := json.Unmarshal([]byte(configDriveStr), &configMap); err == nil {
				return h.parseConfigDriveMap(configMap)
			}
		}

		// For ISO files, we would use utils.ConfigDrive to parse
		// This is a placeholder for ISO parsing functionality
		return nil, fmt.Errorf("ISO configdrive parsing not yet implemented")
	}

	// Try to parse as direct JSON data
	if configMap, ok := configDriveInfo.(map[string]any); ok {
		return h.parseConfigDriveMap(configMap)
	}

	return nil, fmt.Errorf("unsupported configdrive format")
}

// configDriveData holds extracted configdrive information.
type configDriveData struct {
	MetaData    map[string]any    `json:"meta_data,omitempty"`
	UserData    string            `json:"user_data,omitempty"`
	NetworkData map[string]any    `json:"network_data,omitempty"`
	VendorData  map[string]any    `json:"vendor_data,omitempty"`
	PublicKeys  map[string]string `json:"public_keys,omitempty"`
}

// parseConfigDriveMap parses configdrive data from a map.
func (h *Handler) parseConfigDriveMap(configMap map[string]any) (*configDriveData, error) {
	data := &configDriveData{
		PublicKeys: make(map[string]string),
	}

	// Extract meta_data
	if metaData, exists := configMap["meta_data"]; exists {
		if metaMap, ok := metaData.(map[string]any); ok {
			data.MetaData = metaMap
		}
	}

	// Extract user_data
	if userData, exists := configMap["user_data"]; exists {
		if userDataStr, ok := userData.(string); ok {
			data.UserData = userDataStr
		}
	}

	// Extract network_data
	if networkData, exists := configMap["network_data"]; exists {
		if networkMap, ok := networkData.(map[string]any); ok {
			data.NetworkData = networkMap
		}
	}

	// Extract vendor_data
	if vendorData, exists := configMap["vendor_data"]; exists {
		if vendorMap, ok := vendorData.(map[string]any); ok {
			data.VendorData = vendorMap
		}
	}

	// Extract public keys
	if publicKeys, exists := configMap["public_keys"]; exists {
		if keysMap, ok := publicKeys.(map[string]any); ok {
			for name, key := range keysMap {
				if keyStr, ok := key.(string); ok {
					data.PublicKeys[name] = keyStr
				}
			}
		}
	}

	return data, nil
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
			for key, value := range configDriveData.MetaData {
				if strValue, ok := value.(string); ok {
					metaData.Meta[key] = strValue
				}
			}
		}

		// Use configdrive public keys if available
		if len(configDriveData.PublicKeys) > 0 {
			metaData.PublicKeys = configDriveData.PublicKeys
		}

		// Override hostname if available in configdrive
		if hostname, exists := configDriveData.MetaData["hostname"]; exists {
			if hostnameStr, ok := hostname.(string); ok {
				metaData.Hostname = hostnameStr
			}
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

		// Parse configdrive network data
		if links, exists := configDriveData.NetworkData["links"]; exists {
			if linksArray, ok := links.([]any); ok {
				for _, linkData := range linksArray {
					if linkMap, ok := linkData.(map[string]any); ok {
						link := metadata.Link{}
						if id, exists := linkMap["id"]; exists {
							if idStr, ok := id.(string); ok {
								link.ID = idStr
							}
						}
						if linkType, exists := linkMap["type"]; exists {
							if typeStr, ok := linkType.(string); ok {
								link.Type = typeStr
							}
						}
						if mtu, exists := linkMap["mtu"]; exists {
							if mtuFloat, ok := mtu.(float64); ok {
								link.MTU = int(mtuFloat)
							}
						}
						networkData.Links = append(networkData.Links, link)
					}
				}
			}
		}

		if networks, exists := configDriveData.NetworkData["networks"]; exists {
			if networksArray, ok := networks.([]any); ok {
				for _, netData := range networksArray {
					if netMap, ok := netData.(map[string]any); ok {
						network := metadata.Network{}
						if id, exists := netMap["id"]; exists {
							if idStr, ok := id.(string); ok {
								network.ID = idStr
							}
						}
						if netType, exists := netMap["type"]; exists {
							if typeStr, ok := netType.(string); ok {
								network.Type = typeStr
							}
						}
						if link, exists := netMap["link"]; exists {
							if linkStr, ok := link.(string); ok {
								network.Link = linkStr
							}
						}
						networkData.Networks = append(networkData.Networks, network)
					}
				}
			}
		}

		return networkData
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
		return nil, fmt.Errorf("failed to get ironic client: %w", err)
	}

	// Create pagination options
	listOpts := nodes.ListOpts{}

	allPages, err := nodes.List(ironicClient, listOpts).AllPages()
	if err != nil {
		return nil, fmt.Errorf("failed to list nodes: %w", err)
	}

	allNodes, err := nodes.ExtractNodes(allPages)
	if err != nil {
		return nil, fmt.Errorf("failed to extract nodes: %w", err)
	}

	// Look for node with matching IP
	for _, node := range allNodes {
		// Check if the node has this IP in its port information
		if h.nodeHasIP(&node, clientIP) {
			return &node, nil
		}
	}

	return nil, fmt.Errorf("no node found for IP %s", clientIP)
}

// nodeHasIP checks if a node has the specified IP address.
func (h *Handler) nodeHasIP(node *nodes.Node, targetIP string) bool {
	// Check instance_info for IP addresses
	if instanceInfo, exists := node.InstanceInfo["fixed_ips"]; exists {
		if fixedIPs, ok := instanceInfo.([]any); ok {
			for _, ip := range fixedIPs {
				if ipMap, ok := ip.(map[string]any); ok {
					if ipAddr, exists := ipMap["ip_address"]; exists {
						if ipStr, ok := ipAddr.(string); ok && ipStr == targetIP {
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
					return true
				}
			}
		}
	}

	// For testing purposes, if node name contains the IP
	if strings.Contains(node.Name, targetIP) {
		return true
	}

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
		log.Error().Err(err).Msg("Failed to encode JSON response")
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
	}
}

// writeTextResponse writes a plain text response.
func (h *Handler) writeTextResponse(w http.ResponseWriter, data string) {
	w.Header().Set("Content-Type", "text/plain")
	if _, err := w.Write([]byte(data)); err != nil {
		log.Error().Err(err).Msg("Failed to write text response")
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
