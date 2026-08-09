package gethexec

import (
	"context"
	"encoding/hex"
	"errors"
	"io"
	"math/big"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/spf13/pflag"

	runtimeengine "nitro-tipstate-runtime/engine"
	"nitro-tipstate-runtime/fanout"
	"nitro-tipstate-runtime/replica"
	"nitro-tipstate-runtime/tipproxy"
	ramtipstate "nitro-tipstate-runtime/tipstate"
	"nitro-tipstate-runtime/tipwire"
)

func tipStateTestHash(value byte) tipwire.Hash32 {
	var result tipwire.Hash32
	for index := range result {
		result[index] = value
	}
	return result
}

func tipStateTestHashText(value tipwire.Hash32) string {
	return hex.EncodeToString(value[:])
}

func validRemoteTipStateConfig(t *testing.T, proxySocket string) (TipStateConfig, []tipwire.Hash32) {
	t.Helper()
	members := []tipwire.Hash32{
		tipStateTestHash(0x11),
		tipStateTestHash(0x22),
		tipStateTestHash(0x33),
	}
	config := DefaultTipStateConfig
	config.Enable = true
	config.Mode = TipStateModeRemote
	config.Remote.ProxySocket = proxySocket
	config.Remote.ProxyTimeout = time.Second
	config.Remote.SeedBatchBytes = 4 << 10
	config.Remote.LeaseDuration = 300 * time.Millisecond
	config.Remote.HeartbeatInterval = 20 * time.Millisecond
	config.Remote.OperationTimeout = 500 * time.Millisecond
	config.Remote.MembershipEpoch = 7
	config.Remote.MemberIDs = make([]string, len(members))
	for index, member := range members {
		config.Remote.MemberIDs[index] = tipStateTestHashText(member)
	}
	return config, members
}

func TestTipStateRemoteConfigRequiresExactThreeMemberIdentity(t *testing.T) {
	valid, members := validRemoteTipStateConfig(t, "/run/robinhood-tipstate/fanout.sock")
	if err := valid.Validate(); err != nil {
		t.Fatalf("valid remote config: %v", err)
	}
	settings, err := valid.remoteSettings()
	if err != nil {
		t.Fatal(err)
	}
	if len(settings.members) != mandatoryTipStateReplicaCount {
		t.Fatalf("parsed member count = %d", len(settings.members))
	}
	if !reflect.DeepEqual(settings.members, members) {
		t.Fatalf("parsed member identities = %#v, want %#v", settings.members, members)
	}
	if settings.servingPolicy.RPCGasCap != valid.GasCap ||
		settings.servingPolicy.RPCCallTimeoutNanos != uint64(valid.CallTimeout) ||
		settings.servingPolicy.JournalLimit != uint32(valid.JournalLimit) {
		t.Fatalf("authenticated serving policy = %#v", settings.servingPolicy)
	}
	if settings.servingPolicy.OperationTimeoutNanos <= settings.servingPolicy.LeaseDurationNanos {
		t.Fatalf("operation timeout was not independent of lease: %#v", settings.servingPolicy)
	}

	tests := []struct {
		name string
		edit func(*TipStateConfig)
		want string
	}{
		{"unknown mode", func(c *TipStateConfig) { c.Mode = "automatic" }, "mode"},
		{"relative socket", func(c *TipStateConfig) { c.Remote.ProxySocket = "fanout.sock" }, "absolute"},
		{"unclean socket", func(c *TipStateConfig) { c.Remote.ProxySocket += "/../fanout.sock" }, "clean absolute"},
		{"two members", func(c *TipStateConfig) { c.Remote.MemberIDs = c.Remote.MemberIDs[:2] }, "exactly 3"},
		{"member order", func(c *TipStateConfig) {
			c.Remote.MemberIDs[0], c.Remote.MemberIDs[1] = c.Remote.MemberIDs[1], c.Remote.MemberIDs[0]
		}, "strictly increasing"},
		{"zero epoch", func(c *TipStateConfig) { c.Remote.MembershipEpoch = 0 }, "nonzero"},
		{"oversized journal", func(c *TipStateConfig) { c.JournalLimit = 257 }, "live bounds"},
		{"unsafe heartbeat", func(c *TipStateConfig) { c.Remote.HeartbeatInterval = c.Remote.LeaseDuration / 2 }, "heartbeat_interval"},
		{"unsafe proxy ceiling", func(c *TipStateConfig) { c.Remote.ProxyTimeout = c.Remote.OperationTimeout }, "strictly greater"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			candidate, _ := validRemoteTipStateConfig(t, "/run/robinhood-tipstate/fanout.sock")
			test.edit(&candidate)
			err := candidate.Validate()
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("Validate error = %v, want substring %q", err, test.want)
			}
		})
	}
}

