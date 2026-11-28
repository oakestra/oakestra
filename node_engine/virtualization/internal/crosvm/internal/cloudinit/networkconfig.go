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
	// NOTE: The "link-local" key is not documented in the cloud-init documentation and
	//       only works with the "netplan" cloud-init network backend, because it just passes through this config.
	//       However, the default for this option makes the "*-wait-online.service" systemd service time out,
	//       so we need to set it to fix the boot process.
	//
	//       When you use another cloud-init network config backend (because of another distro or newer cloud-init version),
	//       please check that the "*-wait-online.service" systemd service doesn't time out.
	LinkLocal []string `json:"link-local,omitzero"`
}

type NetworkConfigMatch struct {
	Macaddress *string `json:"macaddress,omitempty"`
}

type NetworkConfigNameservers struct {
	Addresses []string `json:"addresses,omitempty"`
}
