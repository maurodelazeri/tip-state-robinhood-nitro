// Copyright 2026, Offchain Labs, Inc.
// For license information, see https://github.com/OffchainLabs/nitro/blob/master/LICENSE.md

//go:build !wasm

package gethexec

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/log"
	"github.com/spf13/pflag"

	ramtipstate "nitro-tipstate-runtime/tipstate"
)

// TipStateConfig is deliberately opt-in. Same-process mode serves the local
// RAM image; remote mode makes an exact three-replica RAM cohort mandatory.
// Neither mode has a database, provider, or upstream RPC fallback.
type TipStateConfig struct {
	Enable       bool                 `koanf:"enable"`
	Mode         string               `koanf:"mode"`
	Listen       string               `koanf:"listen"`
	GasCap       uint64               `koanf:"gas-cap"`
	CallTimeout  time.Duration        `koanf:"call-timeout"`
	JournalLimit int                  `koanf:"journal-limit"`
	Remote       TipStateRemoteConfig `koanf:"remote"`
}

const (
	TipStateModeSameProcess = "same-process"
	TipStateModeRemote      = "remote"
)

// TipStateRemoteConfig fixes the admission policy for one exact mandatory
// three-member cohort. Nitro derives the active set from those member IDs and
// generates fresh producer, seed-stream, and request identities per startup;
// it never discovers or substitutes a replacement member.
type TipStateRemoteConfig struct {
	ProxySocket       string        `koanf:"proxy-socket"`
	ProxyTimeout      time.Duration `koanf:"proxy-timeout"`
	SeedBatchBytes    uint64        `koanf:"seed-batch-bytes"`
	LeaseDuration     time.Duration `koanf:"lease-duration"`
	HeartbeatInterval time.Duration `koanf:"heartbeat-interval"`
	OperationTimeout  time.Duration `koanf:"operation-timeout"`
	MembershipEpoch   uint64        `koanf:"membership-epoch"`
	MemberIDs         []string      `koanf:"member-ids"`
}

var DefaultTipStateRemoteConfig = TipStateRemoteConfig{
	ProxyTimeout:      30 * time.Minute,
	SeedBatchBytes:    32 << 20,
	LeaseDuration:     2 * time.Second,
	HeartbeatInterval: 500 * time.Millisecond,
	OperationTimeout:  time.Second,
}

var DefaultTipStateConfig = TipStateConfig{
	Enable:       false,
	Mode:         TipStateModeSameProcess,
	Listen:       ramtipstate.DefaultConfig.Listen,
	GasCap:       ramtipstate.DefaultConfig.GasCap,
	CallTimeout:  ramtipstate.DefaultConfig.CallTimeout,
	JournalLimit: ramtipstate.DefaultConfig.JournalLimit,
	Remote:       DefaultTipStateRemoteConfig,
}

// This value exists only to let TipStateConfig.Validate check static product
// knobs through the runtime validator. initializeTipState never uses it: Seed
// receives only metadata explicitly installed from the ordinary Nitro node.
const tipStateConfigValidationClientVersion = "tip-state-config-validation-only"

func (c *TipStateConfig) runtimeConfig(httpProfile ramtipstate.HTTPProfile, metadata tipStateRPCMetadata) ramtipstate.Config {
	accounts := make([]common.Address, len(metadata.accounts))
	copy(accounts, metadata.accounts)
	return ramtipstate.Config{
		Listen:        c.Listen,
		GasCap:        c.GasCap,
		CallTimeout:   c.CallTimeout,
		JournalLimit:  c.JournalLimit,
		HTTPProfile:   httpProfile,
		ClientVersion: metadata.clientVersion,
		Accounts:      accounts,
	}
}

func (c *TipStateConfig) Validate() error {
	if !c.Enable {
		return nil
	}
	switch c.Mode {
	case TipStateModeSameProcess:
		return c.runtimeConfig(ramtipstate.DefaultConfig.HTTPProfile, tipStateRPCMetadata{
			clientVersion: tipStateConfigValidationClientVersion,
		}).Validate()
	case TipStateModeRemote:
		_, err := c.remoteSettings()
		return err
	default:
		return fmt.Errorf("tip-state mode %q is not %q or %q", c.Mode, TipStateModeSameProcess, TipStateModeRemote)
	}
}

