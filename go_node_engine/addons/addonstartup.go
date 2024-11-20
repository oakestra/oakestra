package addons

import (
	"go_node_engine/addons/flops"
	"go_node_engine/model"
)

type AddonRuntime interface {
	Startup(config []string)
}

var registeredAddons map[model.AddonType]AddonRuntime

func StartupAddon(addon model.AddonType, config []string) {
	model.GetNodeInfo().AddSupportedAddons(addon)
	if addonruntime, exists := registeredAddons[addon]; exists {
		addonruntime.Startup(config)
	}
}

func init() {
	registeredAddons := make(map[model.AddonType]AddonRuntime)
	// Register your addons HERE
	registeredAddons[model.FLOPS_LEARNER] = flops.FlopsAddon{}
}
