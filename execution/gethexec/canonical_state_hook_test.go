package gethexec

import (
	"errors"
	"fmt"
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/consensus/ethash"
	"github.com/ethereum/go-ethereum/core"
	"github.com/ethereum/go-ethereum/core/rawdb"
	"github.com/ethereum/go-ethereum/core/state"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/params"
)

type testCanonicalPreparation struct {
	commits *int
	aborts  *int
}

func (p *testCanonicalPreparation) Commit() {
	if p.commits != nil {
		(*p.commits)++
	}
}

func (p *testCanonicalPreparation) Abort() {
	if p.aborts != nil {
		(*p.aborts)++
	}
}

type testCanonicalFrame struct {
	blockPrepares  int
	rewindPrepares int
	stepCommits    int
	stepAborts     int
	publishes      int
	aborts         int
	panicPublish   bool
}

func (f *testCanonicalFrame) PrepareBlock(_, _ *types.Header, _ *state.FlatStateUpdate) (core.PreparedCanonicalState, error) {
	f.blockPrepares++
	return &testCanonicalPreparation{commits: &f.stepCommits, aborts: &f.stepAborts}, nil
}

func (f *testCanonicalFrame) PrepareRewind(_, _ *types.Header) (core.PreparedCanonicalState, error) {
	f.rewindPrepares++
	return &testCanonicalPreparation{commits: &f.stepCommits, aborts: &f.stepAborts}, nil
}

func (f *testCanonicalFrame) Publish() {
	if f.panicPublish {
		panic("injected publish failure")
	}
	f.publishes++
}

func (f *testCanonicalFrame) Abort() { f.aborts++ }

type testCanonicalHook struct {
	calls  int
	frames []*testCanonicalFrame
}

func (h *testCanonicalHook) PrepareBlock(_ *core.CanonicalStateScope, _, _ *types.Header, _ *state.FlatStateUpdate) (core.PreparedCanonicalState, error) {
	h.calls++
	return &testCanonicalPreparation{}, nil
}

func (h *testCanonicalHook) PrepareRewind(_ *core.CanonicalStateScope, _, _ *types.Header) (core.PreparedCanonicalState, error) {
	h.calls++
	return &testCanonicalPreparation{}, nil
}

func (h *testCanonicalHook) BeginReorg(_, _ *types.Header) (core.CanonicalStateReorgFrame, error) {
	frame := new(testCanonicalFrame)
	h.frames = append(h.frames, frame)
	return frame, nil
}

func testHeaders() (*types.Header, *types.Header) {
	oldHead := &types.Header{Number: big.NewInt(7), GasLimit: 1}
	newHead := &types.Header{Number: big.NewInt(8), ParentHash: oldHead.Hash(), GasLimit: 1}
	return oldHead, newHead
}

func TestCanonicalStateHookInstallRequiresSeededHead(t *testing.T) {
	genesis := &core.Genesis{BaseFee: big.NewInt(params.InitialBaseFee), Config: params.AllEthashProtocolChanges}
	chain, err := core.NewBlockChain(rawdb.NewMemoryDatabase(), nil, genesis, ethash.NewFaker(), core.DefaultConfig().WithStateScheme(rawdb.HashScheme))
	if err != nil {
		t.Fatal(err)
	}
	defer chain.Stop()
	execution := &ExecutionEngine{bc: chain}
	hook := new(testCanonicalHook)
	fatalCalls := 0
	fatal := func(error) { fatalCalls++ }
	stale := types.CopyHeader(chain.CurrentBlock())
	stale.GasLimit++
	if err := execution.SetCanonicalStateHook(hook, stale, fatal); !errors.Is(err, core.ErrCanonicalStateHook) {
		t.Fatalf("stale seed error=%v, want ErrCanonicalStateHook", err)
	}
	if execution.canonicalStateHook != nil || execution.canonicalStateScope != nil || execution.canonicalStateFatal != nil {
		t.Fatal("stale seed partially installed the execution-engine hook")
	}
	if fatalCalls != 0 {
		t.Fatalf("installation error invoked runtime fatal callback %d times", fatalCalls)
	}
	if err := execution.SetCanonicalStateHook(hook, chain.CurrentBlock(), fatal); err != nil {
		t.Fatalf("exact seeded head was rejected: %v", err)
	}
	if execution.canonicalStateHook == nil || execution.canonicalStateScope == nil || execution.canonicalStateFatal == nil {
		t.Fatal("exact seeded head did not install the execution-engine hook")
	}
}

