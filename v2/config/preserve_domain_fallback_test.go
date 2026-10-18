package config

import (
	"bytes"
	"encoding/json"
	"os"
	"testing"

	C "github.com/sagernet/sing-box/constant"
	"github.com/sagernet/sing-box/experimental/libbox"
	"github.com/sagernet/sing-box/option"
)

const fallbackProxyOptionKey = "preserve-domain-for-fallback-proxy"
const fallbackProxySniffKey = "preserve_domain_for_fallback_proxy"

// Reserved documentation addresses keep these config-only tests independent of
// DNS, a proxy server, and the host's network configuration.
const fallbackProxyTestProfile = `{"outbounds":[{"type":"socks","tag":"test-proxy","server":"192.0.2.10","server_port":1080}]}`

func fallbackProxyTestOptions(t *testing.T, content []byte) *HiddifyOptions {
	t.Helper()
	opts := DefaultHiddifyOptions()
	if err := json.Unmarshal(content, opts); err != nil {
		t.Fatalf("decode app options: %v", err)
	}
	opts.ConnectionTestUrls = []string{"http://192.0.2.20/generate_204"}
	return opts
}

func buildFallbackProxyTestConfig(t *testing.T, opts *HiddifyOptions, want bool) []byte {
	t.Helper()
	ctx := libbox.BaseContext(nil)
	built, err := BuildConfig(ctx, opts, &ReadOptions{Content: fallbackProxyTestProfile})
	if err != nil {
		t.Fatalf("BuildConfig: %v", err)
	}
	content, err := built.MarshalJSONContext(ctx)
	if err != nil {
		t.Fatalf("marshal built config: %v", err)
	}
	var reparsed option.Options
	if err := reparsed.UnmarshalJSONContext(ctx, content); err != nil {
		t.Fatalf("sing-box config JSON round-trip: %v", err)
	}
	if reparsed.Route == nil || len(reparsed.Route.Rules) == 0 || reparsed.Route.Rules[0].DefaultOptions.SniffOptions.PreserveDomainForFallbackProxy != want {
		t.Errorf("typed sing-box sniff flag did not survive JSON round-trip (want %v)", want)
	}
	var decoded struct {
		Route struct {
			Rules []map[string]json.RawMessage `json:"rules"`
		} `json:"route"`
	}
	if err := json.Unmarshal(content, &decoded); err != nil {
		t.Fatalf("decode built config: %v", err)
	}
	if len(decoded.Route.Rules) == 0 {
		t.Fatal("builder produced no route rules")
	}
	first := decoded.Route.Rules[0]
	if string(first["action"]) != `"`+C.RuleActionTypeSniff+`"` {
		t.Fatalf("first action = %s, want sniff", first["action"])
	}
	flag, present := first[fallbackProxySniffKey]
	if want {
		if !present || string(flag) != "true" {
			t.Errorf("first sniff %s = %s (present=%v), want true", fallbackProxySniffKey, flag, present)
		}
	} else if present {
		t.Errorf("disabled sniff option must be omitted, got %s", flag)
	}
	for i, rule := range decoded.Route.Rules[1:] {
		if value, ok := rule[fallbackProxySniffKey]; ok {
			t.Errorf("route rule %d unexpectedly has compatibility flag %s", i+1, value)
		}
	}
	t.Logf("first sniff action: %s", mustMarshalFallbackProxyJSON(t, first))
	return content
}

func mustMarshalFallbackProxyJSON(t *testing.T, value any) []byte {
	t.Helper()
	content, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return content
}

