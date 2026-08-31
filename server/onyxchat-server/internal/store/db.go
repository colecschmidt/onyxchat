package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/url"
	"os"
	"time"

	"github.com/XSAM/otelsql"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/secretsmanager"
	"github.com/aws/aws-sdk-go-v2/service/ssm"
	_ "github.com/jackc/pgx/v5/stdlib"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
)

func ResolveSecret(envKey, ssmPath string) string {
	if v := os.Getenv(envKey); v != "" {
		return v
	}
	cfg, err := config.LoadDefaultConfig(context.Background())
	if err != nil {
		log.Fatalf("failed to load AWS config: %v", err)
	}
	client := ssm.NewFromConfig(cfg)
	withDecryption := true
	out, err := client.GetParameter(context.Background(), &ssm.GetParameterInput{
		Name:           &ssmPath,
		WithDecryption: &withDecryption,
	})
	if err != nil {
		log.Fatalf("failed to fetch SSM param %s: %v", ssmPath, err)
	}
	return *out.Parameter.Value
}

// resolveDBDSN returns SM_DB_DSN directly when it's set (local dev, tests,
// and CI all pass a plain DSN this way). In prod no such env var is set, so
// it fetches the password straight from the RDS-managed Secrets Manager
// secret instead of a Terraform-frozen SSM copy of it — that copy only
// refreshes on `terraform apply` and goes stale between the secret's own
// auto-rotations (every ~7 days), which is what caused new tasks to fail
// DB auth and get rolled back by the ECS deployment circuit breaker on
// 2026-08-30: the secret had rotated two days earlier and nobody had
// re-applied Terraform since.
func resolveDBDSN() string {
	if v := os.Getenv("SM_DB_DSN"); v != "" {
		return v
	}

	secretARN := os.Getenv("SM_DB_SECRET_ARN")
	cfg, err := config.LoadDefaultConfig(context.Background())
	if err != nil {
		log.Fatalf("failed to load AWS config: %v", err)
	}
	out, err := secretsmanager.NewFromConfig(cfg).GetSecretValue(context.Background(), &secretsmanager.GetSecretValueInput{
		SecretId: &secretARN,
	})
	if err != nil {
		log.Fatalf("failed to fetch DB secret %s: %v", secretARN, err)
	}

	var creds struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := json.Unmarshal([]byte(*out.SecretString), &creds); err != nil {
		log.Fatalf("failed to parse DB secret: %v", err)
	}

	return fmt.Sprintf("postgres://%s:%s@%s:%s/%s",
		creds.Username, url.QueryEscape(creds.Password), os.Getenv("SM_DB_HOST"), os.Getenv("SM_DB_PORT"), os.Getenv("SM_DB_NAME"))
}

func MustOpen() *sql.DB {
	dsn := resolveDBDSN()
	db, err := otelsql.Open("pgx", dsn,
		otelsql.WithAttributes(semconv.DBSystemPostgreSQL),
		otelsql.WithSpanOptions(otelsql.SpanOptions{
			RecordError: func(err error) bool { return err != nil },
		}),
	)
	if err != nil {
		log.Fatalf("failed to open db: %v", err)
	}
	// db.t4g.micro's default parameter group caps Postgres at ~112 total
	// connections. ECS autoscaling allows up to 6 tasks (ecr.tf), and
	// deployment_maximum_percent is capped at 150% (main.tf) so a rolling
	// deploy runs at most 9 tasks concurrently. Sized so 9 * maxOpenConnsPerTask
	// stays well under the server-wide ceiling, leaving headroom for
	// migrations/admin access.
	const maxOpenConnsPerTask = 8
	db.SetMaxOpenConns(maxOpenConnsPerTask)
	db.SetMaxIdleConns(maxOpenConnsPerTask)
	db.SetConnMaxLifetime(5 * time.Minute)
	db.SetConnMaxIdleTime(1 * time.Minute)
	if err := db.Ping(); err != nil {
		log.Fatalf("failed to ping db: %v", err)
	}
	return db
}
