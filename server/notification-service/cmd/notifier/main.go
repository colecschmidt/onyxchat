package main

import (
	"context"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/cole/notification-service/internal/notifier"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

func main() {
	log, _ := zap.NewProduction()
	defer log.Sync()

	shutdownTracer := notifier.InitTracer(log)
	defer shutdownTracer(context.Background())

	redisAddr := os.Getenv("SM_REDIS_ADDR")
	if redisAddr == "" {
		redisAddr = "redis:6379"
	}

	rdb := redis.NewClient(&redis.Options{
		Addr:         redisAddr,
		DialTimeout:  5 * time.Second,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 5 * time.Second,
	})
	defer rdb.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	if err := rdb.Ping(ctx).Err(); err != nil {
		log.Fatal("failed to connect to Redis", zap.String("addr", redisAddr), zap.Error(err))
	}
	cancel()
	log.Info("connected to Redis", zap.String("addr", redisAddr))

	serverURL := os.Getenv("ONYXCHAT_SERVER_URL")
	if serverURL == "" {
		serverURL = "http://server:8080"
	}
	serviceSecret := os.Getenv("INTERNAL_SERVICE_SECRET")
	if serviceSecret == "" {
		serviceSecret = "dev-insecure-service-secret"
		log.Warn("INTERNAL_SERVICE_SECRET not set; using insecure dev default")
	}
	pushTokens := notifier.NewPushTokenClient(serverURL, serviceSecret)

	// Prometheus metrics endpoint — scraped by the existing Prometheus instance.
	metricsAddr := os.Getenv("NOTIF_METRICS_ADDR")
	if metricsAddr == "" {
		metricsAddr = ":9091"
	}
	go func() {
		mux := http.NewServeMux()
		mux.Handle("/metrics", promhttp.Handler())
		mux.HandleFunc("/health", func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		})
		log.Info("metrics server listening", zap.String("addr", metricsAddr))
		if err := http.ListenAndServe(metricsAddr, mux); err != nil {
			log.Error("metrics server error", zap.Error(err))
		}
	}()

	svc := notifier.New(rdb, pushTokens, log)

	// Restart loop — reconnects to Redis if the subscription drops.
	runCtx, runCancel := context.WithCancel(context.Background())
	defer runCancel()

	go func() {
		for {
			select {
			case <-runCtx.Done():
				return
			default:
			}
			if err := svc.Run(runCtx); err != nil {
				log.Error("service exited, restarting in 2s", zap.Error(err))
				select {
				case <-runCtx.Done():
					return
				case <-time.After(2 * time.Second):
				}
			}
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	sig := <-stop
	log.Info("shutdown signal received", zap.String("signal", sig.String()))
}
