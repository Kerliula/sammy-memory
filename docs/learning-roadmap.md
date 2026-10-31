# Learning roadmap

What to learn to build Sammy, grouped into concept categories and ordered into milestones that follow the [roadmap phases](../README.md#roadmap). Learn each concept just before the phase that needs it, not all up front. A milestone is done when its proof works, not when the reading is finished.

## Concept categories

| # | Category | Key concepts |
|---|---|---|
| **A** | **Go & service engineering** | Static types, structs, implicit interfaces, errors as values, pointers, modules, `context`, goroutines, `errgroup`, `net/http` + huma/OpenAPI, `slog`, env config, table tests |
| **B** | **Postgres & data modeling** | Schema design, transactions and isolation levels, indexes (B-tree, GIN, GiST, HNSW), `EXPLAIN ANALYZE`, goose migrations, pgx + sqlc, recursive CTEs, `tstzrange`, exclusion constraints, row-level security |
| **C** | **Async & reliability** | Job queues, enqueueing in the same transaction (outbox pattern), at-least-once delivery, idempotency, retries and backoff, API rate limits |
| **D** | **Information retrieval** | Embeddings, cosine similarity, approximate nearest neighbor search (ANN/HNSW: recall vs speed, filtered search), `halfvec`, full-text search (`tsvector`, BM25), Reciprocal Rank Fusion (RRF), reranking (bi-encoder vs cross-encoder), MMR diversity, packing results into a token budget |
| **E** | **LLM engineering** | Messages API, tool use for structured output, extraction prompt design, prompt caching, Batches API, cost/latency, agent loops, grounding and citations, the MCP protocol |
| **F** | **Knowledge representation** | Atomic facts, entity resolution, knowledge graphs in relational tables, bitemporal modeling (valid time vs. known time), belief revision and superseding old facts, consolidation, perspectives (theory of mind) |
| **G** | **Evaluation & experimentation** | The LongMemEval abilities, LLM-as-judge, baselines, retrieval metrics (recall@k, MRR, nDCG), noise floor between runs, ablations, cost tracking |
| **H** | **Operations & security** | Docker Compose, CI (golangci-lint, govulncheck), OpenTelemetry tracing, API-key auth, multi-tenancy, deleting data everywhere it was copied to (the forget API), GoReleaser and distroless images |

## Milestones

| M | Phase | Learn | Proof that you've got it |
|---|---|---|---|
| **M0 Map the territory** | before code | G, F (overview) | Read the LongMemEval paper and skim Hindsight and Honcho. Explain in your own words why top-5 vector search fails each of the README's three examples. Sort 10 LongMemEval questions by the ability they test. |
| **M1 Go + Postgres foundations** | 0 | A (core), B (schema, transactions, migrations, pgx/sqlc), H (compose, CI) | `sammy serve` and `sammy migrate` work, plus a `/health` endpoint, one table, and an integration test (testcontainers) passing in CI |
| **M2 Retain & extract** | 1 | C (all of it), E (SDK, tool use, caching, prompt design), F (atomic facts, turning relative dates into real ones), D (embedding basics) | POST messages and get fact rows with embeddings. Kill the worker mid-job: nothing is lost or duplicated. |
| **M3 Measure** | 2 | G (judge, baseline, metrics, variance), E (Batches API) | A baseline score JSON with a per-ability breakdown, and you know how much the score moves between identical runs |
| **M4 Hybrid recall** | 3 | D (everything else), A (`errgroup` for 4 parallel searches), B (`EXPLAIN ANALYZE`, GIN, HNSW tuning) | The score beats the baseline, and an ablation shows what each piece (BM25, RRF, rerank, MMR) contributes |
| **M5 Entities, graph & time** | 4 | F (entity resolution, graph, bitemporal), B (recursive CTEs, `tstzrange`, GiST, exclusion constraints) | Scores go up on temporal reasoning and multi-session questions, and "as of date X" queries work |
| **M6 Consolidation** | 5 | F (belief revision, evidence and confidence), C (scheduled and idempotent rebuilds), E (summarization) | The vegetarian → "I eat fish again" scenario passes, and the knowledge-update score goes up |
| **M7 Reflect, MCP, perspectives** | 6 | E (agent loops, stop conditions, citations, MCP), F (perspectives) | Claude Code uses Sammy over MCP, and reflect cites only fact IDs it actually retrieved |
| **M8 Harden** | 7 | H (auth, tracing, release), B (row-level security with `SET LOCAL`), C (deleting derived data) | Workspace A can't read B's data even with a buggy query, forget removes every derived copy, and a recall trace shows all 4 searches |
| **M9 Tune** | 8 | G (ablation design, judging small differences), D (BM25 extension test), E (model swaps) | Every remaining feature has a measured score delta that's bigger than run-to-run noise |

## How to go about it

- **Hardest categories:** D, F and A are new territory. B, C and H are mostly a step up from Laravel, MySQL and Redis queues.
- **Overlap with coursework:**
  - KNN from LD1 becomes vector search in category D.
  - The distributed-systems course covers what category C is about: delivery guarantees, idempotency and partial failure.
  - Category E is the "LLM application layer" strand in the mentor model.
- **Don't skip M3.** Every phase after it is judged by the eval score, so learning G early is what tells you whether the later concepts actually work.
