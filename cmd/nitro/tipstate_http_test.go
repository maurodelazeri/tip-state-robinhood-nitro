// Copyright 2026, Offchain Labs, Inc.
// For license information, see https://github.com/OffchainLabs/nitro/blob/master/LICENSE.md

package main

import (
	"reflect"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/rpc"

	"github.com/offchainlabs/nitro/cmd/genericconf"
	"github.com/offchainlabs/nitro/cmd/nitro/config"

	ramtipstate "nitro-tipstate-runtime/tipstate"
)

func TestTipStateHTTPProfileUsesExactNitroConfiguration(t *testing.T) {
	nodeConfig := &config.NodeConfig{
		HTTP: genericconf.HTTPConfig{
			RPCPrefix:  "/custom-rpc",
			CORSDomain: []string{"https://one.example", "https://two.example"},
			VHosts:     []string{"rpc.example", "localhost"},
			ServerTimeouts: genericconf.HTTPServerTimeoutConfig{
				ReadTimeout:       41 * time.Second,
				ReadHeaderTimeout: 42 * time.Second,
				WriteTimeout:      143 * time.Second,
				IdleTimeout:       144 * time.Second,
			},
		},
		Rpc: genericconf.RpcConfig{
			BatchRequestLimit:    50_000,
			MaxBatchResponseSize: 256_000_000,
		},
	}
	want := ramtipstate.HTTPProfileOptions{
		CORSAllowedOrigins:   []string{"https://one.example", "https://two.example"},
		VirtualHosts:         []string{"rpc.example", "localhost"},
		RPCPrefix:            "/custom-rpc",
		BatchRequestLimit:    50_000,
		BatchResponseMaxSize: 256_000_000,
		HTTPBodyLimit:        genericconf.HTTPServerBodyLimitDefault,
		Timeouts: rpc.HTTPTimeouts{
			ReadTimeout:       41 * time.Second,
			ReadHeaderTimeout: 42 * time.Second,
			WriteTimeout:      143 * time.Second,
			IdleTimeout:       144 * time.Second,
		},
	}

	options, err := tipStateHTTPProfileOptions(nodeConfig)
	if err != nil {
		t.Fatalf("derive tip-state HTTP options: %v", err)
	}
	if !reflect.DeepEqual(options, want) {
		t.Fatalf("derived options mismatch\n got: %#v\nwant: %#v", options, want)
	}
	profile, err := tipStateHTTPProfile(nodeConfig)
	if err != nil {
		t.Fatalf("construct immutable HTTP profile: %v", err)
	}
	if got := profile.Options(); !reflect.DeepEqual(got, want) {
		t.Fatalf("immutable profile mismatch\n got: %#v\nwant: %#v", got, want)
	}

	// The product boundary must not retain slices owned by the reloadable node
	// config, even though NewHTTPProfile also clones its construction input.
	nodeConfig.HTTP.CORSDomain[0] = "https://mutated.example"
	nodeConfig.HTTP.VHosts[0] = "mutated.example"
	if !reflect.DeepEqual(options, want) {
		t.Fatalf("derived options changed after source mutation\n got: %#v\nwant: %#v", options, want)
	}
	if got := profile.Options(); !reflect.DeepEqual(got, want) {
		t.Fatalf("immutable profile changed after source mutation\n got: %#v\nwant: %#v", got, want)
	}
}

func TestTipStateHTTPProfileRejectsNilNodeConfig(t *testing.T) {
	if _, err := tipStateHTTPProfile(nil); err == nil {
		t.Fatal("nil Nitro node config was accepted")
	}
}

func TestTipStateHTTPProfileUsesGethTimeoutSanitization(t *testing.T) {
	nodeConfig := &config.NodeConfig{
		HTTP: genericconf.HTTPConfig{
			ServerTimeouts: genericconf.HTTPServerTimeoutConfig{
				ReadTimeout:       999 * time.Millisecond,
				ReadHeaderTimeout: 0,
				WriteTimeout:      -time.Second,
				IdleTimeout:       time.Nanosecond,
			},
		},
	}
	options, err := tipStateHTTPProfileOptions(nodeConfig)
	if err != nil {
		t.Fatalf("derive sanitized HTTP options: %v", err)
	}
	if options.Timeouts != rpc.DefaultHTTPTimeouts {
		t.Fatalf("sanitized timeouts=%+v, want Geth defaults %+v", options.Timeouts, rpc.DefaultHTTPTimeouts)
	}
}
