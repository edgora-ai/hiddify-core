package hcore

import (
	"context"
	"math"
	"runtime/debug"
	"testing"

	"github.com/sagernet/sing-box/experimental/libbox"
	"github.com/sagernet/sing-box/service/oomkiller"
)

func preserveRuntimeMemoryPolicy(t *testing.T) {
	t.Helper()
	previousLimit := debug.SetMemoryLimit(-1)
	previousGC := debug.SetGCPercent(-1)
	t.Cleanup(func() {
		debug.SetMemoryLimit(previousLimit)
		debug.SetGCPercent(previousGC)
	})
}

func TestIOSRuntimeMemoryLimit(t *testing.T) {
	preserveRuntimeMemoryPolicy(t)
	for _, disableMemoryLimit := range []bool{false, true} {
		debug.SetMemoryLimit(40 * 1024 * 1024)
		debug.SetGCPercent(100)
		configureRuntimeMemoryLimit(true, disableMemoryLimit)
		if actual := debug.SetMemoryLimit(-1); actual != iosGoMemoryLimit {
			t.Fatalf("iOS Go memory budget = %d, want %d (disable=%t)", actual, iosGoMemoryLimit, disableMemoryLimit)
		}
		if actual := debug.SetGCPercent(-1); actual != oomkiller.DefaultAppleNetworkExtensionGCPercent {
			t.Fatalf("iOS GOGC = %d, want %d", actual, oomkiller.DefaultAppleNetworkExtensionGCPercent)
		}
	}
}

func TestIOSRuntimeMemoryLimitReappliedAfterSetup(t *testing.T) {
	preserveRuntimeMemoryPolicy(t)
	for range 3 {
		debug.SetMemoryLimit(40 * 1024 * 1024)
		debug.SetGCPercent(100)
		libbox.SetMemoryLimit(true)
		configureRuntimeMemoryLimit(true, false)
		if actual := debug.SetMemoryLimit(-1); actual != iosGoMemoryLimit {
			t.Fatalf("service startup raised the iOS Go memory budget to %d", actual)
		}
		if actual := debug.SetGCPercent(-1); actual != 50 {
			t.Fatalf("service startup reset iOS GOGC to %d", actual)
		}
	}
}

func TestNonIOSRuntimeMemoryPolicyUnchanged(t *testing.T) {
	preserveRuntimeMemoryPolicy(t)
	for _, disableMemoryLimit := range []bool{false, true} {
		libbox.SetMemoryLimit(!disableMemoryLimit)
		expectedLimit := debug.SetMemoryLimit(-1)
		debug.SetMemoryLimit(1024 * 1024)
		debug.SetGCPercent(75)
		configureRuntimeMemoryLimit(false, disableMemoryLimit)
		if actual := debug.SetMemoryLimit(-1); actual != expectedLimit {
			t.Fatalf("non-iOS memory budget changed: got %d, want %d", actual, expectedLimit)
		}
		if actual := debug.SetGCPercent(-1); actual != 75 {
			t.Fatalf("non-iOS GOGC changed to %d", actual)
		}
		if disableMemoryLimit && debug.SetMemoryLimit(-1) != math.MaxInt64 {
			t.Fatal("non-iOS disable-memory-limit setting was not retained")
		}
	}
}

func TestServiceOptionsEnableIOSMemoryProtection(t *testing.T) {
	ctx := context.Background()
	for _, isIOS := range []bool{false, true} {
		options := newServiceOptions(ctx, isIOS)
		if options.OOMKillerEnabled != isIOS {
			t.Fatalf("OOM protection enabled=%t for iOS=%t", options.OOMKillerEnabled, isIOS)
		}
		if options.Context != ctx || options.Handler == nil || len(options.ExtraServices) != 1 {
			t.Fatal("existing service context, handler or lifecycle manager changed")
		}
	}
}