func TestCanonicalStateHookRequiresCreateBlocksMutexAndPoisonIsSticky(t *testing.T) {
	engine := new(ExecutionEngine)
	fatalCalls := 0
	engine.canonicalStateFatal = func(error) { fatalCalls++ }
	inner := new(testCanonicalHook)
	hook := &createBlocksLockedStateHook{engine: engine, inner: inner}
	installedScope := new(core.CanonicalStateScope)
	hook.scope = installedScope
	engine.canonicalStateHook = hook
	engine.canonicalStateScope = installedScope

	if _, err := hook.PrepareBlock(installedScope, nil, nil, nil); err == nil {
		t.Fatal("unlocked block callback was accepted")
	}
	first := engine.canonicalStatePoisoned()
	if first == nil || fatalCalls != 1 {
		t.Fatalf("first failure=%v fatal calls=%d, want a sticky failure and one callback", first, fatalCalls)
	}
	if _, err := hook.PrepareRewind(installedScope, nil, nil); err == nil || err.Error() != first.Error() {
		t.Fatalf("second callback did not return sticky poison: %v", err)
	}
	if fatalCalls != 1 || inner.calls != 0 {
		t.Fatalf("fatal calls=%d inner calls=%d, want 1 and 0", fatalCalls, inner.calls)
	}

	if _, err := engine.DigestMessage(0, nil, nil); err == nil || err.Error() != first.Error() {
		t.Fatalf("DigestMessage did not refuse poison immediately: %v", err)
	}
	if err := engine.appendBlock(nil, nil, nil, 0); err == nil || err.Error() != first.Error() {
		t.Fatalf("appendBlock did not refuse poison immediately: %v", err)
	}
	if _, err := engine.Reorg(0, nil, nil); err == nil || err.Error() != first.Error() {
		t.Fatalf("Reorg did not refuse poison immediately: %v", err)
	}
}

func TestCanonicalStateHookRejectsConcurrentForeignMutexHolder(t *testing.T) {
	engine := new(ExecutionEngine)
	fatalCalls := 0
	engine.canonicalStateFatal = func(error) { fatalCalls++ }
	inner := new(testCanonicalHook)
	hook := &createBlocksLockedStateHook{engine: engine, inner: inner}
	installedScope := new(core.CanonicalStateScope)
	hook.scope = installedScope
	engine.canonicalStateHook = hook
	engine.canonicalStateScope = installedScope

	locked := make(chan struct{})
	release := make(chan struct{})
	done := make(chan struct{})
	go func() {
		engine.createBlocksMutex.Lock()
		close(locked)
		<-release
		engine.createBlocksMutex.Unlock()
		close(done)
	}()
	<-locked
	foreignScope := new(core.CanonicalStateScope)
	_, err := hook.PrepareBlock(foreignScope, nil, nil, nil)
	close(release)
	<-done
	if err == nil {
		t.Fatal("callback was accepted merely because another goroutine held createBlocksMutex")
	}
	if fatalCalls != 1 || inner.calls != 0 {
		t.Fatalf("fatal calls=%d inner calls=%d, want 1 and 0", fatalCalls, inner.calls)
	}
}

