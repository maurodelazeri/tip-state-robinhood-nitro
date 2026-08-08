// Copyright 2026, Offchain Labs, Inc.
// For license information, see https://github.com/OffchainLabs/nitro/blob/master/LICENSE.md

//go:build !wasm

package gethexec

import (
	"errors"
	"sync/atomic"

	"github.com/ethereum/go-ethereum/core"
)

const (
	startupUninitialized uint32 = iota
	startupInitializing
	startupReady
	startupExclusiveHeld
	startupReleased
	startupStarted
	startupFailed
)

// StartupExclusiveScope is a one-shot ownership capability for work which must
// run after ExecutionEngine.Initialize and before block production starts. It
// owns createBlocksMutex for its entire lifetime; CheckHeld verifies ownership
// from the exact issued capability rather than probing sync.Mutex with TryLock.
type StartupExclusiveScope struct {
	engine *ExecutionEngine
	held   atomic.Bool
}

// AcquireStartupExclusive blocks until it owns createBlocksMutex. The scope is
// deliberately one-shot: a failed or repeated attempt cannot reopen the
// startup boundary after state seeding has begun.
func (s *ExecutionEngine) AcquireStartupExclusive() (*StartupExclusiveScope, error) {
	if s == nil {
		return nil, errors.New("nil execution engine")
	}
	if !s.startupLifecycle.CompareAndSwap(startupReady, startupExclusiveHeld) {
		return nil, errors.New("startup exclusivity requires a ready execution engine and is issued only once")
	}

	s.createBlocksMutex.Lock()
	scope := &StartupExclusiveScope{engine: s}
	scope.held.Store(true)
	s.startupExclusive.Store(scope)
	return scope, nil
}

// CheckHeld implements nitroseed.HeadLock. It validates the exact live
// capability and does not infer ownership from whether some goroutine has the
// mutex locked.
func (s *StartupExclusiveScope) CheckHeld() error {
	if s == nil || s.engine == nil || !s.held.Load() {
		return errors.New("startup-exclusive scope is not held")
	}
	if s.engine.startupExclusive.Load() != s {
		return errors.New("startup-exclusive scope is not active")
	}
	if s.engine.startupLifecycle.Load() != startupExclusiveHeld {
		return errors.New("execution-engine startup lifecycle left exclusive state")
	}
	return nil
}

// BlockChain exposes the already-open local Geth chain only while this scope
// owns the startup boundary. The caller must not retain it after Release.
func (s *StartupExclusiveScope) BlockChain() (*core.BlockChain, error) {
	if err := s.CheckHeld(); err != nil {
		return nil, err
	}
	if s.engine.bc == nil {
		return nil, errors.New("execution engine has no blockchain")
	}
	return s.engine.bc, nil
}

// Release closes the startup boundary and unlocks createBlocksMutex. It is
// idempotent so it is safe in deferred initialization cleanup.
func (s *StartupExclusiveScope) Release() {
	if s == nil || s.engine == nil || !s.held.CompareAndSwap(true, false) {
		return
	}
	if !s.engine.startupExclusive.CompareAndSwap(s, nil) {
		s.engine.createBlocksMutex.Unlock()
		s.engine.startupLifecycle.Store(startupFailed)
		panic("startup-exclusive capability ownership corrupted")
	}
	s.engine.createBlocksMutex.Unlock()
	if !s.engine.startupLifecycle.CompareAndSwap(startupExclusiveHeld, startupReleased) {
		s.engine.startupLifecycle.Store(startupFailed)
		panic("startup-exclusive lifecycle ownership corrupted")
	}
}
