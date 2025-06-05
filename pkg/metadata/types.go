package metadata

import "time"

// MetaData represents the OpenStack metadata structure
type MetaData struct {
	UUID             string            `json:"uuid"`
	Name             string            `json:"name,omitempty"`
	AvailabilityZone string            `json:"availability_zone,omitempty"`
	Hostname         string            `json:"hostname,omitempty"`
	LaunchIndex      int               `json:"launch_index,omitempty"`
	PublicKeys       map[string]string `json:"public_keys,omitempty"`
	Meta             map[string]string `json:"meta,omitempty"`
	Keys             []Key             `json:"keys,omitempty"`
	AdminPass        string            `json:"admin_pass,omitempty"`
	ProjectID        string            `json:"project_id,omitempty"`
	CreationTime     *time.Time        `json:"creation_time,omitempty"`
}

// Key represents an SSH key
type Key struct {
	Type string `json:"type"`
	Data string `json:"data"`
	Name string `json:"name"`
}

// NetworkData represents the OpenStack network data structure
type NetworkData struct {
	Links    []Link    `json:"links"`
	Networks []Network `json:"networks"`
	Services []Service `json:"services"`
}

// Link represents a network link
type Link struct {
	ID                 string `json:"id"`
	Type               string `json:"type"`
	EthernetMacAddress string `json:"ethernet_mac_address,omitempty"`
	MTU                int    `json:"mtu,omitempty"`
}

// Network represents a network configuration
type Network struct {
	ID        string   `json:"id"`
	Type      string   `json:"type"`
	Link      string   `json:"link"`
	IPAddress string   `json:"ip_address,omitempty"`
	Netmask   string   `json:"netmask,omitempty"`
	Gateway   string   `json:"gateway,omitempty"`
	Routes    []Route  `json:"routes,omitempty"`
	DNS       []string `json:"dns,omitempty"`
}

// Route represents a network route
type Route struct {
	Network string `json:"network"`
	Gateway string `json:"gateway"`
	Netmask string `json:"netmask"`
	Metric  int    `json:"metric,omitempty"`
}

// Service represents a network service
type Service struct {
	Type    string `json:"type"`
	Address string `json:"address"`
}

// UserData represents user data (typically cloud-init)
type UserData string

// VendorData represents vendor-specific data
type VendorData any
