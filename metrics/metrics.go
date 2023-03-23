package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"time"
)

var defaultMetrics *Metrics

type Metrics struct {
	clients                   prometheus.Gauge
	subs                      *prometheus.GaugeVec
	readWriteLatencyHistogram *prometheus.HistogramVec
}

func Registerer() prometheus.Registerer {
	return registerMetrics()
}

func registerMetrics() *prometheus.Registry {

	defaultMetrics = &Metrics{}
	//new registry
	registry := prometheus.NewRegistry()
	factory := promauto.With(registry)

	defaultMetrics.clients = factory.NewGauge(
		prometheus.GaugeOpts{
			Name: "thetan_multiplayer_hub_clients_count",
			Help: "Number of clients currently connected",
		},
	)

	defaultMetrics.subs = factory.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "thetan_multiplayer_hub_subscriptions_count",
			Help: "Number of user subscribed to a topic",
		},
		[]string{"type", "topic"},
	)

	defaultMetrics.readWriteLatencyHistogram =
		factory.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "websocket_read_write_latency_seconds",
				Help:    "Latency of websocket read-write operations in seconds.",
				Buckets: []float64{0.01, 0.05, 0.1, 0.5, 1, 5},
			},
			[]string{"operation"},
		)
	return registry
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

	defaultMetrics.readWriteLatencyHistogram.WithLabelValues("read").Observe(time.Since(startTime).Seconds())
}
func RecordWriteLatencyMessage(startTime time.Time) {
	if defaultMetrics == nil {
		return
	}

	defaultMetrics.readWriteLatencyHistogram.WithLabelValues("write").Observe(time.Since(startTime).Seconds())
}
