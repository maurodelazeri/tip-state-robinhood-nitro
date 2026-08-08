// Copyright 2026, Offchain Labs, Inc.
// For license information, see https://github.com/OffchainLabs/nitro/blob/master/LICENSE.md

package main

import (
	"errors"
	"slices"

	"github.com/ethereum/go-ethereum/node"
	"github.com/ethereum/go-ethereum/rpc"

	"github.com/offchainlabs/nitro/cmd/genericconf"
	"github.com/offchainlabs/nitro/cmd/nitro/config"

	ramtipstate "nitro-tipstate-runtime/tipstate"
)

// tipStateHTTPProfileOptions derives the RAM endpoint's complete transport
// behavior from the same parsed configuration already applied to Nitro's
// ordinary HTTP stack. There are deliberately no independent tip-state batch,
// CORS, vhost, prefix, body, or server-timeout defaults here.
func tipStateHTTPProfileOptions(nodeConfig *config.NodeConfig) (ramtipstate.HTTPProfileOptions, error) {
	if nodeConfig == nil {
		return ramtipstate.HTTPProfileOptions{}, errors.New("nil Nitro node config")
	}
	timeouts := nodeConfig.HTTP.ServerTimeouts
	httpTimeouts := rpc.HTTPTimeouts{
		ReadTimeout:       timeouts.ReadTimeout,
		ReadHeaderTimeout: timeouts.ReadHeaderTimeout,
		WriteTimeout:      timeouts.WriteTimeout,
		IdleTimeout:       timeouts.IdleTimeout,
	}
	// Geth sanitizes the ordinary HTTP server's configured timeouts immediately
	// before listen. Apply the same exported normalization so this profile holds
	// the effective values, including when an operator supplied sub-second ones.
	node.CheckTimeouts(&httpTimeouts)
	return ramtipstate.HTTPProfileOptions{
		CORSAllowedOrigins:   slices.Clone(nodeConfig.HTTP.CORSDomain),
		VirtualHosts:         slices.Clone(nodeConfig.HTTP.VHosts),
		RPCPrefix:            nodeConfig.HTTP.RPCPrefix,
		BatchRequestLimit:    nodeConfig.Rpc.BatchRequestLimit,
		BatchResponseMaxSize: nodeConfig.Rpc.MaxBatchResponseSize,
		HTTPBodyLimit:        genericconf.HTTPServerBodyLimitDefault,
		Timeouts:             httpTimeouts,
	}, nil
}

func tipStateHTTPProfile(nodeConfig *config.NodeConfig) (ramtipstate.HTTPProfile, error) {
	options, err := tipStateHTTPProfileOptions(nodeConfig)
	if err != nil {
		return ramtipstate.HTTPProfile{}, err
	}
	return ramtipstate.NewHTTPProfile(options)
}
