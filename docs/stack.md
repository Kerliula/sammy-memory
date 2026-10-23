# Stack

The technology choices behind Sammy and why we made them. The guiding rule: **one binary, one database.** Anything that adds a second stateful service has to show it improves the LongMemEval score enough to be worth it.

## Core

| Concern | Choice | Why |
|---|---|---|
| Language | Go (current stable) | One static binary, cheap concurrency for parallel recall, strong Postgres ecosystem. |
| HTTP | `net/http` + [huma v2](https://github.com/danielgtaylor/huma) | Builds the OpenAPI 3.1 spec from Go types, so client SDKs come for free. Runs on the standard mux, so there's little lock-in. |
| MCP | [`modelcontextprotocol/go-sdk`](https://github.com/modelcontextprotocol/go-sdk) | The official SDK. Served over Streamable HTTP on the same server as the REST API. MCP tools map 1:1 to `retain`, `recall`, `reflect` and `profile`. |
| DB access | [pgx v5](https://github.com/jackc/pgx) + [sqlc](https://sqlc.dev) | Retrieval SQL is hand-tuned, so we write it as SQL and get type-safe Go out of it. No ORM. |
| Migrations | [goose](https://github.com/pressly/goose) | Embedded in the binary (`sammy migrate`), and supports Go migrations for backfills. River's own tables go through `rivermigrate`. |
| Job queue | [River](https://riverqueue.com) | A queue built on Postgres. `retain` inserts the messages and enqueues the extraction job in **one transaction**, so no message can slip through unprocessed. |
| Config | [`caarlos0/env`](https://github.com/caarlos0/env) | Environment variables parsed into a struct. |
| Logging | `log/slog` | The standard library is enough. |
| CLI | stdlib subcommands: `serve`, `worker`, `migrate`, `eval` | Switch to cobra only if the CLI grows. |

## Data model (Postgres 17+)

### Vectors

pgvector 0.8+, with an HNSW index on `halfvec` columns, which roughly halves storage compared to `vector`.

Turn on iterative index scans (`hnsw.iterative_scan`). Every recall query is filtered by workspace and peer. Without iterative scans, HNSW can quietly return fewer than `k` results once the filter removes candidates.

### Keyword search

Start with native full-text search: a `tsvector` column, a GIN index and `ts_rank_cd`.

Recall fuses results with Reciprocal Rank Fusion, which uses each result's **rank, not its score**, so true BM25 scoring matters less than it seems. A BM25 extension is a Phase 8 test and stays only if it moves the score:

- [pg_textsearch](https://github.com/timescale/pg_textsearch): BM25 under the PostgreSQL license.
- ParadeDB `pg_search`: capable, but AGPL, which sits badly next to Sammy's Apache-2.0 license.

### Entity graph

Plain tables (`entities`, `fact_entities`, `fact_links`) queried with recursive CTEs. Recall only expands 1–2 hops, which doesn't need a graph database or Apache AGE.

### Bitemporal facts

Each fact carries two `tstzrange` columns:

- `valid_during`: when the fact was true in the world.
- `known_during`: when Sammy believed it.

Both columns get GiST indexes (`btree_gist`). An exclusion constraint stops a belief from having two overlapping "current" versions, so "superseded, never silently overwritten" is enforced by the schema, not just by application code.

### Multi-tenancy

Every table has a `workspace_id` from Phase 1. Phase 7 enables row-level security keyed on `SET LOCAL app.workspace_id`, with no schema changes needed.

## Models

Embedder, reranker and LLM each sit behind a small Go interface so Phase 8 tests can swap them.

| Job | Choice | Notes |
|---|---|---|
| SDK | [`anthropic-sdk-go`](https://github.com/anthropics/anthropic-sdk-go) | Official SDK. |
| Fact extraction | Claude Haiku 5.5 | High volume. Tool use for structured output, prompt caching on the extraction instructions. |
| Consolidation & reflect | Claude Sonnet 5.5 | Try Opus 5.5 only if the eval shows reasoning is the bottleneck. |
| Backfills & eval runs | Message Batches API | Cheaper, and fine for offline work. |
| Embeddings | [Voyage AI](https://www.voyageai.com) | Anthropic doesn't offer embeddings; Voyage is the partner it recommends. |
| Reranking | Voyage rerank | Optional local setup: [text-embeddings-inference](https://github.com/huggingface/text-embeddings-inference) serving `bge-reranker-v2-m3` as a compose service. |
| Token packing | Local estimate + safety margin | Too slow to call the API on every recall. The eval suite checks the estimate against Anthropic's `count_tokens`. |

## Quality & operations

| Concern | Choice | Why |
|---|---|---|
| Tests | [testcontainers-go](https://golang.testcontainers.org) with the `pgvector/pgvector` image | Every test runs against real Postgres, with no mocks. Each test rolls back its transaction so the suite stays fast. |
| Eval | `cmd/eval` | Loads a LongMemEval subset, runs retain → recall, grades answers with an LLM judge and saves results as JSON so runs can be compared over time. Runs in CI on a label, because it costs money. |
| Observability | OpenTelemetry, [`otelpgx`](https://github.com/exaring/otelpgx), River's OTel middleware | Traces each recall across its 4 parallel searches, so the ~200–500 ms budget can be checked. |
| Lint & security | `golangci-lint` v2, `govulncheck`, `sqlc diff` | Run on every PR. |
| Release | [GoReleaser](https://goreleaser.com) → distroless image | One binary, one small image. |

## Deliberately not used

- **LangChainGo or other agent frameworks.** The reflect loop is the product; we own it.
- **A separate vector DB (Qdrant, Weaviate), Neo4j, Redis, Kafka.** Each one breaks "one database", and Postgres covers all of it at this scale.
- **GORM or ent.** sqlc is a better fit for hand-tuned retrieval SQL.

## Open questions

- **Keyword search:** native `tsvector` or a BM25 extension? Decided by Phase 8 tests.
- **Local-only mode:** should `docker compose up` work with no API keys, using local embedding and reranking models? This costs some quality and adds a model service to compose.
