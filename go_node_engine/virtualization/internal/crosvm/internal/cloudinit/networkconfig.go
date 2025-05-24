package cloudinit

type NetworkConfig struct {
	Network NetworkConfigNetwork `json:"network"`
}

type NetworkConfigNetwork struct {
	Version   string                           `json:"version"`
	Ethernets map[string]NetworkConfigEthernet `json:"ethernets,omitempty"`
}

type NetworkConfigEthernet struct {
	Match       *NetworkConfigMatch       `json:"match,omitempty"`
	Dhcp4       *bool                     `json:"dhcp4,omitempty"`
	Dhcp6       *bool                     `json:"dhcp6,omitempty"`
	Addresses   []string                  `json:"addresses,omitempty"`
	Gateway4    *string                   `json:"gateway4,omitempty"`
	Gateway6    *string                   `json:"gateway6,omitempty"`
	Nameservers *NetworkConfigNameservers `json:"nameservers,omitempty"`
}

type NetworkConfigMatch struct {
	Name *string `json:"name,omitempty"`
}

type NetworkConfigNameservers struct {
	Addresses []string `json:"addresses,omitempty"`
}
