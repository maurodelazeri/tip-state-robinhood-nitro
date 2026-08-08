package gethexec

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"math/big"
	"net/http"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core"
	"github.com/ethereum/go-ethereum/core/rawdb"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"

	"github.com/offchainlabs/nitro/arbos"
	"github.com/offchainlabs/nitro/arbos/arbosState"
	"github.com/offchainlabs/nitro/arbos/arbostypes"
	"github.com/offchainlabs/nitro/arbos/l1pricing"
	"github.com/offchainlabs/nitro/cmd/chaininfo"
	"github.com/offchainlabs/nitro/statetransfer"
)

var productTipStateContract = common.HexToAddress("0x000000000000000000000000000000000000cafe")

const productTipStatePrivateKey = "0000000000000000000000000000000000000000000000000000000000000001"

type staticTipStateConfigFetcher struct{ config *Config }

func (f staticTipStateConfigFetcher) Get() *Config { return f.config }

func TestTipStateSameProcessProductLifecycle(t *testing.T) {
	chain := productTipStateNitroChain(t)
	config := &Config{TipState: DefaultTipStateConfig}
	config.TipState.Enable = true
	config.TipState.Listen = "127.0.0.1:0"
	config.TipState.CallTimeout = 5 * time.Second
	if err := config.TipState.Validate(); err != nil {
		t.Fatalf("validate tip-state config: %v", err)
	}

	engine := NewExecutionEngine(chain, 0, false, false, nil, nil)
	node := &ExecutionNode{
		ExecEngine:    engine,
		configFetcher: staticTipStateConfigFetcher{config: config},
	}
	fatalErrChan := make(chan error, 2)
	if err := node.SetTipStateFatalErrChan(fatalErrChan); err != nil {
		t.Fatalf("set product fatal error channel: %v", err)
	}
	if err := node.SetTipStateFatalErrChan(fatalErrChan); err == nil {
		t.Fatal("duplicate product fatal error channel was accepted")
	}

	targetConfig := DefaultStylusTargetConfig
	if err := targetConfig.Validate(); err != nil {
		t.Fatalf("validate Stylus target config: %v", err)
	}
	if err := engine.Initialize(0, &targetConfig); err != nil {
		t.Fatalf("initialize execution engine: %v", err)
	}
	if err := node.initializeTipState(context.Background()); err != nil {
		t.Fatalf("seed and install product tip-state runtime: %v", err)
	}
	t.Cleanup(node.stopTipState)
	if engine.canonicalStateHook == nil || engine.canonicalStateScope == nil {
		t.Fatal("product lifecycle did not install the scoped canonical state hook")
	}
	if engine.startupExclusive.Load() != nil {
		t.Fatal("product lifecycle retained startup exclusivity after hook installation")
	}
	runtime := node.tipState.Load()
	if runtime == nil {
		t.Fatal("product lifecycle did not retain the seeded RAM runtime")
	}
	seedHead := runtime.SeedHeader()
	if seedHead.Hash() != chain.CurrentBlock().Hash() || seedHead.Root != chain.CurrentBlock().Root {
		t.Fatal("product lifecycle seeded a head different from the canonical chain")
	}

	if err := node.startTipState(); err != nil {
		t.Fatalf("start product tip-state RPC: %v", err)
	}
	endpoint := "http://" + runtime.Address()
	want := common.BigToHash(big.NewInt(42)).Bytes()
	got := productTipStateEthCall(t, endpoint, productTipStateContract)
	if !bytes.Equal(got, want) {
		t.Fatalf("product eth_call returned 0x%x, want 0x%x", got, want)
	}
	oldGeneration := runtime.Generation()
	block := productTipStateAppendStorageUpdate(t, engine, chain, 43)
	newGeneration := runtime.Generation()
	if newGeneration == oldGeneration || newGeneration.Number() != oldGeneration.Number()+1 {
		t.Fatalf("live block did not atomically publish a successor: old=%d new=%d", oldGeneration.Number(), newGeneration.Number())
	}
	if newGeneration.Header().Hash() != block.Hash() || newGeneration.Header().Root != chain.CurrentBlock().Root {
		t.Fatal("published RAM generation does not match the scoped Geth canonical block")
	}
	oldStorage, err := oldGeneration.Image().Storage(productTipStateContract, common.Hash{})
	if err != nil {
		t.Fatalf("read pinned old generation storage: %v", err)
	}
	newStorage, err := newGeneration.Image().Storage(productTipStateContract, common.Hash{})
	if err != nil {
		t.Fatalf("read successor generation storage: %v", err)
	}
	if oldStorage != common.BigToHash(big.NewInt(42)) || newStorage != common.BigToHash(big.NewInt(43)) {
		t.Fatalf("MVCC storage old=%s new=%s, want 42 then 43", oldStorage, newStorage)
	}
	want = common.BigToHash(big.NewInt(43)).Bytes()
	got = productTipStateEthCall(t, endpoint, productTipStateContract)
	if !bytes.Equal(got, want) {
		t.Fatalf("post-block product eth_call returned 0x%x, want 0x%x", got, want)
	}
	if attempts := runtime.Generation().Database().FallbackAttempts(); attempts != 0 {
		t.Fatalf("product runtime attempted %d database fallbacks", attempts)
	}
	select {
	case err := <-fatalErrChan:
		t.Fatalf("unexpected product fatal error: %v", err)
	default:
	}
	runtime.fatal(errors.New("injected canonical publication failure"))
	if runtime.Failure() == nil {
		t.Fatal("terminal canonical failure did not poison the RAM RPC")
	}
	select {
	case <-fatalErrChan:
	case <-time.After(time.Second):
		t.Fatal("terminal canonical failure was not delivered to Nitro's fatal channel")
	}
	poisonedResponse, err := http.Post(endpoint, "application/json", bytes.NewReader([]byte(`{"jsonrpc":"2.0","id":2,"method":"eth_call","params":[]}`)))
	if err != nil {
		t.Fatalf("request poisoned endpoint: %v", err)
	}
	_ = poisonedResponse.Body.Close()
	if poisonedResponse.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("poisoned endpoint status=%d, want %d", poisonedResponse.StatusCode, http.StatusServiceUnavailable)
	}

	node.stopTipState()
	client := &http.Client{Timeout: time.Second}
	if _, err := client.Get(endpoint + "/healthz"); err == nil {
		t.Fatal("product tip-state RPC still accepted connections after stop")
	}
}

