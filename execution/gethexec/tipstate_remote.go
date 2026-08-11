// Copyright 2026, Offchain Labs, Inc.
// For license information, see https://github.com/OffchainLabs/nitro/blob/master/LICENSE.md

//go:build !wasm

package gethexec

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"math"
	"path/filepath"
	"sort"
	"time"

	"github.com/ethereum/go-ethereum/core/rawdb"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/log"
	"github.com/offchainlabs/nitro/arbos/programs"

	"nitro-tipstate-runtime/fanout"
	"nitro-tipstate-runtime/nitroseed"
	"nitro-tipstate-runtime/tipproxy"
	"nitro-tipstate-runtime/tipwire"
)

const (
	mandatoryTipStateReplicaCount = 3
	// The local fanout proxy owns and chowns its 0600 socket as root. Nitro
	// admits only that fixed kernel peer identity; this is not configurable.
	mandatoryTipStateProxyUID = uint32(0)
)

type tipStateRemoteSettings struct {
	proxySocket     string
	proxyTimeout    time.Duration
	seedBatchBytes  uint64
	membershipEpoch uint64
	activeSetID     tipwire.Hash32
	members         []tipwire.Hash32
	servingPolicy   tipwire.ServingPolicy
}

// tipStateRemoteRuntime retains only the live RAM fanout hook and immutable
// evidence needed for diagnostics. It deliberately does not retain the Geth
// database, blockchain, startup scope, RPC client, or proxy connection as a
// separate fallback-capable object graph; the hook owns the one fixed session.
type tipStateRemoteRuntime struct {
	hook    *fanout.CanonicalHook
	binding tipwire.Binding
	cursor  tipwire.Cursor
}

func parseTipStateHash32(name, value string) (tipwire.Hash32, error) {
	var result tipwire.Hash32
	if value == "" {
		return result, fmt.Errorf("%s is required", name)
	}
	if len(value) != hex.EncodedLen(len(result)) {
		return result, fmt.Errorf("%s must contain exactly 64 hexadecimal digits", name)
	}
	if _, err := hex.Decode(result[:], []byte(value)); err != nil {
		return tipwire.Hash32{}, fmt.Errorf("%s is not exact hexadecimal: %w", name, err)
	}
	if result == (tipwire.Hash32{}) {
		return tipwire.Hash32{}, fmt.Errorf("%s must be nonzero", name)
	}
	return result, nil
}

