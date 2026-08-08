# Robinhood Tip-State Nitro Fork

This repository is the Nitro producer and embedding-client fork for the
Robinhood current-tip RAM runtime. Nitro owns canonical execution, emits the
complete startup snapshot and ordered committed updates, and hosts the local
RPC listener. Serving requests execute only against the paired runtime's
immutable RAM generation; they never fall back to Nitro's database or ordinary
RPC endpoint.

## Upstream provenance

- Canonical fork: `https://github.com/maurodelazeri/tip-state-robinhood-nitro`
- Canonical paired runtime: `https://github.com/maurodelazeri/tip-state-robinhood-runtime`
- Official upstream: `https://github.com/OffchainLabs/nitro.git`
- Upstream release: `v3.11.2`
- Fork-point commit: `3599acae1ad2fab4059fc46453c9cd3294126641`
- Fork-point tree: `dfe132649a5d53b2595387b3e90e94a9576521e9`
- Annotated base tag: `tipstate-upstream-nitro-v3.11.2`
- Embedded Geth commit: `f3a977ddf30b138da2fe673ac5cbff2bc6dd4c88`
- Embedded Geth tree: `212f87a2768d389dbc781d55f2a93829e3095f2b`
- Paired runtime repository: sibling `../tip-state-robinhood-runtime`

The exact fork point and embedded Geth identity are explicit. Never infer them
from a version string, image tag, or moving upstream branch.

## Functional lineage

1. `a9d134c18e0d02e866b891a6737afd1f9a41cdaa` couples Geth canonical
   publication to a synchronous scoped state-update hook.
2. `a7190d4ea660a2150d7a1dc0346448c0eef29e58` adds the startup-exclusive
   seed, in-process runtime lifecycle, fail-closed publication, and local RPC.
3. `0cc27f6026b8e89a9ed8adf554440d8d799fa104` copies Nitro's parsed HTTP
   profile so the local listener preserves transport behavior and configured
   limits.
4. `633781451f10e4a0284089eb310ee5b9f6c22239` injects the exact Nitro
   client identity and startup account snapshot for the final RPC surface.

The functional tip-state commit is
`633781451f10e4a0284089eb310ee5b9f6c22239`; its tree is
`7aea7bc2a7b7e21f0001349621d72a16e505e90f`.

## Two-repository build boundary

This fork intentionally preserves Nitro's upstream submodule model. It does
not commit a generated `third_party` runtime, a locally patched Geth worktree,
native outputs, compiler caches, or a Docker product marker. Consequently the
fork alone is not the final Docker context.

The paired runtime repository owns the numbered patch provenance and the
deterministic materializer. That materializer checks out the functional commit
from this clean fork, replaces the Geth gitlink with the content-hashed
0001+0002 materialization, stages the content-hashed runtime module under
`third_party`, authenticates the complete Docker context, and only then builds.
This is the smallest reproducible two-repository model. Avoid unpublished Geth
gitlink commits, flattened submodules, or hand-maintained vendored copies.

## Custom source boundary

The Nitro-root changes are limited to:

- canonical-hook and startup-exclusivity integration in `execution/gethexec`;
- same-process lifecycle and configuration in `execution/gethexec` and
  `cmd/nitro`;
- the exact NodeInterface compatibility boundary in
  `execution/nodeinterface`;
- the local module replacement and Docker staging lines; and
- focused product tests beside those packages.

The paired runtime's Geth patches own the flat reader, exact call/trace/estimate
bridges, and canonical flat-state export. Keep upstream source outside these
boundaries unchanged unless an explicit requirement demands a narrow change.

## Upgrade rule

For every Nitro upgrade:

1. record the official release, exact base commit/tree, embedded Geth
   commit/tree, and annotated base tag;
2. reapply the smallest numbered patches and record the functional commit/tree;
3. update the paired runtime's complete provenance binding;
4. run locked normal, race, vet, product-build, and image gates;
5. perform a fresh authenticated seed and byte-level current-tip comparison
   before deployment promotion.

Never move a qualified base tag or bypass the paired materializer.

Only `main` and immutable `tipstate-upstream-*` provenance tags belong on the
fork's `origin`. Keep official branches and tags on the read-only `upstream`
remote; never mirror them into the product namespace.
