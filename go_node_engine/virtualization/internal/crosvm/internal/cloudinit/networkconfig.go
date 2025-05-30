package cloudinit

type NetworkConfig struct {
	Version   int                              `json:"version"`
	Ethernets map[string]NetworkConfigEthernet `json:"ethernets,omitempty"`
}

type NetworkConfigEthernet struct {
	Match     *NetworkConfigMatch `json:"match,omitempty"`
	Dhcp4     *bool               `json:"dhcp4,omitempty"`
	Dhcp6     *bool               `json:"dhcp6,omitempty"`
	Addresses []string            `json:"addresses,omitempty"`
	// TODO(axiphi): gateway4 and gateway6 are deprecated, need to be exchanged
	Gateway4    *string                   `json:"gateway4,omitempty"`
	Gateway6    *string                   `json:"gateway6,omitempty"`
	Nameservers *NetworkConfigNameservers `json:"nameservers,omitempty"`
}

type NetworkConfigMatch struct {
	Macaddress *string `json:"macaddress,omitempty"`
}

type NetworkConfigNameservers struct {
	Addresses []string `json:"addresses,omitempty"`
}
