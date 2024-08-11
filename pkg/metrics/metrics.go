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
)

func init() {
	prometheus.MustRegister(requestDurationHist)
	prometheus.MustRegister(placeOrderDurationHist)
	prometheus.MustRegister(assetBalanceGauge)
	prometheus.MustRegister(spreadGauge)
	prometheus.MustRegister(midpriceGague)
}

func RecordRequestDuration(exchange, path string, start time.Time) {
	duration := time.Since(start).Seconds()
	requestDurationHist.WithLabelValues(exchange, path).Observe(duration)
}

func RecordPlaceOrderDuration(exchange string, start time.Time) {
	duration := time.Since(start).Seconds()
	placeOrderDurationHist.WithLabelValues(exchange).Observe(duration)
}

func RecordAssetBalance(exchange, asset string, balance float64) {
	assetBalanceGauge.WithLabelValues(exchange, asset).Set(balance)
}

func RecordBBO(exchange, asset string, bbo models.BBO) {
	midprice := bbo.Midprice().InexactFloat64()
	midpriceGague.WithLabelValues(exchange, asset).Set(midprice)

	if spread, ok := bbo.Spread(); ok {
		spreadGauge.WithLabelValues(exchange, asset).Set(spread.InexactFloat64())
	}
}
