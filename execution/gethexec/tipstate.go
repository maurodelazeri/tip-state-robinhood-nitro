// Copyright 2026, Offchain Labs, Inc.
// For license information, see https://github.com/OffchainLabs/nitro/blob/master/LICENSE.md

//go:build !wasm

package gethexec

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/ethereum/go-ethereum/log"
	"github.com/spf13/pflag"

	ramtipstate "nitro-tipstate-runtime/tipstate"
)

// TipStateConfig is deliberately opt-in. The endpoint is backed only by the
// same-process RAM image and ArbOS runtime: it has no database, provider, or
// upstream RPC fallback.
type TipStateConfig struct {
	Enable       bool          `koanf:"enable"`
	Listen       string        `koanf:"listen"`
	GasCap       uint64        `koanf:"gas-cap"`
	CallTimeout  time.Duration `koanf:"call-timeout"`
	JournalLimit int           `koanf:"journal-limit"`
}

var DefaultTipStateConfig = TipStateConfig{
	Enable:       false,
	Listen:       ramtipstate.DefaultConfig.Listen,
	GasCap:       ramtipstate.DefaultConfig.GasCap,
	CallTimeout:  ramtipstate.DefaultConfig.CallTimeout,
	JournalLimit: ramtipstate.DefaultConfig.JournalLimit,
}

func (c *TipStateConfig) runtimeConfig(httpProfile ramtipstate.HTTPProfile) ramtipstate.Config {
	return ramtipstate.Config{
		Listen:       c.Listen,
		GasCap:       c.GasCap,
		CallTimeout:  c.CallTimeout,
		JournalLimit: c.JournalLimit,
		HTTPProfile:  httpProfile,
	}
}

func (c *TipStateConfig) Validate() error {
	if !c.Enable {
		return nil
	}
	return c.runtimeConfig(ramtipstate.DefaultConfig.HTTPProfile).Validate()
}

func TipStateConfigAddOptions(prefix string, f *pflag.FlagSet) {
	f.Bool(prefix+".enable", DefaultTipStateConfig.Enable, "enable the same-process RAM-only current-tip JSON-RPC endpoint")
	f.String(prefix+".listen", DefaultTipStateConfig.Listen, "tip-state HTTP JSON-RPC listen address")
	f.Uint64(prefix+".gas-cap", DefaultTipStateConfig.GasCap, "maximum gas accepted by tip-state call and trace execution")
	f.Duration(prefix+".call-timeout", DefaultTipStateConfig.CallTimeout, "timeout for one tip-state eth_call")
	f.Int(prefix+".journal-limit", DefaultTipStateConfig.JournalLimit, "number of committed in-memory deltas retained for atomic reorgs")
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

	scope, err := n.ExecEngine.AcquireStartupExclusive()
	if err != nil {
		return err
	}
	defer scope.Release()
	chain, err := scope.BlockChain()
	if err != nil {
		return err
	}
	runtime, err := ramtipstate.Seed(ctx, chain, scope, config.TipState.runtimeConfig(n.tipStateHTTPProfile.profile))
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
	if runtime == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := runtime.Stop(ctx); err != nil {
		log.Error("failed to stop tip-state runtime", "err", err)
	}
}

func newTipStateFatalReporter(runtime *ramtipstate.Runtime, fatalErrChan chan<- error) func(error) {
	return func(err error) {
		if err == nil {
			err = errors.New("unknown tip-state failure")
		}
		failure := fmt.Errorf("tip-state runtime failed: %w", err)
		runtime.Poison(failure)
		select {
		case fatalErrChan <- failure:
		default:
			log.Crit("tip-state fatal error channel is full", "err", failure)
		}
	}
}
