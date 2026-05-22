package notifier

import "github.com/prometheus/client_golang/prometheus"

var (
	NotificationsSent = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "notifications_sent_total",
		Help: "Total notifications successfully delivered.",
	})

	NotificationsSkipped = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "notifications_skipped_online_total",
		Help: "Notifications skipped because recipient was already online.",
	})

	NotificationErrors = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "notification_delivery_errors_total",
		Help: "Notification deliveries that failed after all retries.",
	})

	NotificationDuration = prometheus.NewHistogram(prometheus.HistogramOpts{
		Name:    "notification_delivery_duration_seconds",
		Help:    "Time spent on notification delivery including retries.",
		Buckets: prometheus.DefBuckets,
	})
)

func init() {
	prometheus.MustRegister(NotificationsSent, NotificationsSkipped, NotificationErrors, NotificationDuration)
}