func TestTipStateRemoteProductionTimingDefaults(t *testing.T) {
	remote := DefaultTipStateRemoteConfig
	if remote.LeaseDuration != 2*time.Second || remote.HeartbeatInterval != 500*time.Millisecond || remote.OperationTimeout != 5*time.Second {
		t.Fatalf("remote timing defaults lease=%s heartbeat=%s operation=%s", remote.LeaseDuration, remote.HeartbeatInterval, remote.OperationTimeout)
	}
	if remote.ProxyTimeout != 30*time.Minute || remote.ProxyTimeout <= remote.OperationTimeout {
		t.Fatalf("remote proxy ceiling=%s operation=%s", remote.ProxyTimeout, remote.OperationTimeout)
	}
}

func TestTipStateRemoteProtocolIdentityIsCompiledNotSelectable(t *testing.T) {
	flags := pflag.NewFlagSet("tip-state", pflag.ContinueOnError)
	TipStateConfigAddOptions("execution.tip-state", flags)
	if flag := flags.Lookup("execution.tip-state.remote.protocol-digest"); flag != nil {
		t.Fatalf("operator-selectable protocol flag remains registered: %s", flag.Name)
	}
	if tipwire.ProtocolDigest() == (tipwire.Hash32{}) {
		t.Fatal("compiled protocol identity is zero")
	}
}

func TestTipStateModeIsExplicitAndSameProcessCompatible(t *testing.T) {
	config := DefaultTipStateConfig
	config.Enable = true
	config.Mode = ""
	if err := config.Validate(); err == nil {
		t.Fatal("enabled empty mode was accepted")
	}
	config.Mode = TipStateModeSameProcess
	if err := config.Validate(); err != nil {
		t.Fatalf("same-process mode depends on empty remote config: %v", err)
	}
	config.Enable = false
	config.Mode = "ignored-while-disabled"
	if err := config.Validate(); err != nil {
		t.Fatalf("disabled tip-state validated inactive mode: %v", err)
	}
}

type tipStateTestCloser struct{ closed atomic.Bool }

func (c *tipStateTestCloser) Close() error {
	c.closed.Store(true)
	return nil
}

type tipStateReceiverExchange struct {
	members   []tipwire.Hash32
	receivers []*replica.Receiver
	fail      atomic.Bool
	closed    atomic.Bool
}

func (e *tipStateReceiverExchange) Exchange(ctx context.Context, frame []byte) ([]fanout.MemberReply, error) {
	if e.closed.Load() {
		return nil, errors.New("test mandatory cohort is closed")
	}
	if e.fail.Load() {
		return nil, errors.New("injected mandatory member transport failure")
	}
	replies := make([]fanout.MemberReply, len(e.receivers))
	for index, receiver := range e.receivers {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		reply, err := receiver.HandleFrame(frame)
		if err != nil {
			return nil, err
		}
		replies[index] = fanout.MemberReply{MemberID: e.members[index], Frame: reply}
	}
	return replies, nil
}

func (e *tipStateReceiverExchange) Close() error {
	if e.closed.Swap(true) {
		return nil
	}
	for _, receiver := range e.receivers {
		_ = receiver.Poison(errors.New("test mandatory cohort closed"))
	}
	return nil
}

type tipStateManifestCapture struct {
	mu        sync.Mutex
	manifests []tipwire.SeedManifest
	closers   []*tipStateTestCloser
}

func (c *tipStateManifestCapture) prepare(manifest tipwire.SeedManifest, generation *runtimeengine.Generation) (io.Closer, error) {
	if generation == nil {
		return nil, errors.New("nil verified seed generation")
	}
	closer := new(tipStateTestCloser)
	c.mu.Lock()
	c.manifests = append(c.manifests, manifest)
	c.closers = append(c.closers, closer)
	c.mu.Unlock()
	return closer, nil
}

func (c *tipStateManifestCapture) snapshot() ([]tipwire.SeedManifest, []*tipStateTestCloser) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]tipwire.SeedManifest(nil), c.manifests...), append([]*tipStateTestCloser(nil), c.closers...)
}