func TipStateConfigAddOptions(prefix string, f *pflag.FlagSet) {
	f.Bool(prefix+".enable", DefaultTipStateConfig.Enable, "enable the RAM-only current-tip integration")
	f.String(prefix+".mode", DefaultTipStateConfig.Mode, "tip-state mode: same-process or remote")
	f.String(prefix+".listen", DefaultTipStateConfig.Listen, "same-process tip-state HTTP JSON-RPC listen address")
	f.Uint64(prefix+".gas-cap", DefaultTipStateConfig.GasCap, "maximum gas accepted by tip-state call and trace execution")
	f.Duration(prefix+".call-timeout", DefaultTipStateConfig.CallTimeout, "timeout for one tip-state eth_call")
	f.Int(prefix+".journal-limit", DefaultTipStateConfig.JournalLimit, "number of committed in-memory deltas retained for atomic reorgs")
	f.String(prefix+".remote.proxy-socket", DefaultTipStateRemoteConfig.ProxySocket, "clean absolute Unix socket for the mandatory persistent-TCP fanout proxy")
	f.Duration(prefix+".remote.proxy-timeout", DefaultTipStateRemoteConfig.ProxyTimeout, "seed exchange ceiling; live operations use their shorter cohort deadline")
	f.Uint64(prefix+".remote.seed-batch-bytes", DefaultTipStateRemoteConfig.SeedBatchBytes, "target bytes per bounded remote seed batch")
	f.Duration(prefix+".remote.lease-duration", DefaultTipStateRemoteConfig.LeaseDuration, "mandatory cohort serving lease duration")
	f.Duration(prefix+".remote.heartbeat-interval", DefaultTipStateRemoteConfig.HeartbeatInterval, "mandatory cohort heartbeat interval")
	f.Duration(prefix+".remote.operation-timeout", DefaultTipStateRemoteConfig.OperationTimeout, "bound for each mandatory live cohort operation")
	f.Uint64(prefix+".remote.membership-epoch", DefaultTipStateRemoteConfig.MembershipEpoch, "exact nonzero mandatory cohort membership epoch")
	f.StringSlice(prefix+".remote.member-ids", DefaultTipStateRemoteConfig.MemberIDs, "three exact member IDs in strictly increasing hexadecimal order")
}

type tipStateRuntime struct {
	*ramtipstate.Runtime
	fatal func(error)
}

// tipStateHTTPProfile holds the immutable, defensively copied transport
// profile supplied by cmd/nitro before execution-engine initialization.
type tipStateHTTPProfile struct {
	profile ramtipstate.HTTPProfile
}

// tipStateRPCMetadata is the immutable process identity captured from the
// ordinary Nitro endpoint before execution-engine initialization. Accounts are
// a startup snapshot: the RAM endpoint never retains the account manager.
type tipStateRPCMetadata struct {
	clientVersion string
	accounts      []common.Address
}

// SetTipStateRPCMetadata supplies the exact ordinary Nitro client identity and
// a startup snapshot of its account-manager addresses. Both the input slice and
// the later runtime Config are defensively copied.
func (n *ExecutionNode) SetTipStateRPCMetadata(clientVersion string, accounts []common.Address) error {
	if n.tipStateRPCMetadata != nil {
		return errors.New("tip-state RPC metadata was already set")
	}
	if n.ExecEngine == nil {
		return errors.New("tip-state requires an execution engine")
	}
	if n.ExecEngine.startupLifecycle.Load() != startupUninitialized {
		return errors.New("tip-state RPC metadata must be set before execution-engine initialization")
	}
	if clientVersion == "" {
		return errors.New("tip-state RPC client version is empty")
	}
	accountSnapshot := make([]common.Address, len(accounts))
	copy(accountSnapshot, accounts)
	n.tipStateRPCMetadata = &tipStateRPCMetadata{
		clientVersion: clientVersion,
		accounts:      accountSnapshot,
	}
	return nil
}

// SetTipStateHTTPProfile supplies the ordinary Nitro HTTP/RPC transport
// profile before Initialize. The runtime must reproduce the node's configured
// CORS, vhost, prefix, timeout, body, and batch behavior instead of maintaining
// a second set of defaults.
func (n *ExecutionNode) SetTipStateHTTPProfile(profile ramtipstate.HTTPProfile) error {
	if n.tipStateHTTPProfile != nil {
		return errors.New("tip-state HTTP profile was already set")
	}
	if n.ExecEngine == nil {
		return errors.New("tip-state requires an execution engine")
	}
	if n.ExecEngine.startupLifecycle.Load() != startupUninitialized {
		return errors.New("tip-state HTTP profile must be set before execution-engine initialization")
	}
	if err := profile.Validate(); err != nil {
		return fmt.Errorf("invalid tip-state HTTP profile: %w", err)
	}
	cloned, err := ramtipstate.NewHTTPProfile(profile.Options())
	if err != nil {
		return fmt.Errorf("clone tip-state HTTP profile: %w", err)
	}
	n.tipStateHTTPProfile = &tipStateHTTPProfile{profile: cloned}
	return nil
}

