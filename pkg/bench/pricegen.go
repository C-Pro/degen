package bench

import (
	"math"
	"math/rand" // nosemgrep: deterministic Monte-Carlo seeding, not security-sensitive
	"time"

	"degen/pkg/models"

	"github.com/shopspring/decimal"
)

// Expected-range coefficients for a driftless Gaussian random walk.
//
// The harness calibrates a symbol's swing amplitude from its 24h high-low
// range, in the spirit of the Parkinson range volatility estimator. For
// Brownian motion observed continuously over a window, the expected high-low
// range equals sigma_terminal * sqrt(8/pi). A discrete N-step walk only samples
// that motion and therefore misses the extrema reached between steps, so its
// expected range is smaller by a constant proportional to the per-step sigma
// (the Asmussen-Glynn-Pitman correction for the maximum of a Gaussian random
// walk: E[max] = sigma*sqrt(2N/pi) + sigma*zeta(1/2)/sqrt(2*pi), with
// zeta(1/2) < 0). We invert that relation in stepSigma to choose the per-step
// sigma that reproduces a target range in expectation across seeds.
const (
	// rangeCoef = sqrt(8/pi): continuous-BM expected range per unit terminal sigma.
	rangeCoef = 1.5957691216057308
	// discreteCorr = 2 * (-zeta(1/2)) / sqrt(2*pi): the per-step-sigma deficit the
	// discrete sampling introduces on the range (max minus min, hence the factor 2).
	discreteCorr = 1.1651680395203477
)

// stepSigma returns the per-step log-return standard deviation such that a
// driftless Gaussian random walk of n steps has an expected high-low log-range
// equal to targetLogRange (matched in expectation, not per-path). Returns 0 for
// a degenerate target (n < 2 or non-positive range), which yields a flat price.
func stepSigma(targetLogRange float64, n int) float64 {
	if n < 2 || targetLogRange <= 0 {
		return 0
	}

	denom := math.Sqrt(float64(n))*rangeCoef - discreteCorr
	if denom <= 0 {
		// Tiny n: the discrete correction would dominate; fall back to the
		// continuous coefficient rather than producing a negative/huge sigma.
		denom = math.Sqrt(float64(n)) * rangeCoef
	}

	return targetLogRange / denom
}

// targetLogRange derives the calibration target from a 24h OHLC ticker: the
// natural log of the high/low ratio. This is the quantity that encodes the
// symbol's 24h swing amplitude (a 15%-range coin yields ~ln(1.15), a 1%-range
// stablecoin ~ln(1.01)). The open/close direction is intentionally discarded;
// only the swing magnitude is reproduced.
func targetLogRange(t Ticker) float64 {
	if t.High <= 0 || t.Low <= 0 || t.High < t.Low {
		return 0
	}

	return math.Log(t.High / t.Low)
}

// PriceModel generates driftless geometric Gaussian random-walk BBO paths whose
// expected 24h high-low swing matches a symbol's OHLC ticker amplitude.
type PriceModel struct {
	startPrice float64
	halfSpread float64 // half of the relative bid-ask spread, (ask-bid)/mid/2
	sigmaStep  float64
	ticks      int
	levelSize  decimal.Decimal
}

// NewPriceModel calibrates a price model from a benchmark config. The per-step
// volatility is derived from the ticker's high-low range so that, across seeds,
// the mean simulated swing matches the symbol's actual 24h swing.
func NewPriceModel(cfg Config) PriceModel {
	// The path has cfg.Ticks samples and therefore cfg.Ticks-1 increments; the
	// range statistic is over the number of increments.
	steps := cfg.Ticks - 1
	if steps < 1 {
		steps = 1
	}

	size := cfg.BBOLevelSize
	if size <= 0 {
		size = defaultLevelSize
	}

	return PriceModel{
		startPrice: cfg.StartPrice,
		halfSpread: cfg.Spread / 2,
		sigmaStep:  stepSigma(targetLogRange(cfg.Ticker), steps),
		ticks:      cfg.Ticks,
		levelSize:  decimal.NewFromFloat(size),
	}
}

// SigmaStep exposes the calibrated per-step log volatility (mostly for tests and
// diagnostics).
func (m PriceModel) SigmaStep() float64 { return m.sigmaStep }

// Generate produces a deterministic BBO path for the given RNG. The first tick
// sits exactly at the configured start price; each subsequent tick applies one
// zero-drift Gaussian log-return step. A symmetric relative spread is wrapped
// around every midprice.
func (m PriceModel) Generate(rng *rand.Rand) []models.BBO {
	bbos := make([]models.BBO, m.ticks)
	logP := math.Log(m.startPrice)
	base := time.Now().UTC()

	for i := 0; i < m.ticks; i++ {
		if i > 0 {
			logP += m.sigmaStep * rng.NormFloat64()
		}
		mid := math.Exp(logP)
		bid := mid * (1 - m.halfSpread)
		ask := mid * (1 + m.halfSpread)
		bbos[i] = models.BBO{
			Bid:       models.PriceLevel{Price: decimal.NewFromFloat(bid), Size: m.levelSize},
			Ask:       models.PriceLevel{Price: decimal.NewFromFloat(ask), Size: m.levelSize},
			Timestamp: base.Add(time.Duration(i) * time.Second),
		}
	}

	return bbos
}
