package nodeinterface_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"math/big"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core"
	"github.com/ethereum/go-ethereum/core/rawdb"
	"github.com/ethereum/go-ethereum/core/types"

	"github.com/offchainlabs/nitro/arbos"
	"github.com/offchainlabs/nitro/arbos/arbosState"
	"github.com/offchainlabs/nitro/arbos/arbostypes"
	"github.com/offchainlabs/nitro/cmd/chaininfo"
	"github.com/offchainlabs/nitro/solgen/go/node_interfacegen"
	"github.com/offchainlabs/nitro/statetransfer"

	"nitro-tipstate-runtime/arbosruntime"
	"nitro-tipstate-runtime/engine"
	"nitro-tipstate-runtime/oracle"
	"nitro-tipstate-runtime/rpcserver"
	ramtipstate "nitro-tipstate-runtime/tipstate"
)

const nodeInterfaceRobinhoodFixture = "../../third_party/nitro-tipstate-runtime/testdata/conformance/robinhood_eth_call_matrix_1d1a4ae.json"

var (
	nodeInterfaceEstimateTarget = common.HexToAddress("0x000000000000000000000000000000000000bEEF")
	nodeInterfaceRevertTarget   = common.HexToAddress("0x000000000000000000000000000000000000bEee")
)

type nodeInterfaceHeldStartupLock struct{}

func (nodeInterfaceHeldStartupLock) CheckHeld() error { return nil }

func TestInProcessTipStateUsesFullNitroNodeInterfaceFailClosed(t *testing.T) {
	chain := nodeInterfaceTipStateChain(t)
	config := ramtipstate.DefaultConfig
	config.Listen = "127.0.0.1:0"
	config.CallTimeout = 5 * time.Second
	runtime, err := ramtipstate.Seed(context.Background(), chain, nodeInterfaceHeldStartupLock{}, config)
	if err != nil {
		t.Fatalf("seed in-process tip-state runtime: %v", err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = runtime.Stop(ctx)
	})
	if err := runtime.Start(func(err error) { t.Errorf("unexpected tip-state fatal error: %v", err) }); err != nil {
		t.Fatalf("start in-process tip-state RPC: %v", err)
	}
	endpoint := "http://" + runtime.Address()

	nodeABI, err := node_interfacegen.NodeInterfaceMetaData.GetAbi()
	if err != nil {
		t.Fatal(err)
	}
	gasEstimateInput, err := nodeABI.Pack("gasEstimateL1Component", common.Address{}, false, []byte{})
	if err != nil {
		t.Fatal(err)
	}
	result, rpcErr := nodeInterfaceTipStateCall(t, endpoint, gasEstimateInput)
	if rpcErr != nil {
		t.Fatalf("full Nitro gasEstimateL1Component failed on RAM backend: %s", rpcErr.Message)
	}
	decoded, err := nodeABI.Unpack("gasEstimateL1Component", result)
	if err != nil {
		t.Fatalf("decode gasEstimateL1Component result 0x%x: %v", result, err)
	}
	if len(decoded) != 3 {
		t.Fatalf("gasEstimateL1Component returned %d values, want 3", len(decoded))
	}

	// gasEstimateComponents is also state-contained. Its inner estimate must
	// start from a fresh canonical generation, so an outer override which turns
	// the target into an always-reverting contract cannot leak into it.
	componentsInput, err := nodeABI.Pack("gasEstimateComponents", nodeInterfaceEstimateTarget, false, []byte{})
	if err != nil {
		t.Fatal(err)
	}
	canonicalComponents, rpcErr := nodeInterfaceTipStateCall(t, endpoint, componentsInput)
	if rpcErr != nil {
		t.Fatalf("full Nitro gasEstimateComponents failed on RAM backend: %s", rpcErr.Message)
	}
	components, err := nodeABI.Unpack("gasEstimateComponents", canonicalComponents)
	if err != nil {
		t.Fatalf("decode gasEstimateComponents result 0x%x: %v", canonicalComponents, err)
	}
	if len(components) != 4 {
		t.Fatalf("gasEstimateComponents returned %d values, want 4", len(components))
	}
	overriddenComponents, rpcErr := nodeInterfaceTipStateCallWithTail(
		t,
		endpoint,
		componentsInput,
		map[string]any{
			nodeInterfaceEstimateTarget.Hex(): map[string]any{"code": "0x60006000fd"},
		},
		map[string]any{"gasLimit": "0x5208"},
	)
	if rpcErr != nil {
		t.Fatalf("outer-override gasEstimateComponents failed on RAM backend: %s", rpcErr.Message)
	}
	if !bytes.Equal(overriddenComponents, canonicalComponents) {
		t.Fatalf("outer overrides leaked into nested estimate\n got:  0x%x\n want: 0x%x", overriddenComponents, canonicalComponents)
	}
	revertInput, err := nodeABI.Pack("gasEstimateComponents", nodeInterfaceRevertTarget, false, []byte{})
	if err != nil {
		t.Fatal(err)
	}
	_, rpcErr = nodeInterfaceTipStateCall(t, endpoint, revertInput)
	if rpcErr == nil {
		t.Fatal("reverting nested gas estimate unexpectedly succeeded")
	}

	historyInput, err := nodeABI.Pack("findBatchContainingBlock", uint64(0))
	if err != nil {
		t.Fatal(err)
	}
	_, rpcErr = nodeInterfaceTipStateCall(t, endpoint, historyInput)
	if rpcErr == nil {
		t.Fatalf("history-dependent NodeInterface call did not fail closed: %+v", rpcErr)
	}
	if attempts := runtime.Generation().Database().FallbackAttempts(); attempts != 0 {
		t.Fatalf("full Nitro NodeInterface attempted %d forbidden fallbacks", attempts)
	}
}