func productTipStateAppendStorageUpdate(t *testing.T, engine *ExecutionEngine, chain *core.BlockChain, value byte) *types.Block {
	t.Helper()
	privateKey, err := crypto.HexToECDSA(productTipStatePrivateKey)
	if err != nil {
		t.Fatal(err)
	}
	current := chain.CurrentBlock()
	nextNumber := new(big.Int).Add(current.Number, common.Big1)
	nextTime := current.Time + 1
	arbosVersion := types.DeserializeHeaderExtraInformation(current).ArbOSFormatVersion
	signer := types.MakeSigner(chain.Config(), nextNumber, nextTime, arbosVersion)
	data := make([]byte, common.HashLength)
	data[len(data)-1] = value
	feeCap := big.NewInt(1_000_000_000_000)
	if current.BaseFee != nil && current.BaseFee.Sign() > 0 {
		feeCap = new(big.Int).Mul(current.BaseFee, big.NewInt(2))
	}
	tx, err := types.SignNewTx(privateKey, signer, &types.DynamicFeeTx{
		ChainID:   chain.Config().ChainID,
		Nonce:     0,
		GasTipCap: new(big.Int),
		GasFeeCap: feeCap,
		Gas:       100_000_000,
		To:        &productTipStateContract,
		Value:     new(big.Int),
		Data:      data,
	})
	if err != nil {
		t.Fatalf("sign synthetic storage transaction: %v", err)
	}
	header := &arbostypes.L1IncomingMessageHeader{
		Kind:        arbostypes.L1MessageType_L2Message,
		Poster:      l1pricing.BatchPosterAddress,
		BlockNumber: current.Number.Uint64() + 1,
		Timestamp:   nextTime,
		L1BaseFee:   new(big.Int),
	}
	hooks := MakeZeroTxSizeSequencingHooksForTesting(types.Transactions{tx}, nil, nil, nil)

	engine.createBlocksMutex.Lock()
	defer engine.createBlocksMutex.Unlock()
	statedb, err := chain.StateAt(current.Root)
	if err != nil {
		t.Fatalf("open synthetic canonical state: %v", err)
	}
	block, statedb, receipts, err := arbos.ProduceBlockAdvanced(
		header,
		current.Nonce.Uint64(),
		current,
		statedb,
		chain,
		hooks,
		false,
		core.NewMessageSequencingContext(engine.wasmTargets),
		false,
		nil,
	)
	if err != nil {
		t.Fatalf("produce synthetic Nitro block: %v", err)
	}
	if block == nil || len(receipts) == 0 {
		t.Fatal("synthetic Nitro block produced no committed work")
	}
	for _, txErr := range hooks.txErrors {
		if txErr != nil {
			t.Fatalf("synthetic storage transaction failed: %v", txErr)
		}
	}
	if err := engine.appendBlock(block, statedb, receipts, 0); err != nil {
		t.Fatalf("append scoped synthetic Nitro block: %v", err)
	}
	return block
}

