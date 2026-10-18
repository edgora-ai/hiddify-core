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
	opts.LogFile = ""
	opts.ConnectionTestUrls = []string{"http://192.0.2.20/generate_204"}
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
					opts.LogFile = ""
					opts.ConnectionTestUrls = []string{"http://192.0.2.20/generate_204"}
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

func TestBuildConfigAppDirectDNSStrategyCompatibility(t *testing.T) {
	strategies := []C.DomainStrategy{
		C.DomainStrategyAsIS,
		C.DomainStrategyPreferIPv4,
		C.DomainStrategyPreferIPv6,
		C.DomainStrategyIPv4Only,
		C.DomainStrategyIPv6Only,
	}
	for _, outbound := range []Outbound{Outbound_direct, Outbound_direct_with_fragment} {
		for _, fakeDNS := range []bool{false, true} {
			for _, strategy := range strategies {
				t.Run(fmt.Sprintf("%s/fake=%t/strategy=%d", outbound, fakeDNS, strategy), func(t *testing.T) {
					opts := DefaultHiddifyOptions()
					opts.EnableClashApi = false
					opts.LogFile = ""
					opts.EnableFakeDNS = fakeDNS
					opts.DirectDnsAddress = "local"
					opts.DirectDnsDomainStrategy = option.DomainStrategy(strategy)
					opts.RemoteDnsAddress = "udp://1.1.1.1"
					// Avoid DNS probes and remote rule-sets: this is config-only coverage.
					opts.ConnectionTestUrls = []string{"http://192.0.2.20/generate_204"}
					opts.RouteRule = &RouteRuleOptions{RouteRule: &RouteRule{Rules: []*Rule{{
						Enabled:        true,
						Outbound:       outbound,
						DomainSuffixes: []string{"direct.invalid"},
					}}}}
					ctx := libbox.BaseContext(nil)
					options, err := BuildConfig(ctx, opts, &ReadOptions{
						Content: `{"outbounds":[{"type":"socks","tag":"proxy","server":"192.0.2.10","server_port":1080}]}`,
					})
					if err != nil {
						t.Fatal(err)
					}
					foundDirect, foundFake := false, false
					for _, rule := range options.DNS.Rules {
						if domains := rule.DefaultOptions.DomainSuffix; len(domains) == 1 && domains[0] == "direct.invalid" {
							foundDirect = rule.DefaultOptions.Action == C.RuleActionTypeRoute && rule.DefaultOptions.RouteOptions.Server == DNSMultiDirectTag
						}
						if rule.DefaultOptions.RouteOptions.Server == DNSFakeTag && len(rule.DefaultOptions.QueryType) > 0 {
							foundFake = true
						}
					}
					if !foundDirect || foundFake != fakeDNS {
						t.Fatalf("generated DNS rules changed: direct=%t, fake=%t (want %t)", foundDirect, foundFake, fakeDNS)
					}
					encoded, err := options.MarshalJSONContext(ctx)
					if err != nil {
						t.Fatal(err)
					}
					var serialized struct {
						DNS struct {
							Rules any `json:"rules"`
						} `json:"dns"`
					}
					if err := json.Unmarshal(encoded, &serialized); err != nil {
						t.Fatal(err)
					}
					rules, ok := serialized.DNS.Rules.([]any)
					if !ok || len(rules) == 0 {
						t.Fatal("serialized config lost DNS rules")
					}
					assertNoLegacyDNSRuleStrategy(t, rules)
					var roundTrip option.Options
					if err := roundTrip.UnmarshalJSONContext(ctx, encoded); err != nil {
						t.Fatal(err)
					}
					if roundTrip.Route == nil || roundTrip.Route.DefaultDomainResolver == nil {
						t.Fatal("serialized config lost the default domain resolver")
					}
					resolver := roundTrip.Route.DefaultDomainResolver
					if resolver.Server != DNSMultiDirectTag || resolver.Strategy != option.DomainStrategy(strategy) {
						t.Fatalf("serialized bootstrap strategy changed: %+v", resolver)
					}
					if err := libbox.CheckConfigOptions(&roundTrip); err != nil {
						t.Fatalf("generated App-rule DNS config cannot initialize the core: %v", err)
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