// This test runs in nodeinterface_test rather than the runtime module so the
// real Nitro NodeInterface init hook has replaced the standalone hook exactly
// as it does in cmd/nitro. The complete proof-backed Robinhood corpus must
// still match byte-for-byte without any state fallback.
func TestInProcessTipStateFullNitroNodeInterfaceCorpus(t *testing.T) {
	loaded, err := oracle.Load(nodeInterfaceRobinhoodFixture)
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded.Calls) != 15 {
		t.Fatalf("matrix calls %d, want 15", len(loaded.Calls))
	}
	store, err := engine.NewStore(loaded.Generation)
	if err != nil {
		t.Fatal(err)
	}
	handler, err := rpcserver.NewHandler(store, arbosruntime.NewInProcess(arbosruntime.Config{
		GasCap:  600_000_000,
		Timeout: 5 * time.Second,
	}))
	if err != nil {
		t.Fatal(err)
	}

	for _, vector := range loaded.Calls {
		vector := vector
		t.Run(vector.Label, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(vector.RequestBody()))
			request.Header.Set("Content-Type", "application/json")
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, request)
			if response.Code != http.StatusOK {
				t.Fatalf("HTTP status %d: %s", response.Code, response.Body.String())
			}
			if err := vector.CompareResponse(response.Body.Bytes()); err != nil {
				t.Fatal(err)
			}
		})
	}
	if attempts := loaded.Generation.Database().FallbackAttempts(); attempts != 0 {
		t.Fatalf("forbidden state/database fallback attempts: %d", attempts)
	}
	if missing := loaded.Generation.Image().Counts().Missing; missing != 0 {
		t.Fatalf("strict witness missing reads: %d", missing)
	}
}

type nodeInterfaceRPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func nodeInterfaceTipStateCall(t *testing.T, endpoint string, input []byte) ([]byte, *nodeInterfaceRPCError) {
	return nodeInterfaceTipStateCallWithTail(t, endpoint, input)
}

func nodeInterfaceTipStateCallWithTail(t *testing.T, endpoint string, input []byte, tail ...any) ([]byte, *nodeInterfaceRPCError) {
	t.Helper()
	params := []any{
		map[string]any{"to": types.NodeInterfaceAddress, "input": hexutil.Encode(input)},
		"latest",
	}
	params = append(params, tail...)
	payload := struct {
		JSONRPC string `json:"jsonrpc"`
		ID      int    `json:"id"`
		Method  string `json:"method"`
		Params  []any  `json:"params"`
	}{
		JSONRPC: "2.0",
		ID:      1,
		Method:  "eth_call",
		Params:  params,
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	request, err := http.NewRequest(http.MethodPost, endpoint, bytes.NewReader(encoded))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Content-Type", "application/json")
	client := &http.Client{Timeout: 10 * time.Second}
	response, err := client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	var decoded struct {
		Result string                 `json:"result"`
		Error  *nodeInterfaceRPCError `json:"error"`
	}
	if err := json.Unmarshal(body, &decoded); err != nil {
		t.Fatalf("decode NodeInterface RPC response %s: %v", body, err)
	}
	return common.FromHex(decoded.Result), decoded.Error
}

func nodeInterfaceTipStateChain(t *testing.T) *core.BlockChain {
	t.Helper()
	config := chaininfo.ArbitrumDevTestChainConfig()
	cache := core.DefaultConfig().WithStateScheme(rawdb.HashScheme)
	cache.SnapshotLimit = 32
	cache.SnapshotWait = true
	database := rawdb.NewMemoryDatabase()
	t.Cleanup(func() { database.Close() })
	root, err := arbosState.InitializeArbosInDatabase(
		database,
		cache,
		statetransfer.NewMemoryInitDataReader(&statetransfer.ArbosInitializationInfo{
			Accounts: []statetransfer.AccountInitializationInfo{
				{
					Addr:       nodeInterfaceEstimateTarget,
					EthBalance: new(big.Int),
					ContractInfo: &statetransfer.AccountInitContractInfo{
						// Return storage slot zero, forcing real EVM execution and
						// the estimator's binary-search path.
						Code: []byte{0x60, 0x00, 0x54, 0x60, 0x00, 0x52, 0x60, 0x20, 0x60, 0x00, 0xf3},
						ContractStorage: map[common.Hash]common.Hash{
							common.Hash{}: common.BigToHash(big.NewInt(42)),
						},
					},
				},
				{
					Addr:       nodeInterfaceRevertTarget,
					EthBalance: new(big.Int),
					ContractInfo: &statetransfer.AccountInitContractInfo{
						Code: []byte{0x60, 0x00, 0x60, 0x00, 0xfd},
					},
				},
			},
		}),
		config,
		nil,
		arbostypes.TestInitMessage,
		0,
		0,
	)
	if err != nil {
		t.Fatalf("initialize synthetic ArbOS database: %v", err)
	}
	genesis := arbosState.MakeGenesisBlock(common.Hash{}, config.ArbitrumChainParams.GenesisBlockNum, 0, root, config)
	batch := database.NewBatch()
	core.WriteHeadBlock(batch, genesis)
	if err := batch.Write(); err != nil {
		t.Fatalf("write synthetic Nitro genesis: %v", err)
	}
	rawdb.WriteChainConfig(database, genesis.Hash(), config)
	chain, err := core.NewBlockChain(database, config, nil, arbos.Engine{IsSequencer: true}, cache)
	if err != nil {
		t.Fatalf("create synthetic Nitro blockchain: %v", err)
	}
	t.Cleanup(chain.Stop)
	if err := chain.Snapshots().Verify(chain.CurrentBlock().Root); err != nil {
		t.Fatalf("synthetic Nitro snapshot is incomplete: %v", err)
	}
	return chain
}
