package kafka

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	kafkago "github.com/segmentio/kafka-go"
)

var kafkaConsumerLag = promauto.NewGaugeVec(
	prometheus.GaugeOpts{
		Name: "sagawallet_kafka_consumer_lag",
		Help: "Latest observed Kafka consumer lag by group and topic.",
	},
	[]string{"group_id", "topic"},
)

type readerStatsProvider interface {
	Stats() kafkago.ReaderStats
}

func (c *Consumer) observeLag(topic string) {
	statsProvider, ok := c.reader.(readerStatsProvider)
	if !ok {
		return
	}
	lag := statsProvider.Stats().Lag
	if lag < 0 {
		lag = 0
	}
	kafkaConsumerLag.WithLabelValues(c.cfg.GroupID, topic).Set(float64(lag))
}
