package config

import (
	"encoding/json"
	"fmt"
	"testing"

	C "github.com/sagernet/sing-box/constant"
	"github.com/sagernet/sing-box/experimental/libbox"
	"github.com/sagernet/sing-box/option"
)

func TestBuildConfigFakeDNSWithIPv4OnlyBootstrapStarts(t *testing.T) {
	opts := DefaultHiddifyOptions()
	opts.EnableClashApi = false
	opts.EnableFakeDNS = true
	opts.DirectDnsAddress = "local"
	opts.DirectDnsDomainStrategy = option.DomainStrategy(C.DomainStrategyIPv4Only)
	opts.RemoteDnsAddress = "udp://1.1.1.1"
	options, err := BuildConfig(libbox.BaseContext(nil), opts, &ReadOptions{Content: chainTestProfile})
	if err != nil {
		t.Fatal(err)
	}
	if err := libbox.CheckConfigOptions(options); err != nil {
		t.Fatalf("generated Fake DNS config cannot initialize the core: %v", err)
	}
}

func TestBuildConfigDNSStrategyCompatibility(t *testing.T) {
	strategies := []C.DomainStrategy{
		C.DomainStrategyAsIS,
		C.DomainStrategyPreferIPv4,
		C.DomainStrategyPreferIPv6,
		C.DomainStrategyIPv4Only,
		C.DomainStrategyIPv6Only,
	}
	for _, fakeDNS := range []bool{false, true} {
		for _, region := range []string{"other", "cn"} {
			for _, strategy := range strategies {
				t.Run(fmt.Sprintf("fake=%t/region=%s/strategy=%d", fakeDNS, region, strategy), func(t *testing.T) {
					opts := DefaultHiddifyOptions()
					opts.EnableClashApi = false
					opts.EnableFakeDNS = fakeDNS
					opts.Region = region
					opts.DirectDnsAddress = "local"
					opts.DirectDnsDomainStrategy = option.DomainStrategy(strategy)
					opts.RemoteDnsAddress = "udp://1.1.1.1"
					options, err := BuildConfig(libbox.BaseContext(nil), opts, &ReadOptions{Content: chainTestProfile})
					if err != nil {
						t.Fatal(err)
					}
					resolver := options.Route.DefaultDomainResolver
					if resolver == nil || resolver.Server != DNSMultiDirectTag || resolver.Strategy != option.DomainStrategy(strategy) {
						t.Fatalf("bootstrap domain resolution strategy changed: %+v", resolver)
					}
					encoded, err := json.Marshal(options.DNS.Rules)
					if err != nil {
						t.Fatal(err)
					}
					var rules any
					if err := json.Unmarshal(encoded, &rules); err != nil {
						t.Fatal(err)
					}
					assertNoLegacyDNSRuleStrategy(t, rules)
					if err := libbox.CheckConfigOptions(options); err != nil {
						t.Fatalf("generated DNS config cannot initialize the core: %v", err)
					}
				})
			}
		}
	}
}

func assertNoLegacyDNSRuleStrategy(t *testing.T, value any) {
	t.Helper()
	switch value := value.(type) {
	case map[string]any:
		if _, exists := value["strategy"]; exists {
			t.Fatal("generated DNS rule still contains the legacy strategy action option")
		}
		for _, child := range value {
			assertNoLegacyDNSRuleStrategy(t, child)
		}
	case []any:
		for _, child := range value {
			assertNoLegacyDNSRuleStrategy(t, child)
		}
	}
}
