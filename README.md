# gocrawler (minimal starter)

A tiny starter crawler in Go (for learning / prototyping).

Quick run:
```bash
git init
# save files
go mod tidy
go run ./cmd/crawler -seed=https://example.com -c=2
```

Notes:
- This scaffold is intentionally small. It is NOT production-ready.
- Next steps: fix Fetcher to return usable body, add URL normalization, robots.txt checks, per-host rate limiters, persistent visited set (Badger), and storage (Postgres or SQLite).