func TestTipStateRemoteThreeReplicaLifecycleAndFatalNoFallback(t *testing.T) {
	chain := productTipStateNitroChain(t)
	proxySocket := t.TempDir() + "/fanout.sock"
	tipConfig, memberIDs := validRemoteTipStateConfig(t, proxySocket)
	config := &Config{
		TipState:     tipConfig,
		Caching:      DefaultCachingConfig,
		StylusTarget: DefaultStylusTargetConfig,
	}
	if err := config.StylusTarget.Validate(); err != nil {
		t.Fatal(err)
	}
	settings, err := config.TipState.remoteSettings()
	if err != nil {
		t.Fatal(err)
	}
	genesis := chain.GetHeaderByNumber(0)
	admission := replica.CohortAdmission{
		ChainID: chain.Config().ChainID.Uint64(), GenesisHash: tipwire.Hash32(genesis.Hash()),
		MembershipEpoch: settings.membershipEpoch,
		ActiveSetID:     settings.activeSetID,
	}
	capture := new(tipStateManifestCapture)
	receivers := make([]*replica.Receiver, len(settings.members))
	for index, member := range settings.members {
		receiver, err := replica.NewReceiver(replica.Options{
			MemberID: member, Admission: admission, Limits: tipwire.DefaultLimits(),
			ServingPolicy: settings.servingPolicy, PrepareServing: capture.prepare,
		})
		if err != nil {
			t.Fatalf("construct receiver %d: %v", index, err)
		}
		receivers[index] = receiver
	}
	exchange := &tipStateReceiverExchange{members: settings.members, receivers: receivers}
	proxy, err := tipproxy.Listen(proxySocket, mandatoryTipStateProxyUID, tipproxy.DefaultLimits(), time.Second, exchange)
	if err != nil {
		t.Fatal(err)
	}
	processCtx, cancel := context.WithCancel(context.Background())
	serveResult := make(chan error, 1)
	go func() { serveResult <- proxy.Serve(processCtx) }()
	t.Cleanup(func() {
		cancel()
		_ = proxy.Close()
	})

	engine := NewExecutionEngine(chain, 0, false, false, nil, nil)
	node := &ExecutionNode{ExecEngine: engine, configFetcher: staticTipStateConfigFetcher{config: config}}
	accounts := []common.Address{productTipStateSender(t), productTipStateContract, productTipStateSender(t)}
	if err := node.SetTipStateRPCMetadata(productTipStateClientVersion, accounts); err != nil {
		t.Fatal(err)
	}
	httpProfile, err := ramtipstate.NewHTTPProfile(ramtipstate.HTTPProfileOptions{
		CORSAllowedOrigins: []string{"https://z.example", "https://a.example", "https://z.example"},
		VirtualHosts:       []string{"z.example", "a.example", "z.example"},
		RPCPrefix:          "/exact",
		BatchRequestLimit:  17,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := node.SetTipStateHTTPProfile(httpProfile); err != nil {
		t.Fatal(err)
	}
	fatalErrChan := make(chan error, 2)
	if err := node.SetTipStateFatalErrChan(fatalErrChan); err != nil {
		t.Fatal(err)
	}
	if err := engine.Initialize(config.Caching.StylusLRUCacheCapacity, &config.StylusTarget); err != nil {
		t.Fatal(err)
	}
	if err := node.initializeTipState(processCtx); err != nil {
		t.Fatalf("initialize remote producer: %v", err)
	}
	if engine.startupExclusive.Load() != nil {
		t.Fatal("remote initialization retained startup exclusivity")
	}
	remote := node.tipStateRemote.Load()
	if remote == nil || remote.hook == nil || engine.canonicalStateHook == nil || engine.canonicalStateScope == nil {
		t.Fatal("remote initialization did not retain the live hook and scoped Geth installation")
	}
	if node.tipState.Load() != nil {
		t.Fatal("remote mode created a same-process RPC runtime fallback")
	}
	retention, retentionOK := remote.hook.RemoteRetention()
	if !retentionOK || retention.HasLocalImage || retention.HasLocalStore || retention.RetainedDeltas != 0 ||
		retention.RetainedDeltaBytes != 0 || retention.RetainedEvidenceBytes != 0 {
		t.Fatalf("remote producer retained a local seed image/store: %+v / %v", retention, retentionOK)
	}
	if remote.binding.Cohort.ActiveSetID != settings.activeSetID || remote.cursor.Tip.Hash != tipwire.Hash32(chain.CurrentBlock().Hash()) {
		t.Fatalf("retained remote evidence = %#v / %#v", remote.binding, remote.cursor)
	}
	cohort := remote.binding.Cohort
	if cohort.ProducerInstanceID == (tipwire.Hash32{}) || cohort.SeedStreamID == (tipwire.Hash32{}) {
		t.Fatalf("remote startup did not generate fresh incarnation identities: %#v", cohort)
	}
	derivedActiveSet, err := tipwire.DeriveActiveSetID(memberIDs)
	if err != nil || derivedActiveSet != remote.binding.Cohort.ActiveSetID {
		t.Fatalf("active-set derivation = %x, %v", derivedActiveSet, err)
	}

	wireHTTP, err := httpProfile.WireHTTPProfile()
	if err != nil {
		t.Fatal(err)
	}
	manifests, _ := capture.snapshot()
	if len(manifests) != mandatoryTipStateReplicaCount {
		t.Fatalf("prepared manifests = %d, want %d", len(manifests), mandatoryTipStateReplicaCount)
	}
	for index, manifest := range manifests {
		if manifest.Cohort != cohort || manifest.ServingPolicy != settings.servingPolicy {
			t.Fatalf("receiver %d authenticated cohort/policy mismatch", index)
		}
		if manifest.RequestID == (tipwire.Hash32{}) {
			t.Fatalf("receiver %d manifest has a zero startup request ID", index)
		}
		if manifest.RPCMetadata.ClientVersion != productTipStateClientVersion || !reflect.DeepEqual(manifest.RPCMetadata.HTTP, wireHTTP) {
			t.Fatalf("receiver %d RPC/HTTP metadata = %#v", index, manifest.RPCMetadata)
		}
		wantAccounts := []tipwire.Address20{tipwire.Address20(accounts[0]), tipwire.Address20(accounts[1]), tipwire.Address20(accounts[2])}
		if !reflect.DeepEqual(manifest.RPCMetadata.Accounts, wantAccounts) {
			t.Fatalf("receiver %d account order = %x, want %x", index, manifest.RPCMetadata.Accounts, wantAccounts)
		}
	}
	for index, receiver := range receivers {
		generation, err := receiver.Pin()
		if err != nil || generation.Hash() != chain.CurrentBlock().Hash() || generation.Root() != chain.CurrentBlock().Root {
			t.Fatalf("receiver %d initial generation = %v, %v", index, generation, err)
		}
		if attempts := generation.Database().FallbackAttempts(); attempts != 0 {
			t.Fatalf("receiver %d initial fallback attempts = %d", index, attempts)
		}
	}

	block := productTipStateAppendStorageUpdate(t, engine, chain, 43)
	retention, retentionOK = remote.hook.RemoteRetention()
	if !retentionOK || retention.HasLocalImage || retention.HasLocalStore || retention.RetainedDeltas != 1 || retention.RetainedOperations == 0 ||
		retention.RetainedDeltaBytes != 0 || retention.RetainedEvidenceBytes == 0 || retention.AcknowledgedDeltaBytes == 0 {
		t.Fatalf("remote producer successor retention = %+v / %v", retention, retentionOK)
	}
	for index, receiver := range receivers {
		generation, err := receiver.Pin()
		if err != nil || generation.Hash() != block.Hash() || generation.Root() != block.Root() {
			t.Fatalf("receiver %d successor generation = %v, %v", index, generation, err)
		}
		storage, err := generation.Image().Storage(productTipStateContract, common.Hash{})
		if err != nil || storage != common.BigToHash(big.NewInt(43)) {
			t.Fatalf("receiver %d successor storage = %s, %v", index, storage, err)
		}
		if attempts := generation.Database().FallbackAttempts(); attempts != 0 {
			t.Fatalf("receiver %d successor fallback attempts = %d", index, attempts)
		}
	}

	exchange.fail.Store(true)
	select {
	case fatal := <-fatalErrChan:
		if fatal == nil || !strings.Contains(fatal.Error(), "mandatory") {
			t.Fatalf("remote fatal error = %v", fatal)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("mandatory cohort failure did not reach Nitro fatal supervision")
	}
	if node.tipState.Load() != nil {
		t.Fatal("mandatory cohort failure activated a same-process fallback")
	}
	node.stopTipState()
	if node.tipStateRemote.Load() != nil {
		t.Fatal("remote runtime remained retained after stop")
	}
	select {
	case err := <-serveResult:
		if err == nil {
			t.Fatal("proxy stopped after injected failure without an error")
		}
	case <-time.After(time.Second):
		t.Fatal("proxy did not stop after injected mandatory exchange failure")
	}
	_, closers := capture.snapshot()
	_ = proxy.Close()
	for index, closer := range closers {
		if !closer.closed.Load() {
			t.Fatalf("receiver %d retained its prepared serving stack after terminal close", index)
		}
	}
}

var _ fanout.Exchange = (*tipStateReceiverExchange)(nil)
var _ io.Closer = (*tipStateTestCloser)(nil)
