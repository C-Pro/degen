package main

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand" // nosemgrep: deterministic day sampling, not security-sensitive
	"os"
	"path/filepath"
	"time"

	"degen/pkg/bench"
	"degen/pkg/connectors/pintupro"
)

const (
	cacheDir = "cmd/strategybench/.cache"
	// addressableDays is the FIXED span (from windowStart) that seeds are hashed
	// over. It is generous so the usable window keeps growing for years without
	// the per-seed mapping ever shifting. Days past "today" are rejected, so the
	// effective window is [windowStart, yesterday] and widens 1:1 with calendar
	// time (e.g. ~730 days now, ~830 in 100 days).
	addressableDays = 3650 // ~10 years
	dayFetchTries   = 100  // re-sample when a day is in the future or has no data
)

// windowStart is the FIXED absolute lower bound of the sampling window (UTC). It
// never moves, so cached days never fall out of the window and a given seed
// always maps to the same absolute day — caches are reused forever, never
// invalidated. (~730 days before 2026-06-14.)
var windowStart = time.Date(2024, 6, 14, 0, 0, 0, 0, time.UTC)

// randomDayCandleSource maps each seed to a stable absolute UTC day in
// [windowStart, yesterday], downloads that day's candles (cached on disk), and
// re-samples a different day when the sampled one is still in the future or has
// no data (e.g. the symbol was listed more recently). Deterministic per seed.
func randomDayCandleSource(api *pintupro.API, symbol, interval string) bench.CandleSource {
	todayStart := time.Now().UTC().Truncate(24 * time.Hour)
	return func(seed int64) ([]bench.Candle, error) {
		for attempt := 0; attempt < dayFetchTries; attempt++ {
			day, valid := selectDay(seed, attempt, todayStart)
			if !valid {
				continue // future / incomplete current day: re-sample (no fetch)
			}
			candles, err := loadOrFetchDay(api, symbol, interval, day)
			if err != nil {
				return nil, err
			}
			if len(candles) > 0 {
				return candles, nil
			}
			// Empty day (no listing / no data yet): try a different day.
		}
		return nil, fmt.Errorf("no candle data for %s across %d sampled days (seed %d); symbol may be too new",
			symbol, dayFetchTries, seed)
	}
}

// selectDay deterministically maps (seed, attempt) to a UTC-midnight day in the
// fixed addressable span [windowStart, windowStart+addressableDays). It reports
// valid=false when the day is not strictly before today (a future or the
// incomplete current day), so the caller re-samples. The mapping does not depend
// on the current date, so it is stable as the usable window grows.
func selectDay(seed int64, attempt int, today time.Time) (time.Time, bool) {
	rng := rand.New(rand.NewSource(seed*1000003 + int64(attempt)*2654435761))
	day := windowStart.AddDate(0, 0, rng.Intn(addressableDays))
	return day, day.Before(today)
}

// loadOrFetchDay returns one day's candles from the on-disk cache, fetching and
// caching them (including a genuinely-empty result, to avoid re-requesting a
// known-empty day) on a miss.
func loadOrFetchDay(api *pintupro.API, symbol, interval string, day time.Time) ([]bench.Candle, error) {
	path := cachePath(symbol, interval, day)
	if c, ok, err := readCache(path); err != nil {
		return nil, err
	} else if ok {
		return c, nil
	}

	from := day.Unix()
	to := from + 24*3600
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	cs, err := api.GetCandlesticks(ctx, symbol, interval, from, to)
	if err != nil {
		// A transient/network error must not be cached as "empty".
		return nil, fmt.Errorf("fetch %s %s: %w", symbol, day.Format("2006-01-02"), err)
	}

	candles := make([]bench.Candle, 0, len(cs))
	for _, c := range cs {
		candles = append(candles, bench.Candle{
			Open:  c.Open.InexactFloat64(),
			High:  c.High.InexactFloat64(),
			Low:   c.Low.InexactFloat64(),
			Close: c.Close.InexactFloat64(),
		})
	}
	if err := writeCache(path, candles); err != nil {
		return nil, err
	}
	return candles, nil
}

// cachePath e.g. cmd/strategybench/.cache/ohlc-WLD-IDR-15m-2026-06-06.yml
// (content is JSON, which is valid YAML).
func cachePath(symbol, interval string, day time.Time) string {
	name := fmt.Sprintf("ohlc-%s-%s-%s.yml", symbol, interval, day.Format("2006-01-02"))
	return filepath.Join(cacheDir, name)
}

func readCache(path string) ([]bench.Candle, bool, error) {
	b, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	var c []bench.Candle
	if err := json.Unmarshal(b, &c); err != nil {
		return nil, false, fmt.Errorf("corrupt cache %s: %w", path, err)
	}
	return c, true, nil
}

func writeCache(path string, candles []bench.Candle) error {
	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		return err
	}
	b, err := json.Marshal(candles)
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path) // atomic publish
}