func TestCanonicalStateReorgFramePublishesExactlyOnce(t *testing.T) {
	engine := new(ExecutionEngine)
	inner := new(testCanonicalHook)
	hook := &createBlocksLockedStateHook{engine: engine, inner: inner}
	installedScope := new(core.CanonicalStateScope)
	hook.scope = installedScope
	engine.canonicalStateHook = hook
	engine.canonicalStateScope = installedScope
	oldHead, newHead := testHeaders()

	engine.createBlocksMutex.Lock()
	defer engine.createBlocksMutex.Unlock()
	if _, err := hook.PrepareBlock(installedScope, oldHead, newHead, nil); err != nil {
		t.Fatal(err)
	}
	if inner.calls != 1 {
		t.Fatalf("ordinary preparations=%d, want 1", inner.calls)
	}

	if err := hook.beginReorg(newHead, oldHead); err != nil {
		t.Fatal(err)
	}
	frame := inner.frames[0]
	prepared, err := hook.PrepareRewind(installedScope, newHead, oldHead)
	if err != nil {
		t.Fatal(err)
	}
	prepared.Commit()
	for i := 0; i < 2; i++ {
		prepared, err := hook.PrepareBlock(installedScope, oldHead, newHead, nil)
		if err != nil {
			t.Fatal(err)
		}
		prepared.Commit()
	}
	if frame.publishes != 0 || frame.stepCommits != 3 {
		t.Fatalf("before publish: publications=%d sealed steps=%d", frame.publishes, frame.stepCommits)
	}
	if err := hook.publishReorg(); err != nil {
		t.Fatal(err)
	}
	if frame.publishes != 1 || frame.aborts != 0 {
		t.Fatalf("complete reorg publications=%d aborts=%d", frame.publishes, frame.aborts)
	}

	// A pure rollback still has one rewind step and exactly one publication.
	if err := hook.beginReorg(newHead, oldHead); err != nil {
		t.Fatal(err)
	}
	pureRollback := inner.frames[1]
	prepared, err = hook.PrepareRewind(installedScope, newHead, oldHead)
	if err != nil {
		t.Fatal(err)
	}
	prepared.Commit()
	if err := hook.publishReorg(); err != nil {
		t.Fatal(err)
	}
	if pureRollback.rewindPrepares != 1 || pureRollback.blockPrepares != 0 || pureRollback.publishes != 1 {
		t.Fatalf("pure rollback rewinds=%d blocks=%d publications=%d", pureRollback.rewindPrepares, pureRollback.blockPrepares, pureRollback.publishes)
	}

	if err := hook.beginReorg(newHead, oldHead); err != nil {
		t.Fatal(err)
	}
	aborted := inner.frames[2]
	if err := hook.abortReorg(); err != nil {
		t.Fatal(err)
	}
	if aborted.aborts != 1 || aborted.publishes != 0 {
		t.Fatalf("aborted frame aborts=%d publications=%d", aborted.aborts, aborted.publishes)
	}
}

func TestCanonicalStateReorgPublishPanicPoisonsEngine(t *testing.T) {
	engine := new(ExecutionEngine)
	fatalCalls := 0
	engine.canonicalStateFatal = func(error) { fatalCalls++ }
	inner := new(testCanonicalHook)
	hook := &createBlocksLockedStateHook{engine: engine, inner: inner}
	installedScope := new(core.CanonicalStateScope)
	hook.scope = installedScope
	engine.canonicalStateHook = hook
	engine.canonicalStateScope = installedScope
	oldHead, newHead := testHeaders()

	engine.createBlocksMutex.Lock()
	defer engine.createBlocksMutex.Unlock()
	if err := hook.beginReorg(newHead, oldHead); err != nil {
		t.Fatal(err)
	}
	inner.frames[0].panicPublish = true
	if err := hook.publishReorg(); err == nil {
		t.Fatal("publish panic was accepted")
	}
	if engine.canonicalStatePoisoned() == nil || fatalCalls != 1 {
		t.Fatalf("poison=%v fatal calls=%d", engine.canonicalStatePoisoned(), fatalCalls)
	}
	if hook.active != nil {
		t.Fatal("panicked published frame remained active")
	}
}

func TestCanonicalStateResultPoisonsCoreExportFailureOnce(t *testing.T) {
	engine := new(ExecutionEngine)
	fatalCalls := 0
	engine.canonicalStateFatal = func(error) { fatalCalls++ }
	ordinary := errors.New("ordinary append error")
	if got := engine.canonicalStateResult(ordinary); got != ordinary || engine.canonicalStatePoisoned() != nil {
		t.Fatalf("ordinary error was poisoned: %v", got)
	}

	exportFailure := fmt.Errorf("export flat update: %w", core.ErrCanonicalStateHook)
	first := engine.canonicalStateResult(exportFailure)
	if first == nil || fatalCalls != 1 {
		t.Fatalf("first result=%v fatal calls=%d", first, fatalCalls)
	}
	second := engine.canonicalStateResult(exportFailure)
	if second == nil || second.Error() != first.Error() || fatalCalls != 1 {
		t.Fatalf("sticky result=%v fatal calls=%d, first=%v", second, fatalCalls, first)
	}
}
