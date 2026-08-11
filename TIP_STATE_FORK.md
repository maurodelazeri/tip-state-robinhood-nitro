# Robinhood Tip-State Nitro Fork

This is the producer and embedding-client half of the Robinhood current-tip
RAM service. The paired runtime is
[`maurodelazeri/tip-state-robinhood-runtime`](https://github.com/maurodelazeri/tip-state-robinhood-runtime).
Read this file before building, changing, rebasing, or recovering either
repository.

The two repositories are a single authenticated product source. This fork
owns Nitro lifecycle and canonical-publication integration. The runtime owns
the flat-state implementation, RPC service, three-replica transport, Geth
patches, source lock, deterministic materializers, deployment units, and live
qualification procedure. Neither repository can be substituted with an
unrelated checkout that happens to have the same release number.

## Recovery contract

A clean clone of both repositories is sufficient to reconstruct and test the
software product. Generated dependency trees, native outputs, Docker contexts,
and standalone binaries are intentionally absent from Git and are recreated by
the paired runtime scripts. Provider credentials, cloud credentials, TLS
material, host secrets, the existing Nitro chain database, and cloud resources
are also intentionally absent. A completely destroyed environment therefore
requires those external secrets and infrastructure to be provisioned again.
Nitro chain data must be restored or independently resynchronized before the
recovered deployment can be qualified or admitted. The code can seed whatever
canonical head Nitro has when startup reaches the hook; that alone is not
proof that the node has reached the network tip. Follow the paired runtime
`RUNBOOK.md` freshness gate before routing traffic.

The source and operational authorities are:

| Question | Authoritative source |
| --- | --- |
| Which Nitro/Geth/runtime source is permitted? | Runtime `SOURCE.lock` and `scripts/verify-source.sh` |
| How is everything rebuilt from blank hosts? | Runtime [`REBUILD.md`](https://github.com/maurodelazeri/tip-state-robinhood-runtime/blob/main/REBUILD.md), backed by `scripts/build-native.sh`, `scripts/materialize-nitro-product.sh`, and `scripts/test-offline.sh` |
| What does the runtime do and which RPCs exist? | Runtime `README.md` and `DESIGN.md` |
| How is the qualified topology installed, restarted, and recovered? | Runtime [`RUNBOOK.md`](https://github.com/maurodelazeri/tip-state-robinhood-runtime/blob/main/RUNBOOK.md), `DESIGN.md`, and `deploy/remote/` |
| What changes are allowed in this fork? | This file and `AGENTS.md` |

If prose and an executable provenance check disagree, stop. Do not weaken the
check to make a build pass. Reconcile and review the source identities first.

## Repository placement and remotes

Keep the repositories as siblings with these exact directory names:

```text
<workspace>/
├── tip-state-robinhood-nitro/
└── tip-state-robinhood-runtime/
```

The default scripts derive that layout. `NITRO_SOURCE_DIR` can point the
runtime scripts at a different Nitro checkout, but it does not relax any
identity or cleanliness check.

This fork has two remotes with deliberately different authority:

| Remote | URL | Purpose | Push policy |
| --- | --- | --- | --- |
| `origin` | `git@github.com:maurodelazeri/tip-state-robinhood-nitro.git` | Product `main` and immutable `tipstate-upstream-*` provenance tags | Product maintainers only |
| `upstream` | `https://github.com/OffchainLabs/nitro.git` | Official Nitro releases, commits, and comparison refs | Disabled |

Do not push official upstream branches or its complete tag namespace to
`origin`. Do not force-push product `main`, and do not move a published
`tipstate-upstream-*` tag.

## Exact v3.11.2 provenance

These values identify the qualified source product. Git commit/tree identities
and content SHA-256 identities are different namespaces and must not be
interchanged.

| Binding | Exact value |
| --- | --- |
| Official Nitro release | `v3.11.2` |
| Upstream Nitro commit | `3599acae1ad2fab4059fc46453c9cd3294126641` |
| Upstream Nitro tree | `dfe132649a5d53b2595387b3e90e94a9576521e9` |
| Annotated provenance tag | `tipstate-upstream-nitro-v3.11.2` |
| Annotated tag object | `bb5469eb06c45c978a097fbb72a6d15ce05f8f88` |
| Embedded official Geth gitlink | `f3a977ddf30b138da2fe673ac5cbff2bc6dd4c88` |
| Embedded official Geth tree | `212f87a2768d389dbc781d55f2a93829e3095f2b` |
| Exact-block functional commit | `ac09b3c1eac147cb789e341042fc9ac9a31d0e1c` |
| Exact-block functional tree | `2f1fa0081e27702d7c571b903138628d77174086` |
| Exact-block documentation checkpoint | `8c64aac64a2f00a854c279bde681e5ed5e2b3206` |
| Exact-block documentation tree | `5848335991d26afa7961252e5118b36e82259dc6` |
| Runtime-patched Geth content SHA-256 | `01b66a95178ca6d43b6cd7a5d4a20bdb41e3f0ac38a83c6843b2bf80fa7934da` |
| Runtime product closure SHA-256 | `6f5d648b98317b49f2a9e37237a8e227ff8977db29d6966a0fd67c920ae6fe7c` |
| Complete Nitro product context SHA-256 | `95e6e6cd0e289827e1e068e05358cf20680beddb652e0774701776a75500fa04` |
| Compiled chain-wire protocol SHA-256 | `9932978586331bfd40a8d5ef333bcd69663aa9a441c95e1efbe44a0797c7983c` |

The documentation checkpoint is an immutable point in the source lineage, not
a moving alias for whatever commit is currently at `main`. The paired
runtime's `nitro_fork_commit` and `nitro_fork_tree` entries identify the exact
current documentation tip accepted by its build. After a documentation-only
commit, those two lock values must be advanced together while all functional
and product identities remain unchanged.

The provenance tag is annotated, not lightweight. Its object resolves to the
upstream commit shown above. The fork does not change the `go-ethereum`,
`brotli`, or `crates/tools/wasmer` gitlinks from upstream v3.11.2:

| Submodule | Gitlink |
| --- | --- |
| `go-ethereum` | `f3a977ddf30b138da2fe673ac5cbff2bc6dd4c88` |
| `brotli` | `f4153a09f87cbb9c826d8fc12c74642bb2d879ea` |
| `crates/tools/wasmer` | `63c981919d5a5598cdafb197841fa784b5cde955` |

## Functional lineage

The source line is deliberately linear. Documentation commits separate some
numbered functional deltas so their exact parent trees can be replayed and
authenticated independently.

| Commit | Tree | Kind | Purpose |
| --- | --- | --- | --- |
| `a9d134c18e0d02e866b891a6737afd1f9a41cdaa` | `2e1078402cadf67b02247a07cb675efb99d63104` | functional | Couple Geth canonical publication to the scoped synchronous state hook |
| `a7190d4ea660a2150d7a1dc0346448c0eef29e58` | `799a026170fe6a13bc1db25876365f98120f546b` | functional | Add startup-exclusive seeding, same-process runtime lifecycle, and local RAM RPC |
| `0cc27f6026b8e89a9ed8adf554440d8d799fa104` | `0ba7ee8ef24b8462a78d16d452ae85af94dafca8` | functional | Derive the RAM listener's HTTP behavior from Nitro's parsed profile |
| `633781451f10e4a0284089eb310ee5b9f6c22239` | `7aea7bc2a7b7e21f0001349621d72a16e505e90f` | functional | Inject exact client identity and the startup account snapshot |
| `3bf782fba5d984eb3c3b4094bbfdfbea01223ad6` | `87fa45a1ef61666c3cadab4e3f2ab8c2d6ec604c` | documentation | Record original fork provenance |
| `6b039f2cee8730a958aaeb1a0231be741f7d7e51` | `58699df9d6e86ac0092b8b298d1a6a0e208abd5e` | documentation | Establish the remote-cohort patch baseline and repository policy |
| `0c8f425eada133219e4207882d045acaea15a21c` | `350c8c0cd6254e2a9eec2da2dbe24f98a354f526` | functional | Add the mandatory remote three-replica lifecycle and Unix-proxy boundary |
| `f7cc72cde78f521ac2fdd11dd52ade0a5d657034` | `ef3dbc89869d7d35605370d9a2e2e869c4e8d57b` | documentation | Establish the timeout-contract patch baseline |
| `613c19158c836c83aa641190734ddfcbeedc9b88` | `e48e8ce92266e6c03333c2a3c2d8d02d8dc156d0` | functional | Separate the live-operation deadline from the serving lease |
| `65c4b8530bdcee72ff56d49898903530963c10fb` | `947da93b6b7a53949ad07da09deea1965214b70a` | documentation | Establish the exact-block patch baseline |
| `ac09b3c1eac147cb789e341042fc9ac9a31d0e1c` | `2f1fa0081e27702d7c571b903138628d77174086` | functional | Carry the complete canonical block through seed and committed updates |
| `8c64aac64a2f00a854c279bde681e5ed5e2b3206` | `5848335991d26afa7961252e5118b36e82259dc6` | documentation | Record the exact-block functional identity |

`ac09b3c1eac147cb789e341042fc9ac9a31d0e1c` is the source commit checked out
by the current product materializer. Later documentation-only tips authenticate
the repository without changing the materialized Docker source tree.

## Numbered patch chain owned by the runtime

The sibling runtime preserves the functional deltas as byte-exact patches.
`scripts/verify-source.sh` independently replays them and compares the result
with the committed trees above.

| Patch | Applies to | SHA-256 | Result/purpose |
| --- | --- | --- | --- |
| `patches/0001-flat-reader-no-triedb.patch` | Official embedded Geth | `7fea5c33ff78c96af97190ddce12304b2a1cffc7870690e44d71235715395d38` | Flat-state reader and export support without retaining trie DB state in the serving runtime |
| `patches/0002-geth-canonical-flat-state-hook.patch` | Geth after 0001 | `375c2517a9b7855efefd5cacf46f7613d59ea191360878f68939584ad1e509af` | Scoped canonical prepare/commit/reorg hook used by Nitro |
| `patches/0003-nitro-atomic-canonical-state-hook.patch` | Upstream Nitro commit | `9ca274c04790f211c5eca18cfa5cae02eeafbca8f06872f1b649bd793d8095e4` | Atomic Nitro/Geth canonical publication integration |
| `patches/0004-nitro-tipstate-product.patch` | Result of 0003 | `7e5d179e7d308da6cae13271630e947493bee8defb17f799137575197b04b3cd` | Startup seed, embedded runtime, local service, and lifecycle |
| `patches/0005-nitro-tipstate-http-profile.patch` | Result of 0004 | `0b4af2ef983f36098e6586d15dfd127c3503f0150eb252ac820db64921db8413` | Exact HTTP transport-profile propagation |
| `patches/0006-nitro-tipstate-rpc-metadata.patch` | Result of 0005 | `9dc5dad0a5cccda05c87a8c31c257ff041b5fc8080cca9fd6703bf8a6793687d` | Exact client/account RPC metadata propagation; produces commit `633781451...` |
| `patches/0007-nitro-tipstate-remote-cohort.patch` | Commit `6b039f2ce...` | `d2e1c318e197af8ce9b9465e7ff79a07ad2d97deb95f158250ea889ba91ab691` | Mandatory authenticated three-member remote cohort; produces commit `0c8f425ea...` |
| `patches/0008-nitro-tipstate-timeout-contract.patch` | Commit `f7cc72cde...` | `df2676f5729c81ecffd02d41e0108e140a6e51e6d15d7a23d399d26cc4277664` | Lease-independent operation timeout; produces commit `613c19158...` |
| `patches/0009-nitro-exact-canonical-block.patch` | Commit `65c4b8530...` | `3aadfb77db8b370837a29c4034b286364d0610c8c109b5df7f8772c323ff185b` | Complete canonical block in seed and live publications; produces commit `ac09b3c1e...` |

Never regenerate a numbered patch from an unverified worktree or edit a
published patch in place. A functional change requires a new forward patch,
new committed source/tree identities, updated content hashes, and complete
qualification.

## Custom source boundary

Relative to upstream v3.11.2, the product changes only these Nitro areas:

- `go.mod` adds the local `nitro-tipstate-runtime` module replacement;
- `Dockerfile` makes the staged module's `go.mod` and `go.sum` available to the
  dependency-download layers;
- `cmd/nitro/nitro.go` installs transport metadata, process metadata, and the
  fatal channel before execution initialization;
- `cmd/nitro/tipstate_http.go` derives the effective HTTP profile from Nitro's
  already-parsed configuration;
- `execution/gethexec/executionengine.go` supplies startup exclusivity and the
  canonical publication boundary;
- `execution/gethexec/node.go`, `tipstate.go`, and `tipstate_remote.go` own the
  opt-in same-process/remote lifecycle;
- `execution/gethexec/startup_exclusive.go` owns the one-shot startup
  capability;
- `execution/nodeinterface/node_interface.go` admits the narrow pinned-state
  estimator interface without exposing a history-bearing backend; and
- focused tests beside those packages prove the boundary.

The full non-documentation path inventory from upstream to the functional tip
is:

```text
Dockerfile
cmd/nitro/nitro.go
cmd/nitro/tipstate_http.go
cmd/nitro/tipstate_http_test.go
execution/gethexec/canonical_state_hook_test.go
execution/gethexec/executionengine.go
execution/gethexec/node.go
execution/gethexec/startup_exclusive.go
execution/gethexec/startup_exclusive_test.go
execution/gethexec/tipstate.go
execution/gethexec/tipstate_integration_test.go
execution/gethexec/tipstate_remote.go
execution/gethexec/tipstate_remote_test.go
execution/nodeinterface/node_interface.go
execution/nodeinterface/tipstate_inprocess_test.go
go.mod
```

`AGENTS.md` and this document are the only additional fork-root documentation
paths at the exact-block functional tip. The current main branch may contain
later documentation-only recovery changes. No submodule gitlink changes are
part of the custom boundary.

## Why this checkout must not be built standalone

The clean fork is intentionally a source component, not the final Docker
context:

1. `go.mod` contains
   `replace nitro-tipstate-runtime => ./third_party/nitro-tipstate-runtime`,
   but this repository does not commit that generated directory.
2. The committed `go-ethereum` gitlink is the official unmodified embedded
   commit. The production build requires the content-authenticated result of
   runtime patches 0001 and 0002 instead.
3. The Dockerfile copies the staged runtime module in both Go dependency
   layers. A raw `docker build` has no such path and is therefore incomplete.
4. Native Stylus and generated contract prerequisites are build outputs, not
   source files.

Consequently, `go test ./...`, `make build`, or `docker build .` from a fresh
fork checkout is not the product build and may fail on the deliberately absent
module. Do not work around that by committing `third_party`, changing the
replacement to a network module, moving the Geth gitlink to an unpublished
commit, or copying a developer worktree into place. Use the paired runtime's
materializer.

## Clean-room clone and identity verification

The following starts from an empty workspace and uses the repositories'
canonical SSH origins. This Nitro fork is public; the paired runtime repository
is private. Configure a GitHub SSH identity with runtime access before cloning
both through SSH. An authenticated GitHub CLI/HTTPS checkout is equivalent,
but never place a personal access token in a command, remote URL, script, or
repository file.

```bash
set -euo pipefail
mkdir tip-state-robinhood-recovery
cd tip-state-robinhood-recovery
git clone git@github.com:maurodelazeri/tip-state-robinhood-nitro.git
git clone git@github.com:maurodelazeri/tip-state-robinhood-runtime.git
cd tip-state-robinhood-nitro
git remote add upstream https://github.com/OffchainLabs/nitro.git
git remote set-url --push upstream no-push-configured
git fetch origin tag tipstate-upstream-nitro-v3.11.2
git fetch --no-tags upstream 3599acae1ad2fab4059fc46453c9cd3294126641
```

Verify the immutable base and functional lineage before initializing or
building anything:

```bash
set -euo pipefail
test "$(git cat-file -t refs/tags/tipstate-upstream-nitro-v3.11.2)" = tag
test "$(git rev-parse refs/tags/tipstate-upstream-nitro-v3.11.2)" = \
  bb5469eb06c45c978a097fbb72a6d15ce05f8f88
test "$(git rev-parse refs/tags/tipstate-upstream-nitro-v3.11.2^{commit})" = \
  3599acae1ad2fab4059fc46453c9cd3294126641
test "$(git rev-parse refs/tags/tipstate-upstream-nitro-v3.11.2^{tree})" = \
  dfe132649a5d53b2595387b3e90e94a9576521e9
test "$(git rev-parse ac09b3c1eac147cb789e341042fc9ac9a31d0e1c^{tree})" = \
  2f1fa0081e27702d7c571b903138628d77174086
test "$(git ls-tree ac09b3c1eac147cb789e341042fc9ac9a31d0e1c go-ethereum | awk '{print $3}')" = \
  f3a977ddf30b138da2fe673ac5cbff2bc6dd4c88
git merge-base --is-ancestor \
  3599acae1ad2fab4059fc46453c9cd3294126641 \
  ac09b3c1eac147cb789e341042fc9ac9a31d0e1c
```

Then prove that this checkout is the exact main-branch tip locked by its
sibling. This deliberately avoids hard-coding a moving documentation tip in
the instructions:

```bash
set -euo pipefail
runtime_dir=../tip-state-robinhood-runtime
locked_commit=$(awk -F= '$1 == "nitro_fork_commit" { print $2 }' "$runtime_dir/SOURCE.lock")
locked_tree=$(awk -F= '$1 == "nitro_fork_tree" { print $2 }' "$runtime_dir/SOURCE.lock")
test "$(git rev-parse HEAD)" = "$locked_commit"
test "$(git rev-parse HEAD^{tree})" = "$locked_tree"
worktree_status="$(git status --porcelain=v1 --untracked-files=all)"
test -z "$worktree_status"
```

For a minimal authenticated native build, let `scripts/build-native.sh`
initialize the three submodules it needs. For a manual source audit, these are
the equivalent initial operations:

```bash
set -euo pipefail
git submodule sync --recursive
git submodule update --init --depth 1 \
  brotli crates/tools/wasmer go-ethereum
git submodule status brotli crates/tools/wasmer go-ethereum
```

The three status lines must name the gitlinks in the provenance table and must
not begin with `+` or `U`. A leading `-` before initialization only means the
submodule is not populated; it is not a different source identity. Never use
`git submodule update --remote`, because that replaces pinned gitlinks with
moving branch tips.

Initializing every upstream submodule is optional for a manual full-tree
audit:

```bash
set -euo pipefail
git submodule update --init --recursive
```

The product materializer performs its own recursive initialization in a
temporary detached checkout and excludes Geth because it installs the
independently authenticated patched tree there.

## Rebuild and test from the paired sources

Required host capabilities are Bash, Git, Docker with a working daemon and
network access, GNU core utilities, `patch`, `tar`, sufficient disk space, and
permission to run containers. The scripts pin their Rust, Go, and release
container inputs and validate generated native artifacts. Run them from the
runtime repository, not this fork. The paired runtime's `REBUILD.md` is the
complete blank-host procedure; the commands below are its authenticated core:

```bash
set -euo pipefail
cd ../tip-state-robinhood-runtime
bash scripts/build-native.sh
bash scripts/materialize-geth.sh
bash scripts/verify-source.sh
bash scripts/test-offline.sh
```

What those gates establish:

1. `build-native.sh` checks the locked Nitro lineage, initializes exact
   `brotli`, Wasmer, and Geth gitlinks, builds the pinned Stylus library/header,
   extracts exact contract artifacts, stages the runtime module temporarily,
   and validates the generated Go inputs.
2. `materialize-geth.sh` archives official Geth at
   `f3a977ddf30b138da2fe673ac5cbff2bc6dd4c88`, applies patches 0001 and
   0002, and authenticates the resulting content tree as
   `01b66a95178ca6d43b6cd7a5d4a20bdb41e3f0ac38a83c6843b2bf80fa7934da`.
3. `verify-source.sh` checks the annotated base tag, every locked commit/tree,
   every numbered patch digest, replayed functional trees, current clean fork
   tip, Geth materialization, and runtime product closure.
4. `test-offline.sh` reruns source verification, then executes race tests,
   normal tests, and `go vet` with Docker networking disabled.

Materialize the complete Docker source context only after all four commands
pass:

```bash
set -euo pipefail
bash scripts/materialize-nitro-product.sh
test "$(bash scripts/hash-nitro-product.sh .product-work/nitro-product)" = \
  95e6e6cd0e289827e1e068e05358cf20680beddb652e0774701776a75500fa04
```

The materializer does all of the following rather than mutating this checkout:

1. verifies both sibling sources and their hashes;
2. creates a temporary local clone and checks out detached functional commit
   `ac09b3c1eac147cb789e341042fc9ac9a31d0e1c`;
3. initializes every upstream submodule except `go-ethereum` recursively;
4. replaces the Geth directory with `.deps/go-ethereum`, whose content hash is
   already authenticated;
5. stages only the runtime production closure under
   `third_party/nitro-tipstate-runtime`;
6. writes `.nitro-tipstate-product` with the functional commit, seven Nitro
   patch digests, runtime closure digest, patched-Geth digest, and final product
   digest; and
7. hashes every product source path, file mode, and symlink target (excluding
   only Git administration and the provenance marker itself) before atomically
   moving the context into `.product-work/nitro-product`.

Build the producer image from that context:

```bash
set -euo pipefail
TIPSTATE_CANDIDATE_TAG="robinhood-nitro-tipstate:v3.11.2-95e6e6cd-rebuild-$(date -u +%Y%m%dT%H%M%S%NZ)"
docker build --network host --target nitro-node \
  --tag "$TIPSTATE_CANDIDATE_TAG" \
  .product-work/nitro-product
```

The qualified deployment used image
`sha256:7e5ce8980c16cf63eaedbb8d115bf49fca8fe5f9369f864f8d1a76b2a16dd676`
and Nitro executable SHA-256
`e3397dc351d8702528aad039a0ca745b58bcee8fa2f56e9a1fbfe8fbee41a94d`.
Those are deployed-artifact identities, not substitutes for the source gates.
Some upstream Docker stages use package repositories or tagged base images, so
a later rebuild must be treated as a new artifact and requalified if its image
identity differs. The authenticated source-context hash must still match. Do
not assign the published `v3.11.2-95e6e6cd-full` tag until both historic
artifact identities match; the paired runtime's `REBUILD.md` Section 8 contains
the fail-closed inspection and promotion commands. A different repeat-stable
image remains uniquely tagged and must complete the new-candidate release and
qualification path in `RUNBOOK.md`.

`scripts/materialize-nitro-product.sh` refuses to overwrite an existing
destination. On a repeated build, preserve or deliberately remove only the
known generated `.product-work/nitro-product` directory, or set
`NITRO_PRODUCT_DIR` to a new destination. Never delete or overwrite either
source repository to clear a generated build.

## Runtime wiring

The production path is remote mode:

```text
ordinary Nitro configuration and local Geth chain
        │
        ├── effective HTTP profile, client identity, accounts
        └── one startup-exclusive pinned canonical head
                         │
                         ▼
              Nitro tip-state producer hook
                         │ authenticated Unix session
                         ▼
              root-owned seed_fanout_proxy
                    ┌────┴────┐
                    ▼         ▼         ▼
                 cell A     cell B     cell C
              RAM replica  RAM replica  RAM replica
```

The proxy holds one fixed persistent TCP connection to each mandatory member.
There is no replica discovery, optional member, quorum mode, reconnect path,
database-backed serving path, ordinary-RPC fallback, provider fallback, or
historical state lookup.

Startup is ordered as follows:

1. `cmd/nitro` permits tip-state only with a same-process execution node. It
   snapshots Nitro's exact client name, account-manager addresses, effective
   CORS/vhost/prefix/batch/body/timeout profile, and buffered process-fatal
   channel before execution-engine initialization.
2. The execution engine initializes, then issues one startup-exclusive
   capability that owns `createBlocksMutex`. Normal block production cannot
   race the snapshot.
3. Remote mode validates exactly three sorted nonzero member IDs, derives the
   active-set ID, generates fresh producer/seed/request identities, resolves
   chain ID, genesis, and the pinned head, and connects to the root-owned Unix
   proxy. The admitted proxy peer UID is fixed at 0.
4. All three replicas acknowledge bootstrap before the first snapshot record.
   Nitro streams chain configuration, execution policy, RPC/HTTP metadata,
   recent hashes, activated assemblies, complete flat state, and the exact
   canonical block body. Remote mode does not construct a producer-local flat
   image or generation store.
5. All three final seed acknowledgements must agree. Nitro verifies the seed
   manifest's number, hash, root, and complete block against the pinned head,
   creates the live fan-out hook, and installs it at that same expected head.
6. The hook is admitted before the startup-exclusive capability is released.
   Only then can Nitro continue normal startup and block creation.
7. Each committed canonical transition and bounded reorg passes synchronously
   through the scoped hook. The complete new canonical block accompanies its
   flat-state update. Reorg frames publish exactly once after all steps commit.
8. Heartbeats renew the short serving lease only for the exact admitted cohort.
   A seed, live operation, root, sequence, membership, timeout, or publication
   failure is terminal: the hook is poisoned and Nitro reports through the
   process-wide fatal channel.

Same-process mode remains a tested embedding mode. It constructs the complete
RAM generation inside Nitro and starts its own listener after initialization.
It is not the qualified three-cell production topology. In remote mode the
Nitro process hosts no tip-state public RPC listener and retains no serving
state image after the seed.

## Nitro configuration surface

All options are under `--execution.tip-state`. The feature defaults off. The
runtime deployment's `deploy/remote/docker-compose.producer.yml` is the
authoritative qualified argument set; do not infer production values only from
defaults.

| Option | Default | Contract |
| --- | --- | --- |
| `enable` | `false` | Explicit opt-in |
| `mode` | `same-process` | Exactly `same-process` or `remote` |
| `listen` | `127.0.0.1:18546` | Same-process HTTP listener; remote mode does not serve here |
| `gas-cap` | `600000000` | Nonzero `uint64` RPC execution gas ceiling |
| `call-timeout` | `2m` | Same-process: positive and at most 24 hours; remote: `[1ms,24h]` |
| `journal-limit` | `256` | Same-process: positive; remote: `[1,256]` and representable as `uint32` |
| `remote.proxy-socket` | empty | Clean absolute Unix socket path; required in remote mode |
| `remote.proxy-timeout` | `30m` | Seed exchange ceiling; must exceed operation timeout |
| `remote.seed-batch-bytes` | `33554432` | `[1,268435456]` bytes; the upper bound is the 256 MiB wire-frame ceiling |
| `remote.lease-duration` | `2s` | Replica serving lease in `[1ms,60s]` |
| `remote.heartbeat-interval` | `500ms` | `[1ms,lease-duration/3]` |
| `remote.operation-timeout` | `5s` | Per-live-operation bound in `[1ms,60s]`; the qualified deployment explicitly uses `60s` |
| `remote.membership-epoch` | `0` | Must be explicitly set nonzero |
| `remote.member-ids` | empty | Exactly three unique 64-hex identities in increasing byte order |

The production template also fixes the proxy socket, epoch, and exact three
member identities. The proxy and replica services, endpoints, OS limits,
container settings, parent-provider gate, start order, drain order, and reseed
procedure belong to the runtime repository because they coordinate all
processes rather than Nitro alone.

## Validation before deployment

A successful Docker build is necessary but insufficient. Qualification has
five layers, and all must pass for a changed product:

1. source/tag/tree/patch/content authentication (`verify-source.sh`);
2. pinned native prerequisite generation (`build-native.sh`);
3. offline race, normal, vet, protocol, lifecycle, and RPC tests
   (`test-offline.sh`);
4. complete Docker-context hash and image build; and
5. a fresh three-cell seed, root agreement, catch-up to a fresh external
   target, zero backlog, live cadence, exact method/error/header tests,
   byte-level payload comparison against ordinary Nitro, admission testing,
   and bounded RPS/latency measurement.

The exact live commands, method inventory, selector rules, qualification
evidence, deployment identities, and coordinated restart procedure are in the
paired runtime `RUNBOOK.md`, `README.md`, and `DESIGN.md`. `REBUILD.md` covers
artifact reconstruction; `RUNBOOK.md` begins after artifacts exist and owns
host installation, reseed, catch-up, qualification, NLB admission, rollback,
and incident recovery. Do not claim live readiness because the RPC port
accepts TCP, the seed finished, or backlog temporarily reads zero. The replica
lease, fresh-target proof, and external endpoint gates must all pass before
load-balancer admission.

## Safe upstream upgrade

An upstream release upgrade is a new product, not a routine merge into current
`main`. Current scripts intentionally contain v3.11.2 identities and must fail
closed until every new value is reviewed.

1. Fetch upstream without changing product refs:

   ```bash
   set -euo pipefail
   git fetch --tags upstream
   git switch -c upgrade/nitro-vNEXT main
   ```

2. Select the exact official release commit, record its commit and tree, and
   record its embedded Geth commit and tree. Create a new annotated local tag
   named `tipstate-upstream-nitro-vNEXT` at that exact commit. Never retarget
   the existing v3.11.2 tag.
3. In coordinated runtime and Nitro upgrade branches, port each numbered patch
   as the smallest reviewable forward delta. Resolve upstream conflicts by
   understanding the invariant; do not accept an automatic merge merely
   because it compiles.
4. Add new numbered patches rather than rewriting published ones. Update
   `SOURCE.lock`, the hard-coded provenance assertions in the materializers and
   verifier, patch path allowlists, expected native artifacts, content hashes,
   product context hash, and protocol digest when its schema changes.
5. Preserve separate functional and documentation checkpoints so replay can
   prove the exact patch result independently of prose changes.
6. Review the new custom path inventory against upstream. Any changed path
   outside the declared boundary needs an explicit design reason and tests.
7. Run every validation layer above, build a new uniquely tagged image, and
   perform a completely fresh remote seed and live qualification. Never reuse
   the existing product hash, image tag, or deployed-artifact identity.
8. Only after qualification, fast-forward both product `main` branches and
   push the new annotated provenance tag explicitly.

Useful review commands are:

```bash
set -euo pipefail
nitro_new_upstream_commit=REPLACE_WITH_40_HEX_UPSTREAM_COMMIT
nitro_new_functional_commit=REPLACE_WITH_40_HEX_FUNCTIONAL_COMMIT
[[ $nitro_new_upstream_commit =~ ^[0-9a-f]{40}$ ]]
[[ $nitro_new_functional_commit =~ ^[0-9a-f]{40}$ ]]
git merge-base --is-ancestor "$nitro_new_upstream_commit" "$nitro_new_functional_commit"
git diff --stat "$nitro_new_upstream_commit..$nitro_new_functional_commit"
git diff --submodule=short "$nitro_new_upstream_commit..$nitro_new_functional_commit"
git range-diff \
  3599acae1ad2fab4059fc46453c9cd3294126641..ac09b3c1eac147cb789e341042fc9ac9a31d0e1c \
  "$nitro_new_upstream_commit..$nitro_new_functional_commit"
```

## Commit and push without losing lineage

Documentation-only work must not silently change the materialized functional
commit or product hashes. Because the paired runtime locks the clean Nitro
`HEAD`, publish coordinated documentation in this order:

1. Start with clean, current `main` in both siblings and use `git pull
   --ff-only`; never merge an unrelated local branch into the source line.
2. Commit Nitro documentation first. Record its new commit and tree.
3. Update the runtime's current fork commit/tree lock to that exact Nitro
   documentation tip and keep the immutable functional/checkpoint identities
   separate. Update its verifier to admit only the intended documentation
   paths between the locked functional documentation checkpoint and current
   tip.
4. Rerun runtime source verification and offline gates. Product/Geth/runtime
   content hashes must remain unchanged for prose-only changes.
5. Commit the runtime documentation/lock change, then push Nitro `main` before
   runtime `main`, so every pushed runtime lock refers to an already reachable
   Nitro commit.

Final checks and pushes are intentionally explicit:

```bash
set -euo pipefail
# In tip-state-robinhood-nitro
git diff --check
git status --short --branch
git fetch origin main
git merge-base --is-ancestor origin/main main
git push origin main
test "$(git ls-remote origin refs/heads/main | awk '{print $1}')" = \
  "$(git rev-parse main)"

# In tip-state-robinhood-runtime, after its lock and all gates pass
git diff --check
git status --short --branch
git fetch origin main
git merge-base --is-ancestor origin/main main
git push origin main
test "$(git ls-remote origin refs/heads/main | awk '{print $1}')" = \
  "$(git rev-parse main)"
```

For a new upstream base, push only its one product provenance tag after main:

```bash
set -euo pipefail
git push origin refs/tags/tipstate-upstream-nitro-vNEXT
```

Do not use `git push --tags`, `git push --mirror`, or a force option. Confirm
that `upstream` still has a disabled push URL.

## What must never be committed

Keep both repositories reproducible and reviewable. Do not commit:

- `third_party/nitro-tipstate-runtime` in this fork;
- a patched or flattened `go-ethereum` directory or changed unpublished
  gitlink;
- `.deps`, `.cache`, `.product-work`, Docker build contexts, native `target`
  outputs, generated contract outputs, or compiled binaries;
- unreviewed or new live captures, benchmark/qualification reports, live database contents,
  provider URLs containing credentials, cloud credentials, systemd
  credentials, private keys, tokens, or environment files; or
- a locally retagged image identity presented as a qualified artifact.

Use `git status --ignored --short` to inspect generated files and `git clean
-ndX` only as a dry-run inventory. Remove only exact known generated paths
after preserving any evidence that is still required. A clean Git worktree
does not mean external deployment state has been backed up.

The paired runtime's byte-frozen, credential-free qualification corpus under
`qualification/` is the sole reviewed capture exception. Its compressed and
uncompressed identities are tracked explicitly; it is recovery input, not a
license to add later live captures.

## Disaster-recovery completion checklist

A recovered source/build environment is complete only when all of the
following are true:

- both canonical repositories are sibling clean clones on their locked main
  commits;
- `origin` and read-only `upstream` have the intended roles;
- the annotated base tag, base commit/tree, Geth gitlink/tree, functional
  commit/tree, and current documentation tip all verify;
- numbered patch digests and replayed trees match `SOURCE.lock`;
- patched Geth, runtime closure, wire protocol, and complete product-context
  hashes match;
- native prerequisites, race tests, normal tests, and vet pass;
- the producer image is built only from the materialized context;
- external credentials and infrastructure are provisioned without entering
  either repository;
- Nitro chain data is restored or synchronized, A/B/C start empty, the proxy
  and producer follow the documented coordinated order, and the full seed is
  admitted by all three;
- catch-up is proven against a fresh target with zero backlog and advancing
  cadence; and
- all direct and load-balanced method, error, byte-parity, admission, and
  performance gates pass before traffic is enabled.

That checklist is the boundary between “the code was cloned” and “the complete
Robinhood tip-state product was actually recovered.”
