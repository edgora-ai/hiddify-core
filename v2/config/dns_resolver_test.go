package config

import (
	"encoding/json"
	"net/url"
	"strconv"
	"testing"

	C "github.com/sagernet/sing-box/constant"
	"github.com/sagernet/sing-box/option"
)

func TestProxyEncryptedDNSResolver(t *testing.T) {
	originalMain := OutboundMainDetour
	t.Cleanup(func() { OutboundMainDetour = originalMain })

	cases := []struct {
		name          string
		address       string
		detour        string
		mainDetour    string
		proxyLookup   bool
		emptyResolver bool
	}{
		{name: "proxy HTTPS", address: "https://dns.google/dns-query", detour: OutboundSelectTag, proxyLookup: true},
		{name: "proxy HTTPS default path", address: "https://dns.google", detour: OutboundSelectTag, proxyLookup: true},
		{name: "proxy HTTPS custom port and path", address: "https://dns.example.com:8443/custom-query", detour: OutboundSelectTag, proxyLookup: true},
		{name: "proxy TLS", address: "tls://dns.google", detour: OutboundSelectTag, proxyLookup: true},
		{name: "proxy TLS custom port", address: "tls://dns.example.com:8853", detour: OutboundSelectTag, proxyLookup: true},
		{name: "main chain", address: "https://dns.google/dns-query", detour: ChainExtraSecurityTag, mainDetour: ChainExtraSecurityTag, proxyLookup: true},
		{name: "direct HTTPS", address: "https://dns.google/dns-query"},
		{name: "direct TLS", address: "tls://dns.google"},
		{name: "direct outbound", address: "https://dns.google/dns-query", detour: OutboundDirectTag},
		{name: "direct fragment", address: "https://dns.cloudflare.com/dns-query#fragment=300", detour: OutboundDirectFragmentTag},
		{name: "WARP default direct alias", address: "https://dns.google/dns-query", detour: OutboundDirectFragmentTag},
		{name: "main direct alias", address: "https://dns.google/dns-query", detour: OutboundDirectFragmentTag, mainDetour: OutboundDirectFragmentTag},
		{name: "other proxy", address: "https://dns.google/dns-query", detour: "other-proxy"},
		{name: "selector is not main chain", address: "https://dns.google/dns-query", detour: OutboundSelectTag, mainDetour: ChainExtraSecurityTag},
		{name: "main direct outbound", address: "https://dns.google/dns-query", detour: OutboundDirectTag, mainDetour: OutboundDirectTag},
		{name: "proxy IPv4 HTTPS", address: "https://8.8.8.8/dns-query", detour: OutboundSelectTag, emptyResolver: true},
		{name: "proxy IPv6 HTTPS", address: "https://[2001:4860:4860::8888]/dns-query", detour: OutboundSelectTag, emptyResolver: true},
		{name: "proxy IPv4 custom port", address: "https://8.8.8.8:8443/dns-query", detour: OutboundSelectTag},
		{name: "proxy IPv6 custom port", address: "tls://[2001:4860:4860::8888]:8853", detour: OutboundSelectTag},
		{name: "proxy UDP unchanged", address: "udp://dns.google", detour: OutboundSelectTag},
		{name: "proxy TCP unchanged", address: "tcp://dns.google", detour: OutboundSelectTag},
		{name: "proxy QUIC unchanged", address: "quic://dns.google", detour: OutboundSelectTag},
		{name: "proxy HTTP3 unchanged", address: "h3://dns.google/dns-query", detour: OutboundSelectTag},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			OutboundMainDetour = test.mainDetour
			if OutboundMainDetour == "" {
				OutboundMainDetour = OutboundSelectTag
			}
			server, err := getDNSServerOptions(DNSRemoteTag, test.address, DNSDirectTag, test.detour)
			if err != nil {
				t.Fatal(err)
			}
			address, err := url.Parse(test.address)
			if err != nil {
				t.Fatal(err)
			}
			if server.Tag != DNSRemoteTag || server.Type != address.Scheme {
				t.Fatalf("DNS identity changed: tag %q, type %q", server.Tag, server.Type)
			}

			var remote option.RemoteDNSServerOptions
			switch options := server.Options.(type) {
			case *option.RemoteDNSServerOptions:
				remote = *options
			case *option.RemoteTLSDNSServerOptions:
				remote = options.RemoteDNSServerOptions
			case *option.RemoteHTTPSDNSServerOptions:
				remote = options.RemoteDNSServerOptions
				wantPath := address.Path
				if wantPath == "/dns-query" {
					wantPath = ""
				}
				if options.Path != wantPath {
					t.Fatalf("path = %q, want %q", options.Path, wantPath)
				}
				if options.TLS == nil || !options.TLS.Enabled || options.TLS.Insecure {
					t.Fatal("TLS must remain enabled with certificate verification")
				}
			default:
				t.Fatalf("unexpected DNS options %T", server.Options)
			}
			if remote.Server != address.Hostname() || remote.Detour != dialDetour(test.detour) {
				t.Fatalf("DNS destination changed: server %q, detour %q", remote.Server, remote.Detour)
			}
			var wantPort uint16
			if address.Port() != "" {
				port, err := strconv.ParseUint(address.Port(), 10, 16)
				if err != nil {
					t.Fatal(err)
				}
				wantPort = uint16(port)
			}
			if remote.ServerPort != wantPort {
				t.Fatalf("port = %d, want %d", remote.ServerPort, wantPort)
			}
			if (remote.DomainResolver == nil) != test.proxyLookup {
				t.Fatalf("domain resolver = %+v, want proxy lookup %v", remote.DomainResolver, test.proxyLookup)
			}
			if remote.DomainResolver != nil {
				wantResolver := DNSDirectTag
				if test.emptyResolver {
					wantResolver = ""
				}
				if remote.DomainResolver.Server != wantResolver || remote.DomainResolver.Strategy != option.DomainStrategy(C.DomainStrategyPreferIPv4) {
					t.Fatalf("existing resolver changed: %+v, want server %q and prefer_ipv4", remote.DomainResolver, wantResolver)
				}
			}
			encoded, err := json.Marshal(server.Options)
			if err != nil {
				t.Fatal(err)
			}
			var fields map[string]json.RawMessage
			if err := json.Unmarshal(encoded, &fields); err != nil {
				t.Fatal(err)
			}
			if _, present := fields["domain_resolver"]; present == test.proxyLookup {
				t.Fatalf("serialized resolver presence = %v, want %v", present, !test.proxyLookup)
			}
		})
	}
}
