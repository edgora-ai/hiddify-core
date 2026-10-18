package hcore

import (
	"runtime/debug"

	"github.com/sagernet/sing-box/experimental/libbox"
	"github.com/sagernet/sing-box/service/oomkiller"
)

const iosGoMemoryLimit = 24 * 1024 * 1024

func configureRuntimeMemoryLimit(isIOS bool, disableMemoryLimit bool) {
	libbox.SetMemoryLimit(isIOS || !disableMemoryLimit)
	if isIOS {
		// The 50 MiB extension limit includes native allocations, not just Go memory.
		debug.SetMemoryLimit(iosGoMemoryLimit)
		debug.SetGCPercent(oomkiller.DefaultAppleNetworkExtensionGCPercent)
	}
}
