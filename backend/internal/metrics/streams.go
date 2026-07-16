package metrics

import (
	"net/http"
	"sync"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

const namespace = "leadecho"

var (
	once sync.Once

	StreamConsumerPending = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: namespace,
		Name:      "stream_consumer_pending",
		Help:      "Redis stream pending messages per consumer group.",
	}, []string{"stream", "group"})

	StreamMessagesProcessed = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: namespace,
		Name:      "stream_messages_processed_total",
		Help:      "Stream messages processed by consumers.",
	}, []string{"stream", "group", "result"})

	StreamMessageDuration = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Namespace: namespace,
		Name:      "stream_message_duration_seconds",
		Help:      "Stream message handler duration in seconds.",
		Buckets:   prometheus.DefBuckets,
	}, []string{"stream", "group"})

	StreamDLQOpen = prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace: namespace,
		Name:      "stream_dlq_open_total",
		Help:      "Open dead-letter events awaiting resolution.",
	})
)

func Register() {
	once.Do(func() {
		prometheus.MustRegister(
			StreamConsumerPending,
			StreamMessagesProcessed,
			StreamMessageDuration,
			StreamDLQOpen,
		)
	})
}

func Handler() http.Handler {
	Register()
	return promhttp.Handler()
}

func ObserveProcessed(stream, group, result string, seconds float64) {
	Register()
	StreamMessagesProcessed.WithLabelValues(stream, group, result).Inc()
	if result == "success" {
		StreamMessageDuration.WithLabelValues(stream, group).Observe(seconds)
	}
}

func SetPending(stream, group string, pending float64) {
	Register()
	StreamConsumerPending.WithLabelValues(stream, group).Set(pending)
}

func SetDLQOpen(count float64) {
	Register()
	StreamDLQOpen.Set(count)
}
