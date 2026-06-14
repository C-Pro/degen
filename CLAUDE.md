# CLAUDE.md

Guidance for working in this repository.

## What this is

`degen` is a crypto **market-making trading bot** in Go (module `degen`, Go 1.25,
vendored deps). It started as an educational live-coding demo and has grown a
real **backtesting + parameter auto-tuning harness**. The production strategy is
a "ladder" market maker; the primary live venue is **Pintu Pro** (an Indonesian
exchange on a crypto.com-style API; pairs are quoted in IDR, e.g. `BTC-IDR`).

Honest status of the strategy: on the modeled fees (Pintu: 0.12% maker + 0.21%
PPh sell tax = 0.45% round-trip) the ladder does **not** beat buy-and-hold on the
IDR pairs studied — market-making can't extract edge from a near-driftless daily
process when the round-trip cost is that high. See `autoresearch/` write-ups and
the memory notes. Treat PnL claims skeptically and validate on held-out data.

## Repository layout

```
/main.go                  Live trading bot (the `degen` binary)
/cmd/strategybench/        Backtesting CLI (random-walk & candle-replay)
/cmd/dumper/               Market-data feature dumper (Binance -> CSV) for analytics/ML
/pkg/account/             Account: wraps a connector, tracks balances/positions/orders/PnL
/pkg/strategies/          Strategies: ladder (production MM), monkey (toy MM)
/pkg/connectors/          Exchange connectors (implement account's `exchange` interface)
   pintupro/                live venue (REST + websocket), candles, tickers
   binance/                 market-data only (used by dumper)
   dummy/                   in-memory simulated exchange (used by the bench)
   ws.go                    shared websocket helper
/pkg/bench/               Monte-Carlo backtesting harness + grid-search auto-tuner
/pkg/models/              Shared types: Order, BBO, Position, Balance, SymbolInfo, ExchangeMessage
/pkg/metrics/             Prometheus gauges/histograms (served at :8080/metrics live)
/pkg/accum/               Time-interval accumulators for feature stats (no thread-safety)
/pkg/csvwriter/           Rotating CSV writer (used by dumper)
/autoresearch/            Throwaway scripts, grids, plots, findings (gitignored)
```

## Core architecture

Data flows **connector → account → strategy**:

- **Connector** implements the unexported `exchange` interface in
  `pkg/account/account.go` (`PlaceOrder`, `CancelOrder`, `GetOpenOrders`,
  `Listen`, `Subscribe*`, …). This is the extension point for new venues.
- **`account.Account`** wraps a connector, applies inbound `ExchangeMessage`s to
  authoritative state (balances; positions as a sorted price-level structure
  with realized PnL in `position.go`; open orders & per-side open interest), and
  forwards messages to the strategy via the `Updates()` channel (best-effort,
  latest-view — slow consumers drop). Strategies must read state through the
  `Get*` accessors, not the channel.
- **Strategy** is any type with `See(models.ExchangeMessage)`. There is no formal
  interface in `pkg/strategies` yet (the bench defines a minimal one). Live wiring
  recovers per-message so one panic doesn't kill the loop. Implementations:
  `Ladder` (multi-level reduce-only MM) and `Monkey` (single-level toy).

**The bench (`pkg/bench`)** backtests a strategy against the `dummy` connector:
- Price models (both satisfy `Generate(*rand.Rand) []models.BBO`):
  - `PriceModel` — driftless Gaussian random walk calibrated to a 24h OHLC swing
    (Parkinson). Pure synthetic.
  - `CandleWalk` — replays real OHLC candles; each bar opens at O, closes at C,
    and touches H/L exactly. Carries real trend + intra-bar reversion.
- `matcher` — maker-fill engine: fills resting orders the price crosses, applies
  position/balance updates and the Pintu fee model (maker both sides + sell tax),
  skips fills the account can't cover.
- `Run` / `RunOne` — seeded, deterministic Monte-Carlo over N seeds.
- `GridSearch` / `DefaultTuneGrid` — sweep (levels, allocation, spread, tolerance)
  and return the best params by backtest PnL. Used by the live bot's auto-tuner.
- `CandleSource` — supply a different historical day per seed (regime diversity).

## Live bot flow (`/main.go`)

1. Load env (see below) and fail fast if required vars are missing.
2. Create the pintupro connector + `Account`; read balance and `SymbolInfo`.
3. **Auto-tune**: download the last 7 days of 15m candles and `bench.GridSearch`
   level-spread/allocation/tolerance against them (`tuneLadder`). The detected
   allocation is applied to the deployable budget and capped per-order.
4. Build the `LadderConfig`, start the account, subscribe, dispatch updates to
   `ladder.See`, serve Prometheus at `:8080`, run until SIGINT/SIGTERM, then
   cancel-all and stop.

