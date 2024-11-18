package addons

import (
	"go_node_engine/addons/flops"
	"go_node_engine/model"
)

type AddonRuntime interface {
	StartUp(config []string)
}

var registeredAddons map[model.AddonType]AddonRuntime

func init() {
	// Register HERE your addons
	registeredAddons[model.FLOPS_LEARNER] = flops.FlopsAddon{}
}

func StartupAddon(addon model.AddonType, config []string) {
	model.GetNodeInfo().AddSupportedAddons(addon)
	if addonruntime, exists := registeredAddons[addon]; exists {
		addonruntime.StartUp(config)
	}
}
