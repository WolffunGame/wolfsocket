package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"net/http"
	"time"
)

var defaultMetrics *Metrics

type Metrics struct {
	clients               prometheus.Gauge
	subs                  *prometheus.GaugeVec
	readLatencyHistogram  *prometheus.HistogramVec
	writeLatencyHistogram *prometheus.HistogramVec
}

func Enable() http.Handler {
	defaultMetrics = &Metrics{}
	registerMetrics()

	//default register
	return promhttp.Handler()
}

func registerMetrics() {
	defaultMetrics.clients = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "thetan_multiplayer_hub_clients_count",
			Help: "Number of clients currently connected",
		},
	)

	defaultMetrics.subs = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "thetan_multiplayer_hub_subscriptions_count",
			Help: "Number of user subscribed to a topic",
		},
		[]string{"type", "topic"},
	)

	defaultMetrics.readLatencyHistogram = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "websocket_read_latency_seconds",
			Help:    "Latency of websocket read operations in seconds.",
			Buckets: []float64{0.01, 0.05, 0.1, 0.5, 1, 5},
		},
		[]string{"operation"},
	)
}

func RecordHubClientNew() {
	if defaultMetrics == nil {
		return
	}
	defaultMetrics.clients.Inc()
}

func RecordHubClientClose() {
	if defaultMetrics == nil {
		return
	}
	defaultMetrics.clients.Dec()
}

func RecordHubSubscription(typ, topic string) {
	if defaultMetrics == nil {
		return
	}
	defaultMetrics.subs.WithLabelValues(typ, topic).Inc()
}

func RecordHubUnsubscription(typ, topic string) {
	if defaultMetrics == nil {
		return
	}
	defaultMetrics.subs.WithLabelValues(typ, topic).Dec()
}

func RecordReadLatencyMessage(startTime time.Time) {
	if defaultMetrics == nil {
		return
	}

	defaultMetrics.readLatencyHistogram.WithLabelValues("read").Observe(time.Since(startTime).Seconds())
}
func RecordWriteLatencyMessage(startTime time.Time) {
	if defaultMetrics == nil {
		return
	}

	defaultMetrics.writeLatencyHistogram.WithLabelValues("write").Observe(time.Since(startTime).Seconds())
}
