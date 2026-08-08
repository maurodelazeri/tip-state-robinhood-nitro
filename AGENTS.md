# Tip-State Nitro Constraints

- Canonical fork: `https://github.com/maurodelazeri/tip-state-robinhood-nitro`.
  Keep official Nitro refs on the read-only `upstream` remote; `origin` is for
  product `main` and immutable `tipstate-upstream-*` provenance tags only.
- Read `TIP_STATE_FORK.md` before changing this fork. Preserve the annotated
  upstream base tag and exact functional lineage.
- The intended product branch is `main`. Keep upstream Nitro source unchanged
  outside the declared custom boundary unless an explicit requirement needs a
  narrow modification.
- This repository is one half of the sibling runtime/Nitro pair. Do not commit
  generated `third_party`, patched Geth worktrees, native outputs, Docker
  contexts, compiler caches, credentials, or qualification reports.
- Never add a serving fallback to Nitro's database, ordinary RPC, a provider,
  disk, history, or a partial cache. Missing or unsupported state must fail
  closed.
- Preserve startup root verification, scoped canonical publication, atomic
  generation swaps, bounded reorg handling, and fatal poisoning on divergence.
- Keep the public method inventory synchronized with the paired runtime. Do not
  register a partial method or broaden historical/pending behavior.
- Run the paired runtime's provenance, normal, race, vet, full image, and live
  byte-comparison gates before claiming a production change.
- Do not reformat unrelated upstream files or mutate live chain data,
  containers, endpoints, credentials, or restart policy during source work.
