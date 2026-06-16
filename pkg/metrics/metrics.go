package metrics

import (
	"time"

	"degen/pkg/models"

	"github.com/prometheus/client_golang/prometheus"
)

var (
	requestDurationHist = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name: "request_duration_seconds",
		Help: "The duration of requests",
	}, []string{"exchange", "path"})
	placeOrderDurationHist = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name: "place_order_duration_seconds",
		Help: "End to end duration of placing an order",
	}, []string{"exchange"})
	assetBalanceGauge = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "asset_balance",
		Help: "The balance of an asset",
	}, []string{"exchange", "asset"})
	spreadGauge = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "price_spread",
		Help: "The spread of an asset price",
	}, []string{"exchange", "asset"})
	midpriceGague = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "asset_midprice",
		Help: "The midprice of an asset",
	}, []string{"exchange", "asset"})
	realizedPnLGauge = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "realized_pnl",
		Help: "The realized PnL of a position",
	}, []string{"exchange", "asset"})
	unrealizedPnLGauge = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "unrealized_pnl",
		Help: "The unrealized PnL of a position",
	}, []string{"exchange", "asset"})
	positionSizeGauge = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "position_size",
		Help: "The size of a position",
	}, []string{"exchange", "asset"})
	positionAveragePriceGauge = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "position_average_price",
		Help: "The average price of a position",
	}, []string{"exchange", "asset"})
)

func init() {
	prometheus.MustRegister(
		requestDurationHist,
		placeOrderDurationHist,
		assetBalanceGauge,
		spreadGauge,
		midpriceGague,
		realizedPnLGauge,
		unrealizedPnLGauge,
		positionSizeGauge,
		positionAveragePriceGauge,
	)
}

// ignoredExchange is the in-memory dummy exchange used by the backtesting bench
// (including the live bot's startup auto-tune). Its synthetic orders, fills and
// positions must never pollute the real Prometheus series, so all recorders
// no-op for it.
const ignoredExchange = "dummy"

func ignore(exchange string) bool { return exchange == ignoredExchange }

func RecordPosition(exchange, asset string, position models.Position) {
	if ignore(exchange) {
		return
	}
	positionSizeGauge.WithLabelValues(exchange, asset).Set(position.Amount.InexactFloat64())
	positionAveragePriceGauge.WithLabelValues(exchange, asset).Set(position.AveragePrice.InexactFloat64())
	realizedPnLGauge.WithLabelValues(exchange, asset).Set(position.RealizedPnL.InexactFloat64())
}

func RecordUnrealizedPnL(exchange, asset string, pnl float64) {
	if ignore(exchange) {
		return
	}
	unrealizedPnLGauge.WithLabelValues(exchange, asset).Set(pnl)
}

func RecordRequestDuration(exchange, path string, start time.Time) {
	if ignore(exchange) {
		return
	}
	duration := time.Since(start).Seconds()
	requestDurationHist.WithLabelValues(exchange, path).Observe(duration)
}

func RecordPlaceOrderDuration(exchange string, start time.Time) {
	if ignore(exchange) {
		return
	}
	duration := time.Since(start).Seconds()
	placeOrderDurationHist.WithLabelValues(exchange).Observe(duration)
}

func RecordAssetBalance(exchange, asset string, balance float64) {
	if ignore(exchange) {
		return
	}
	assetBalanceGauge.WithLabelValues(exchange, asset).Set(balance)
}

func RecordBBO(exchange, asset string, bbo models.BBO) {
	if ignore(exchange) {
		return
	}
	midprice := bbo.Midprice().InexactFloat64()
	midpriceGague.WithLabelValues(exchange, asset).Set(midprice)

	if spread, ok := bbo.Spread(); ok {
		spreadGauge.WithLabelValues(exchange, asset).Set(spread.InexactFloat64())
	}
}