func TestTipStateFatalChannelMustBeBufferedAndPreInitialize(t *testing.T) {
	engine := new(ExecutionEngine)
	node := &ExecutionNode{ExecEngine: engine}
	if err := node.SetTipStateFatalErrChan(make(chan error)); err == nil {
		t.Fatal("unbuffered fatal error channel was accepted")
	}
	engine.startupLifecycle.Store(startupReady)
	if err := node.SetTipStateFatalErrChan(make(chan error, 1)); err == nil {
		t.Fatal("fatal error channel was accepted after execution-engine initialization")
	}
}

func productTipStateEthCall(t *testing.T, endpoint string, contract common.Address) []byte {
	t.Helper()
	body := []byte(`{"jsonrpc":"2.0","id":1,"method":"eth_call","params":[{"to":"` + contract.Hex() + `"},"latest"]}`)
	request, err := http.NewRequest(http.MethodPost, endpoint, bytes.NewReader(body))
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
	encoded, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	var decoded struct {
		Result string `json:"result"`
		Error  *struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatalf("decode product RPC response %s: %v", encoded, err)
	}
	if decoded.Error != nil {
		t.Fatalf("product RPC error %d: %s", decoded.Error.Code, decoded.Error.Message)
	}
	return common.FromHex(decoded.Result)
}

func productTipStateNitroChain(t *testing.T) *core.BlockChain {
	t.Helper()
	config := chaininfo.ArbitrumDevTestChainConfig()
	cache := core.DefaultConfig().WithStateScheme(rawdb.HashScheme)
	cache.SnapshotLimit = 32
	cache.SnapshotWait = true
	database := rawdb.NewMemoryDatabase()
	t.Cleanup(func() { database.Close() })
	privateKey, err := crypto.HexToECDSA(productTipStatePrivateKey)
	if err != nil {
		t.Fatal(err)
	}
	fundedAddress := crypto.PubkeyToAddress(privateKey.PublicKey)
	initData := &statetransfer.ArbosInitializationInfo{
		Accounts: []statetransfer.AccountInitializationInfo{
			{
				Addr:       productTipStateContract,
				EthBalance: new(big.Int),
				ContractInfo: &statetransfer.AccountInitContractInfo{
					Code: []byte{
						0x36, 0x15, 0x60, 0x0c, 0x57,
						0x60, 0x00, 0x35, 0x60, 0x00, 0x55, 0x00,
						0x5b, 0x60, 0x00, 0x54, 0x60, 0x00, 0x52, 0x60, 0x20, 0x60, 0x00, 0xf3,
					},
					ContractStorage: map[common.Hash]common.Hash{
						common.Hash{}: common.BigToHash(big.NewInt(42)),
					},
				},
			},
			{
				Addr:       fundedAddress,
				EthBalance: big.NewInt(1_000_000_000_000_000_000),
			},
		},
	}
	root, err := arbosState.InitializeArbosInDatabase(
		database,
		cache,
		statetransfer.NewMemoryInitDataReader(initData),
		config,
		nil,
		arbostypes.TestInitMessage,
		0,
		0,
	)
	if err != nil {
		t.Fatalf("initialize synthetic ArbOS database: %v", err)
	}
	genesis := arbosState.MakeGenesisBlock(
		common.Hash{}, config.ArbitrumChainParams.GenesisBlockNum, 0, root, config,
	)
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
