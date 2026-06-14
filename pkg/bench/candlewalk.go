package bench

import (
	"math"
	"math/rand" // nosemgrep: deterministic Monte-Carlo seeding, not security-sensitive
	"time"

	"degen/pkg/models"

	"github.com/shopspring/decimal"
)

const (
	defaultTicksPerCandle = 30
	minTicksPerCandle     = 4 // need 4 distinct anchors: open, two extremes, close
)

// Candle is one OHLC bar replayed by the candle-following price model.
type Candle struct {
	Open  float64
	High  float64
	Low   float64
	Close float64
}

// CandleWalk replays a sequence of OHLC candles as a tick path. Within each
// candle the path starts at Open, ends at Close, and touches the candle's High
// and Low exactly, with a Brownian-bridge texture in between. Unlike the
// single-ticker PriceModel (a free, driftless random walk calibrated only to a
// swing amplitude), this carries the real trend and intra-bar reversion of the
// downloaded market history, so every seed reproduces the same per-candle OHLC.
type CandleWalk struct {
	candles        []Candle
	halfSpread     float64
	ticksPerCandle int
	levelSize      decimal.Decimal
}

// NewCandleWalk builds a candle-following generator from a config.
func NewCandleWalk(cfg Config) CandleWalk {
	tpc := cfg.TicksPerCandle
	if tpc < minTicksPerCandle {
		tpc = minTicksPerCandle
	}
	size := cfg.BBOLevelSize
	if size <= 0 {
		size = defaultLevelSize
	}
	return CandleWalk{
		candles:        cfg.Candles,
		halfSpread:     cfg.Spread / 2,
		ticksPerCandle: tpc,
		levelSize:      decimal.NewFromFloat(size),
	}
}

// Generate produces the full tick path for the configured candles, applying a
// symmetric relative spread around each midprice.
func (m CandleWalk) Generate(rng *rand.Rand) []models.BBO {
	bbos := make([]models.BBO, 0, len(m.candles)*m.ticksPerCandle)
	base := time.Now().UTC()
	t := 0
	for _, c := range m.candles {
		for _, mid := range candleMids(c, m.ticksPerCandle, rng) {
			bid := mid * (1 - m.halfSpread)
			ask := mid * (1 + m.halfSpread)
			bbos = append(bbos, models.BBO{
				Bid:       models.PriceLevel{Price: decimal.NewFromFloat(bid), Size: m.levelSize},
				Ask:       models.PriceLevel{Price: decimal.NewFromFloat(ask), Size: m.levelSize},
				Timestamp: base.Add(time.Duration(t) * time.Second),
			})
			t++
		}
	}
	return bbos
}

// candleMids returns k midprices for one candle: out[0] == Open, out[k-1] ==
// Close, with the High and Low touched at two random interior ticks (in random
// order) and a clamped Brownian-bridge path between the anchors. The clamp to
// [Low, High] together with the pinned extreme anchors guarantees the path's
// min == Low and max == High exactly.
func candleMids(c Candle, k int, rng *rand.Rand) []float64 {
	out := make([]float64, k)
	if c.High <= c.Low {
		// Degenerate bar (no range): flat at the open.
		for i := range out {
			out[i] = c.Open
		}
		return out
	}

	// Two distinct interior anchor indices for the extremes.
	i1 := 1 + rng.Intn(k-2)
	i2 := 1 + rng.Intn(k-2)
	for i2 == i1 {
		i2 = 1 + rng.Intn(k-2)
	}
	if i1 > i2 {
		i1, i2 = i2, i1
	}

	// Randomise which extreme is reached first.
	e1, e2 := c.Low, c.High
	if rng.Float64() < 0.5 {
		e1, e2 = c.High, c.Low
	}

	idx := []int{0, i1, i2, k - 1}
	val := []float64{c.Open, e1, e2, c.Close}

	sigma := (c.High - c.Low) * 0.5 / math.Sqrt(float64(k))
	for s := 0; s < len(idx)-1; s++ {
		bridgeFill(out, idx[s], idx[s+1], val[s], val[s+1], sigma, rng)
	}

	for i := range out {
		if out[i] < c.Low {
			out[i] = c.Low
		}
		if out[i] > c.High {
			out[i] = c.High
		}
	}
	// Pin the anchors exactly (clamp above only enforces the band).
	out[0] = c.Open
	out[i1] = e1
	out[i2] = e2
	out[k-1] = c.Close
	return out
}

// bridgeFill writes a discrete Brownian bridge into out[ka..kb] (inclusive),
// pinned at value a (index ka) and b (index kb), with per-step noise sigma.
func bridgeFill(out []float64, ka, kb int, a, b, sigma float64, rng *rand.Rand) {
	m := kb - ka
	if m <= 0 {
		out[ka] = a
		return
	}
	raw := make([]float64, m+1)
	for j := 1; j <= m; j++ {
		raw[j] = raw[j-1] + rng.NormFloat64()
	}
	for j := 0; j <= m; j++ {
		frac := float64(j) / float64(m)
		out[ka+j] = a + (b-a)*frac + sigma*(raw[j]-raw[m]*frac)
	}
}

// aggregateTicker collapses a candle history into a single OHLC summary (first
// open, last close, max high, min low) for reporting the overall swing.
func aggregateTicker(candles []Candle) Ticker {
	if len(candles) == 0 {
		return Ticker{}
	}
	t := Ticker{
		Open:  candles[0].Open,
		Close: candles[len(candles)-1].Close,
		High:  candles[0].High,
		Low:   candles[0].Low,
	}
	for _, c := range candles {
		if c.High > t.High {
			t.High = c.High
		}
		if c.Low < t.Low {
			t.Low = c.Low
		}
	}
	return t
}
