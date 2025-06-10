package metadata

import "time"

// MetaData represents the OpenStack metadata structure.
//
//go:generate go tool github.com/atombender/go-jsonschema -o models/network_data.go -p models https://docs.openstack.org/nova/latest/_downloads/9119ca7ac90aa2990e762c08baea3a36/network_data.json
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
	InstanceType     string            `json:"instance_type"`
}

// Key represents an SSH key.
type Key struct {
	Type string `json:"type"`
	Data string `json:"data"`
	Name string `json:"name"`
}

// NetworkData represents the OpenStack network data structure.
type NetworkData struct {
	Links    []Link    `json:"links"`
	Networks []Network `json:"networks"`
	Services []Service `json:"services,omitempty"`
}

// Link represents a network link.
type Link struct {
	ID                 string   `json:"id"`
	Type               string   `json:"type"`
	EthernetMacAddress string   `json:"ethernet_mac_address,omitempty"`
	MTU                int      `json:"mtu,omitempty"`
	BondMode           string   `json:"bond_mode,omitempty"`
	BondLinks          []string `json:"bond_links,omitempty"`
	BondMIIMon         *uint32  `json:"bond_miimon,omitempty"`
	BondHashPolicy     string   `json:"bond_xmit_hash_policy,omitempty"`
}

// Network represents a network configuration.
type Network struct {
	ID      string  `json:"id,omitempty"`
	Link    string  `json:"link"`
	Type    string  `json:"type"`
	Address string  `json:"ip_address,omitempty"`
	Netmask string  `json:"netmask,omitempty"`
	Gateway string  `json:"gateway,omitempty"`
	Routes  []Route `json:"routes,omitempty"`
}

// Route represents a network route.
type Route struct {
	Network string `json:"network,omitempty"`
	Netmask string `json:"netmask,omitempty"`
	Gateway string `json:"gateway,omitempty"`
	Metric  int    `json:"metric,omitempty"`
}

// Service represents a network service.
type Service struct {
	Type    string `json:"type"`
	Address string `json:"address"`
}

// UserData represents user data (typically cloud-init).
type UserData string

// VendorData represents vendor-specific data.
type VendorData any