func TestPreserveDomainForFallbackProxyOptionsJSON(t *testing.T) {
	for _, flag := range []bool{false, true} {
		t.Run(map[bool]string{false: "false", true: "true"}[flag], func(t *testing.T) {
			opts := fallbackProxyTestOptions(t, mustMarshalFallbackProxyJSON(t, map[string]bool{fallbackProxyOptionKey: flag}))
			var serialized map[string]json.RawMessage
			if err := json.Unmarshal(mustMarshalFallbackProxyJSON(t, opts), &serialized); err != nil {
				t.Fatal(err)
			}
			value, present := serialized[fallbackProxyOptionKey]
			if flag && (!present || string(value) != "true") {
				t.Fatalf("true flag lost in HiddifyOptions JSON round-trip: %s", value)
			}
			if !flag && present {
				t.Fatalf("false flag must be omitted: %s", value)
			}
		})
	}
	defaults := mustMarshalFallbackProxyJSON(t, DefaultHiddifyOptions())
	var serialized map[string]json.RawMessage
	if err := json.Unmarshal(defaults, &serialized); err != nil {
		t.Fatal(err)
	}
	if _, present := serialized[fallbackProxyOptionKey]; present {
		t.Fatal("default options must not enable domain preservation")
	}
}

func TestPreserveDomainForFallbackProxyBuildMatrix(t *testing.T) {
	for _, fakeDNS := range []bool{false, true} {
		for _, flag := range []string{"missing", "false", "true"} {
			name := "fake-off/" + flag
			if fakeDNS {
				name = "fake-on/" + flag
			}
			t.Run(name, func(t *testing.T) {
				input := map[string]any{"enable-fake-dns": fakeDNS}
				if flag != "missing" {
					input[fallbackProxyOptionKey] = flag == "true"
				}
				opts := fallbackProxyTestOptions(t, mustMarshalFallbackProxyJSON(t, input))
				buildFallbackProxyTestConfig(t, opts, flag == "true" && !fakeDNS)
			})
		}
	}
}

func TestPreserveDomainForFallbackProxyFakeDNSOnUnchanged(t *testing.T) {
	disabled := buildFallbackProxyTestConfig(t, fallbackProxyTestOptions(t, []byte(`{"enable-fake-dns":true}`)), false)
	enabled := buildFallbackProxyTestConfig(t, fallbackProxyTestOptions(t, []byte(`{"enable-fake-dns":true,"preserve-domain-for-fallback-proxy":true}`)), false)
	if !bytes.Equal(disabled, enabled) {
		t.Fatal("opting in must not change the generated Fake DNS-on config")
	}
}

func TestPreserveDomainForFallbackProxySubscriptionHeadersCannotOptIn(t *testing.T) {
	opts := GetOverridableHiddifyOptions(map[string][]string{
		fallbackProxyOptionKey: {"true"},
	})
	var serialized map[string]json.RawMessage
	if err := json.Unmarshal(mustMarshalFallbackProxyJSON(t, opts), &serialized); err != nil {
		t.Fatal(err)
	}
	if value, present := serialized[fallbackProxyOptionKey]; present {
		t.Fatalf("subscription header must not opt in: %s", value)
	}
}

// The opt-in Flutter test writes these options after a real in-memory profile
// save/read, subscription parse, and typed Dart JSON round-trip, then runs this
// test with the file path. No user profile or real configuration is read.
func TestPreserveDomainForFallbackProxyAppRoundTrip(t *testing.T) {
	path := os.Getenv("HIDDIFY_TEST_FALLBACK_PROXY_OPTIONS")
	if path == "" {
		t.Skip("run the Flutter fallback proxy test with --dart-define=HIDDIFY_TEST_GO_BUILDER=true")
	}
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var cases []struct {
		Name    string          `json:"name"`
		Options json.RawMessage `json:"options"`
		Want    bool            `json:"want"`
	}
	if err := json.Unmarshal(content, &cases); err != nil {
		t.Fatal(err)
	}
	if len(cases) == 0 {
		t.Fatal("Flutter supplied no profile round-trip cases")
	}
	for _, test := range cases {
		t.Run(test.Name, func(t *testing.T) {
			opts := fallbackProxyTestOptions(t, test.Options)
			buildFallbackProxyTestConfig(t, opts, test.Want)
		})
	}
}