func (c *TipStateConfig) remoteSettings() (tipStateRemoteSettings, error) {
	var settings tipStateRemoteSettings
	remote := c.Remote
	if remote.ProxySocket == "" || !filepath.IsAbs(remote.ProxySocket) || filepath.Clean(remote.ProxySocket) != remote.ProxySocket {
		return settings, errors.New("remote proxy-socket must be a clean absolute Unix socket path")
	}
	if remote.ProxyTimeout <= 0 {
		return settings, errors.New("remote proxy-timeout must be positive")
	}
	proxyLimits := tipproxy.DefaultLimits()
	if proxyLimits.MaxMembers != mandatoryTipStateReplicaCount {
		return settings, fmt.Errorf("proxy member limit %d differs from mandatory cohort size %d", proxyLimits.MaxMembers, mandatoryTipStateReplicaCount)
	}
	if remote.SeedBatchBytes == 0 || remote.SeedBatchBytes > proxyLimits.Wire.MaxFrameBytes {
		return settings, fmt.Errorf("remote seed-batch-bytes must be in [1,%d]", proxyLimits.Wire.MaxFrameBytes)
	}
	if remote.MembershipEpoch == 0 {
		return settings, errors.New("remote membership-epoch must be nonzero")
	}
	if c.JournalLimit <= 0 || uint64(c.JournalLimit) > math.MaxUint32 {
		return settings, errors.New("remote journal-limit must fit a positive uint32")
	}
	journalLimit := uint32(c.JournalLimit)
	if journalLimit > proxyLimits.Wire.MaxRemovedBlocks || journalLimit > proxyLimits.Wire.MaxAddedBlocks ||
		journalLimit > proxyLimits.Wire.MaxRecentBlockHashes {
		return settings, fmt.Errorf(
			"remote journal-limit %d exceeds live bounds removed=%d added=%d recent=%d",
			journalLimit, proxyLimits.Wire.MaxRemovedBlocks, proxyLimits.Wire.MaxAddedBlocks,
			proxyLimits.Wire.MaxRecentBlockHashes,
		)
	}

	parse := func(name, value string) (tipwire.Hash32, error) {
		return parseTipStateHash32("remote "+name, value)
	}
	var err error
	if len(remote.MemberIDs) != mandatoryTipStateReplicaCount {
		return settings, fmt.Errorf("remote member-ids contains %d identities, want exactly %d", len(remote.MemberIDs), mandatoryTipStateReplicaCount)
	}
	settings.members = make([]tipwire.Hash32, len(remote.MemberIDs))
	for index, member := range remote.MemberIDs {
		settings.members[index], err = parse(fmt.Sprintf("member-ids[%d]", index), member)
		if err != nil {
			return tipStateRemoteSettings{}, err
		}
		if index > 0 && bytes.Compare(settings.members[index-1][:], settings.members[index][:]) >= 0 {
			return tipStateRemoteSettings{}, errors.New("remote member-ids must be unique and in strictly increasing byte order")
		}
	}
	derivedActiveSet, err := tipwire.DeriveActiveSetID(settings.members)
	if err != nil {
		return tipStateRemoteSettings{}, fmt.Errorf("derive exact remote active set: %w", err)
	}
	settings.activeSetID = derivedActiveSet
	settings.servingPolicy = tipwire.ServingPolicy{
		RPCGasCap:              c.GasCap,
		RPCCallTimeoutNanos:    uint64(c.CallTimeout),
		LeaseDurationNanos:     uint64(remote.LeaseDuration),
		HeartbeatIntervalNanos: uint64(remote.HeartbeatInterval),
		OperationTimeoutNanos:  uint64(remote.OperationTimeout),
		JournalLimit:           uint32(c.JournalLimit),
	}
	if err := settings.servingPolicy.Validate(); err != nil {
		return tipStateRemoteSettings{}, fmt.Errorf("invalid remote serving policy: %w", err)
	}
	if remote.ProxyTimeout <= remote.OperationTimeout {
		return tipStateRemoteSettings{}, errors.New("remote proxy-timeout must be strictly greater than remote operation-timeout")
	}
	settings.proxySocket = remote.ProxySocket
	settings.proxyTimeout = remote.ProxyTimeout
	settings.seedBatchBytes = remote.SeedBatchBytes
	settings.membershipEpoch = remote.MembershipEpoch
	return settings, nil
}

func newTipStateStartupIdentity(name string) (tipwire.Hash32, error) {
	var identity tipwire.Hash32
	for identity == (tipwire.Hash32{}) {
		if _, err := rand.Read(identity[:]); err != nil {
			return tipwire.Hash32{}, fmt.Errorf("generate fresh remote %s: %w", name, err)
		}
	}
	return identity, nil
}

func tipStateBlockRef(header *types.Header) (tipwire.BlockRef, error) {
	if header == nil || header.Number == nil || !header.Number.IsUint64() {
		return tipwire.BlockRef{}, errors.New("tip-state block has no uint64 header number")
	}
	ref := tipwire.BlockRef{
		Number:     header.Number.Uint64(),
		Hash:       tipwire.Hash32(header.Hash()),
		ParentHash: tipwire.Hash32(header.ParentHash),
		StateRoot:  tipwire.Hash32(header.Root),
	}
	if err := ref.Validate(); err != nil {
		return tipwire.BlockRef{}, fmt.Errorf("invalid tip-state block reference: %w", err)
	}
	return ref, nil
}

func tipStateAssemblyPolicy(config *Config) ([]rawdb.WasmTarget, tipwire.RuntimePolicy, error) {
	local := rawdb.LocalTarget()
	cranelift, err := rawdb.CraneliftTarget(local)
	if err != nil {
		return nil, tipwire.RuntimePolicy{}, fmt.Errorf("derive local Cranelift target: %w", err)
	}
	targets := []rawdb.WasmTarget{local, cranelift}
	sort.Slice(targets, func(left, right int) bool { return targets[left] < targets[right] })
	names := make([]string, len(targets))
	for index, target := range targets {
		names[index] = string(target)
	}
	return targets, tipwire.RuntimePolicy{
		AssemblyTargets:       names,
		WasmCacheBytes:        uint64(config.Caching.StylusLRUCacheCapacity) << 20,
		WasmFallback:          config.StylusTarget.AllowFallback,
		NativeStackCacheBytes: config.StylusTarget.NativeStackSize,
	}, nil
}