// SetTipStateFatalErrChan supplies Nitro's process-wide fatal channel before
// Initialize. It must be buffered because the canonical hook and HTTP server
// are synchronous failure boundaries and must never block while reporting a
// terminal error.
func (n *ExecutionNode) SetTipStateFatalErrChan(fatalErrChan chan<- error) error {
	if fatalErrChan == nil {
		return errors.New("nil tip-state fatal error channel")
	}
	if cap(fatalErrChan) == 0 {
		return errors.New("tip-state fatal error channel must be buffered")
	}
	if n.tipStateFatalErrChan != nil {
		return errors.New("tip-state fatal error channel was already set")
	}
	if n.ExecEngine == nil {
		return errors.New("tip-state requires an execution engine")
	}
	if n.ExecEngine.startupLifecycle.Load() != startupUninitialized {
		return errors.New("tip-state fatal error channel must be set before execution-engine initialization")
	}
	n.tipStateFatalErrChan = fatalErrChan
	return nil
}

// initializeTipState runs after ExecutionEngine.Initialize and before
// Backend.Start. It holds the one-shot startup-exclusive scope across the
// complete snapshot and the expected-head hook installation.
func (n *ExecutionNode) initializeTipState(ctx context.Context) error {
	config := n.configFetcher.Get()
	if config == nil {
		return errors.New("nil execution-node config")
	}
	if !config.TipState.Enable {
		return nil
	}
	if n.tipStateFatalErrChan == nil {
		return errors.New("tip-state fatal error channel was not installed")
	}
	if n.tipStateHTTPProfile == nil {
		return errors.New("tip-state HTTP profile was not installed")
	}
	if n.tipStateRPCMetadata == nil {
		return errors.New("tip-state RPC metadata was not installed")
	}
	switch config.TipState.Mode {
	case TipStateModeSameProcess:
		return n.initializeSameProcessTipState(ctx, config)
	case TipStateModeRemote:
		return n.initializeRemoteTipState(ctx, config)
	default:
		return fmt.Errorf("invalid enabled tip-state mode %q", config.TipState.Mode)
	}
}

func (n *ExecutionNode) initializeSameProcessTipState(ctx context.Context, config *Config) error {
	scope, err := n.ExecEngine.AcquireStartupExclusive()
	if err != nil {
		return err
	}
	defer scope.Release()
	chain, err := scope.BlockChain()
	if err != nil {
		return err
	}
	runtime, err := ramtipstate.Seed(ctx, chain, scope, config.TipState.runtimeConfig(
		n.tipStateHTTPProfile.profile,
		*n.tipStateRPCMetadata,
	))
	if err != nil {
		return err
	}
	keepRuntime := false
	defer func() {
		if keepRuntime {
			return
		}
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = runtime.Stop(shutdownCtx)
	}()
	if err := scope.CheckHeld(); err != nil {
		return fmt.Errorf("startup exclusivity lost before canonical hook installation: %w", err)
	}
	fatal := newTipStateFatalReporter(runtime, n.tipStateFatalErrChan)
	if err := n.ExecEngine.SetCanonicalStateHook(runtime.Hook(), runtime.SeedHeader(), fatal); err != nil {
		return fmt.Errorf("install tip-state canonical hook at seeded head: %w", err)
	}

	n.tipState.Store(&tipStateRuntime{Runtime: runtime, fatal: fatal})
	keepRuntime = true
	generation := runtime.Generation()
	log.Info("seeded same-process RAM-only tip-state runtime",
		"block", generation.Header().Number,
		"hash", generation.Header().Hash(),
		"root", generation.Header().Root,
		"rpc", runtime.Address())
	return nil
}

func (n *ExecutionNode) startTipState() error {
	runtime := n.tipState.Load()
	if runtime == nil {
		return nil
	}
	if err := runtime.Start(runtime.fatal); err != nil {
		return err
	}
	log.Info("started same-process RAM-only tip-state RPC", "rpc", runtime.Address())
	return nil
}

func (n *ExecutionNode) stopTipState() {
	runtime := n.tipState.Swap(nil)
	if runtime != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		if err := runtime.Stop(ctx); err != nil {
			log.Error("failed to stop tip-state runtime", "err", err)
		}
		cancel()
	}
	remote := n.tipStateRemote.Swap(nil)
	if remote != nil {
		if err := remote.hook.Close(); err != nil {
			log.Error("failed to stop remote tip-state fanout", "err", err)
		}
	}
}

func newTipStateFatalReporter(runtime *ramtipstate.Runtime, fatalErrChan chan<- error) func(error) {
	return newTipStateFatalReporterWithPoison(runtime.Poison, fatalErrChan)
}

func newRemoteTipStateFatalReporter(fatalErrChan chan<- error) func(error) {
	return newTipStateFatalReporterWithPoison(nil, fatalErrChan)
}

func newTipStateFatalReporterWithPoison(poison func(error), fatalErrChan chan<- error) func(error) {
	return func(err error) {
		if err == nil {
			err = errors.New("unknown tip-state failure")
		}
		failure := fmt.Errorf("tip-state runtime failed: %w", err)
		if poison != nil {
			poison(failure)
		}
		select {
		case fatalErrChan <- failure:
		default:
			log.Crit("tip-state fatal error channel is full", "err", failure)
		}
	}
}
