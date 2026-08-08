package gethexec

import (
	"context"
	"sync"
	"testing"

	"github.com/ethereum/go-ethereum/core"
)

func TestStartupExclusiveCapabilityOwnership(t *testing.T) {
	chain := new(core.BlockChain)
	engine := &ExecutionEngine{bc: chain}
	if _, err := engine.AcquireStartupExclusive(); err == nil {
		t.Fatal("startup-exclusive scope was issued before initialization")
	}
	engine.startupLifecycle.Store(startupReady)
	scope, err := engine.AcquireStartupExclusive()
	if err != nil {
		t.Fatalf("acquire startup-exclusive scope: %v", err)
	}
	if err := scope.CheckHeld(); err != nil {
		t.Fatalf("issued scope is not held: %v", err)
	}
	if got, err := scope.BlockChain(); err != nil || got != chain {
		t.Fatalf("scope blockchain = %p, %v; want %p", got, err, chain)
	}
	foreign := &StartupExclusiveScope{engine: engine}
	foreign.held.Store(true)
	if err := foreign.CheckHeld(); err == nil {
		t.Fatal("foreign startup-exclusive capability was accepted")
	}
	if err := engine.Start(context.Background()); err == nil {
		t.Fatal("execution engine started while startup-exclusive scope was held")
	}

	scope.Release()
	scope.Release()
	if err := scope.CheckHeld(); err == nil {
		t.Fatal("released startup-exclusive scope remained valid")
	}
	if _, err := engine.AcquireStartupExclusive(); err == nil {
		t.Fatal("one-shot startup-exclusive scope was reissued")
	}
	if !engine.createBlocksMutex.TryLock() {
		t.Fatal("Release did not unlock createBlocksMutex")
	}
	engine.createBlocksMutex.Unlock()
}

func TestStartupExclusiveRacesEngineStartAtomically(t *testing.T) {
	for iteration := 0; iteration < 100; iteration++ {
		engine := NewExecutionEngine(nil, 0, false, false, nil, nil)
		engine.disableStylusCacheMetricsCollection = true
		engine.startupLifecycle.Store(startupReady)
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		start := make(chan struct{})
		var (
			scope      *StartupExclusiveScope
			acquireErr error
			startErr   error
			wait       sync.WaitGroup
		)
		wait.Add(2)
		go func() {
			defer wait.Done()
			<-start
			scope, acquireErr = engine.AcquireStartupExclusive()
		}()
		go func() {
			defer wait.Done()
			<-start
			startErr = engine.Start(ctx)
		}()
		close(start)
		wait.Wait()

		if (acquireErr == nil) == (startErr == nil) {
			t.Fatalf("iteration %d: acquire error=%v start error=%v; exactly one transition must win", iteration, acquireErr, startErr)
		}
		if acquireErr == nil {
			if err := scope.CheckHeld(); err != nil {
				t.Fatalf("iteration %d: winning exclusive scope invalid: %v", iteration, err)
			}
			scope.Release()
		} else {
			engine.StopAndWait()
		}
	}
}