func (n *ExecutionNode) tipStateWireRPCMetadata() (tipwire.RPCMetadata, error) {
	httpProfile, err := n.tipStateHTTPProfile.profile.WireHTTPProfile()
	if err != nil {
		return tipwire.RPCMetadata{}, fmt.Errorf("encode exact tip-state HTTP profile: %w", err)
	}
	accounts := make([]tipwire.Address20, len(n.tipStateRPCMetadata.accounts))
	for index, account := range n.tipStateRPCMetadata.accounts {
		accounts[index] = tipwire.Address20(account)
	}
	return tipwire.RPCMetadata{
		ClientVersion: n.tipStateRPCMetadata.clientVersion,
		Accounts:      accounts,
		HTTP:          httpProfile,
	}, nil
}

// initializeRemoteTipState performs the entire bootstrap while Nitro owns its
// blocking startup-exclusive scope. Bootstrap is acknowledged by all three
// members before the extractor emits its first record; Seal exposes the local
// seed and live session only after all three final SeedAck frames agree.
func (n *ExecutionNode) initializeRemoteTipState(ctx context.Context, config *Config) error {
	settings, err := config.TipState.remoteSettings()
	if err != nil {
		return err
	}
	producerInstance, err := newTipStateStartupIdentity("producer instance ID")
	if err != nil {
		return err
	}
	seedStreamID, err := newTipStateStartupIdentity("seed stream ID")
	if err != nil {
		return err
	}
	requestID, err := newTipStateStartupIdentity("bootstrap request ID")
	if err != nil {
		return err
	}
	scope, err := n.ExecEngine.AcquireStartupExclusive()
	if err != nil {
		return err
	}
	defer scope.Release()
	chain, err := scope.BlockChain()
	if err != nil {
		return err
	}
	chainConfig := chain.Config()
	if chainConfig == nil || chainConfig.ChainID == nil || !chainConfig.ChainID.IsUint64() || chainConfig.ChainID.Sign() <= 0 {
		return errors.New("remote tip-state requires a nonzero uint64 chain ID")
	}
	genesis := chain.GetHeaderByNumber(0)
	if genesis == nil || genesis.Number == nil || genesis.Number.Sign() != 0 {
		return errors.New("remote tip-state could not resolve the exact genesis header")
	}
	head := chain.CurrentBlock()
	anchor, err := tipStateBlockRef(head)
	if err != nil {
		return err
	}
	cohort := tipwire.CohortBinding{
		ChainID:            chainConfig.ChainID.Uint64(),
		GenesisHash:        tipwire.Hash32(genesis.Hash()),
		ProtocolDigest:     tipwire.ProtocolDigest(),
		ProducerInstanceID: producerInstance,
		MembershipEpoch:    settings.membershipEpoch,
		ActiveSetID:        settings.activeSetID,
		SeedStreamID:       seedStreamID,
	}
	if err := cohort.Validate(); err != nil {
		return fmt.Errorf("invalid remote tip-state cohort: %w", err)
	}

	proxyLimits := tipproxy.DefaultLimits()
	proxy, err := tipproxy.Dial(ctx, settings.proxySocket, mandatoryTipStateProxyUID, proxyLimits, settings.proxyTimeout)
	if err != nil {
		return fmt.Errorf("dial mandatory tip-state Unix proxy: %w", err)
	}
	keepProxy := false
	defer func() {
		if !keepProxy {
			_ = proxy.Close()
		}
	}()
	seedSession, err := fanout.NewSeedSession(cohort, settings.members, proxyLimits.Wire, proxy)
	if err != nil {
		return fmt.Errorf("create mandatory tip-state seed session: %w", err)
	}
	bootstrap := tipwire.BootstrapRequest{Cohort: cohort, RequestID: requestID, Anchor: anchor}
	if err := seedSession.Bootstrap(ctx, bootstrap); err != nil {
		return fmt.Errorf("bootstrap mandatory tip-state replicas: %w", err)
	}

	assemblyTargets, runtimePolicy, err := tipStateAssemblyPolicy(config)
	if err != nil {
		return err
	}
	rpcMetadata, err := n.tipStateWireRPCMetadata()
	if err != nil {
		return err
	}
	arbNodeConfig := &programs.ArbNodeConfig{
		MaxOpenPages:       config.StylusTarget.MaxStylusOpenPages,
		MaxStylusCallDepth: config.StylusTarget.MaxStylusCallDepth,
	}
	sink, err := fanout.NewSeedFanoutSink(fanout.SeedFanoutOptions{
		Context:          ctx,
		ProducerMode:     fanout.SeedProducerRemoteOnly,
		Cohort:           cohort,
		RequestID:        requestID,
		Limits:           proxyLimits.Wire,
		BatchTargetBytes: settings.seedBatchBytes,
		Session:          seedSession,
		ArbNodeConfig:    arbNodeConfig,
		RuntimePolicy:    runtimePolicy,
		RPCMetadata:      rpcMetadata,
		ServingPolicy:    settings.servingPolicy,
	})
	if err != nil {
		return fmt.Errorf("construct mandatory tip-state seed fanout: %w", err)
	}
	manifest, err := nitroseed.ExtractPinnedHead(ctx, chain, scope, sink, nitroseed.Options{
		AssemblyTargets: assemblyTargets,
	})
	if err != nil {
		return fmt.Errorf("seed mandatory RAM-only tip-state cohort: %w", err)
	}
	producer, producerOK := sink.RemoteProducer()
	live, liveOK := sink.LiveSession()
	binding, bindingOK := sink.Binding()
	if !producerOK || producer == nil || !liveOK || live == nil || !bindingOK {
		return errors.New("mandatory tip-state seed completed without a bounded producer and promoted three-replica session")
	}
	if local, ok := sink.LocalSink(); ok || local != nil {
		return errors.New("remote tip-state unexpectedly retained a local flat-state pipeline")
	}
	if store, ok := sink.Store(); ok || store != nil {
		return errors.New("remote tip-state unexpectedly retained a local generation store")
	}
	seedCursor := tipwire.Cursor{Tip: anchor}
	fatal := newRemoteTipStateFatalReporter(n.tipStateFatalErrChan)
	hook, err := fanout.NewRemoteCanonicalHook(producer, live, proxyLimits.Wire, fanout.HookOptions{
		Context:       ctx,
		GenesisHash:   cohort.GenesisHash,
		InitialCursor: seedCursor,
		ServingPolicy: settings.servingPolicy,
		Fatal:         fatal,
	})
	if err != nil {
		return fmt.Errorf("construct mandatory tip-state canonical fanout hook: %w", err)
	}
	keepHook := false
	defer func() {
		if !keepHook {
			_ = hook.Close()
		}
	}()
	if err := scope.CheckHeld(); err != nil {
		return fmt.Errorf("startup exclusivity lost before remote canonical hook installation: %w", err)
	}
	if manifest == nil || manifest.Header == nil || manifest.Header.Number == nil || !manifest.Header.Number.IsUint64() || manifest.Block == nil ||
		manifest.Header.Hash() != head.Hash() || manifest.Header.Root != head.Root ||
		manifest.Block.Hash() != manifest.Header.Hash() || manifest.Block.Root() != manifest.Header.Root ||
		manifest.Block.NumberU64() != manifest.Header.Number.Uint64() {
		return errors.New("remote tip-state seed manifest differs from the bootstrapped head")
	}
	if err := n.ExecEngine.SetCanonicalStateHook(hook, manifest.Header, fatal); err != nil {
		return fmt.Errorf("install mandatory tip-state fanout hook at seeded head: %w", err)
	}
	if err := hook.Start(); err != nil {
		return fmt.Errorf("admit mandatory tip-state seed cohort: %w", err)
	}

	n.tipStateRemote.Store(&tipStateRemoteRuntime{hook: hook, binding: binding, cursor: seedCursor})
	keepHook = true
	keepProxy = true
	log.Info("seeded and admitted remote RAM-only tip-state cohort",
		"block", manifest.Header.Number,
		"hash", manifest.Header.Hash(),
		"root", manifest.Header.Root,
		"membershipEpoch", cohort.MembershipEpoch,
		"activeSet", fmt.Sprintf("%x", cohort.ActiveSetID))
	return nil
}
