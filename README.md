# gocrawler — minimal starter (ongoing)

A tiny, minimal web crawler written in Go for learning and prototyping. This repository is intentionally small and educational — it's an ongoing project and will gain features incrementally.

Status
- Active/Work-in-progress: new features and improvements will be added step-by-step (see Roadmap).
- Not production-ready: this is a learning scaffold.

Quick start

1. Initialize the repo (if needed), download modules, and run the crawler:

```bash
# from repository root
go mod tidy
go run ./cmd/crawler -seed=https://example.com -c=2
```

2. Example flags
- -seed: starting URL (required for simple run)
- -c: concurrency / number of worker goroutines

(See `cmd/crawler/main.go` for exact flags supported in your version.)

What this project contains
- A small, modular crawler implemented with these internal packages:
  - `fetcher/` — performs HTTP fetches (simple prototype)
  - `parser/` — extracts links from responses
  - `frontier/` — manages URL scheduling
  - `visited/` — in-memory visited set (prototype)
  - `storage/` — pluggable storage for crawled data
  - `cmd/crawler` — small CLI entrypoint

Goals and design principles
- Keep components small and composable so you can replace pieces (e.g., storage or visited set) without rewriting the whole crawler.
- Focus on correctness and clarity over performance for this learning repo.

Planned roadmap (next steps)
These are small, safe, incremental improvements that will be added over time:

1. Improve Fetcher
   - Return a usable response body (streaming / []byte) and expose content-type and status.
   - Add request timeouts and retry/backoff.

2. URL handling
   - Normalize and canonicalize URLs.
   - Resolve relative links using base href.

3. Politeness and robots
   - Add robots.txt checks per host.
   - Implement per-host rate limits and crawl delays.

4. Persistence
   - Replace in-memory `visited` with persistent store (BadgerDB or SQLite).
   - Add a pluggable `storage` implementation (Postgres/SQLite) for extracted data.

5. Tests and examples
   - Add unit tests for core packages and a small integration test.
   - Provide example configs and usage.

How you can help / contribute
- Open issues for bugs or feature requests.
- Send PRs for focused improvements (e.g., better fetcher, URL normalization, or tests).
- If you're experimenting, add a small example under `tests/` showing the behavior you expect.

Developer tips
- To run the CLI from the repo root:

```bash
go run ./cmd/crawler -seed=https://example.com -c=2
```

- Build a binary:

```bash
go build -o bin/crawler ./cmd/crawler
./bin/crawler -seed=https://example.com -c=2
```

License
- Intentionally left unspecified. Add a license file if you plan to publish or collaborate.

Contact / Notes
- This is a learning project. Expect frequent small changes as features are added.
- If you want a guided roadmap for a specific feature, open an issue and label it `roadmap`.