### Env vars (live bot)
Mandatory operator config: `SYMBOL`, `PINTUPRO_API_BASE_URL`, `PINTUPRO_WS_URL`,
`PINTUPRO_KEY`, `PINTUPRO_SECRET`. Strategy/risk config (defaults in `loadConfig`):
`MAKER_FEE` (0.0012), `SELL_TAX` (0.0021), `ORDER_NOTIONAL` (alias `NOTIONAL`),
`MAX_ORDER_NOTIONAL`, `MAX_NOTIONAL_ALLOCATION`. Spread/allocation/tolerance are
**auto-detected**, not configured.

**.env gotcha:** `.env` here is docker-compose format (no `export`). Plain
`. .env` sets shell vars the child process won't inherit (symptom: cryptic
`malformed ws or wss URL`). Run with:
```
set -a && . ./.env && set +a && ./degen
```

## Build / test / run

```
go build ./...                 # build everything
go test ./...                  # all tests (bench tests ~15s; pintupro live tests skip without creds)
make check                     # docker-based lint + test -race + semgrep + osv-scanner (CI parity)
go build -o degen . && set -a && . ./.env && set +a && ./degen   # run live bot

# Backtesting CLI:
go run ./cmd/strategybench -symbol BTC-IDR -strategy ladder -price-model candles
go run ./cmd/strategybench -symbol DOGE-IDR -runs 100 -price-model candles -random-days
```

Match the surrounding code style; keep `go test ./...` green. The bench tests
discard logs via `TestMain`.

## Conventions & gotchas

- **Money is `shopspring/decimal`.** Floats are used only where precision doesn't
  matter (intra-candle texture, position price levels) — keep that boundary.
- **Determinism:** `bench.Run` seeds both the price RNG and the process-global
  `uuid` rand (strategies mint client IDs via `uuid.NewString`; order maps are
  ID-ordered). Because it mutates global uuid state, **`bench.Run`/`GridSearch`
  are not concurrency-safe** — keep grid sweeps sequential.
- **Pintu fee model:** 0.12% maker (both sides) + 0.21% PPh withholding on
  **sells only** (`Config.SellTaxRate`, applied in `matcher.go`). The sell tax is
  large and biases everything toward buy/hold.
- **Logging is verbose** (strategies/account log every order); mute with
  `log.SetOutput(io.Discard)` around bench sweeps (the live tuner already does).
- **Pintu candle endpoint:** `GET /v1/public/get-candlesticks?symbol=X&interval=15m&from=<sec>&to=<sec>`
  (param is `symbol`, times in **seconds**; ~100 bars without from/to, full window
  with). `(*pintupro.API).GetCandlesticks` returns oldest-first.
- **`autoresearch/`, the `degen`/`strategybench` binaries, and
  `cmd/strategybench/.cache/` are gitignored.** Don't commit them.
- Tune-on-7-days fits the **recent** regime; small-sample param search overfits —
  validate on held-out days (`-seed` shifts the window). See `autoresearch/multitoken-findings.md`.

## Intended direction (roadmap)

Planned work and where it slots in — design new code to fit these seams:

- **Multiple strategies/tokens in one process.** Today `/main.go` runs a single
  (symbol, strategy). Introduce a supervisor over N `(Account, Strategy)` units;
  promote the `Strategy` `See` contract and the account's `exchange` interface to
  exported package interfaces (e.g. a `pkg/exchange`), and run one dispatch loop
  per unit. `Account` already isolates per-connector state.
- **Persistence layer.** No DB today (state is in-memory; observability via
  Prometheus; dumper writes CSV). Add a `pkg/store` for orders/fills/positions/
  realized PnL/run config, so restarts and the future UI have history.
- **API + web UI for monitoring/control.** Only `/metrics` exists now. Add a
  `pkg/api` (HTTP/gRPC) control plane: inspect/override strategy params, start/stop
  units, view PnL — backed by the persistence layer.
- **New exchange connectors.** Implement the `exchange` interface (`pkg/account`)
  — that is the whole contract. `binance` is currently market-data only; `pintupro`
  is the full reference. Consider an exported interface package as venues multiply.
- **Blockchain operations** (rebalancing, contract interaction). New `pkg/chain`
  package; treat on-chain transfers/swaps as another "venue"/treasury action the
  account or a rebalancer can invoke.
- **More analytics + predictive models.** `cmd/dumper` + `pkg/accum` +
  `pkg/csvwriter` already emit interval feature vectors from Binance market data;
  `pkg/bench` provides backtesting. A predictive signal would feed strategy
  quoting (e.g. a reservation-price skew) — the bench is the place to validate it
  before going live.
