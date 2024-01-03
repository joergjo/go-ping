package main

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"database/sql"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path"
	"time"

	"github.com/go-sql-driver/mysql"
)

func main() {
	host := mustGetenv("MYSQL_HOST")
	database := mustGetenv("MYSQL_DATABASE")
	user := mustGetenv("MYSQL_USER")
	password := mustGetenv("MYSQL_PASSWORD")
	vol := mustGetenv("VOLUME_PATH")
	rootCACert := mustGetenv("ROOT_CA_CERT")

	listenAddr := os.Getenv("LISTEN_ADDR")
	if listenAddr == "" {
		listenAddr = ":8000"
	}

	db, err := initDB(host, database, user, password, rootCACert)
	if err != nil {
		slog.Error("initializing MySQL driver", "error", err)
		os.Exit(1)
	}
	defer db.Close()

	slog.Info("connected to database server", "host", host, "database", database, "user", user)
	slog.Info("using volume", "path", vol)

	pingHandler := newPingHandler(db, vol)
	http.HandleFunc("/ping", pingHandler)
	http.HandleFunc("/healthz/ready", pingHandler)
	http.HandleFunc("/healthz/live", ok)

	slog.Info("listening on " + listenAddr)
	http.ListenAndServe(listenAddr, nil)
}

func mustGetenv(k string) string {
	v := os.Getenv(k)
	if v == "" {
		slog.Error("missing required env var", "var", k)
		os.Exit(1)
	}
	return v
}

func initDB(host, database, user, password, rootCACert string) (*sql.DB, error) {
	certPool := x509.NewCertPool()
	pem, err := os.ReadFile(rootCACert)
	if err != nil {
		return nil, err
	}
	if ok := certPool.AppendCertsFromPEM(pem); !ok {
		return nil, err
	}
	// The key of the TLS config must match the tls parameter in the connection string!
	mysql.RegisterTLSConfig("custom", &tls.Config{RootCAs: certPool})
	connStr := fmt.Sprintf("%s:%s@tcp(%s:3306)/%s?allowNativePasswords=true&tls=custom", user, password, host, database)
	db, err := sql.Open("mysql", connStr)
	if err != nil {
		return nil, err
	}
	return db, nil
}

func newPingHandler(db *sql.DB, path string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()

		errC := make(chan error, 2)
		go func() {
			errC <- checkMySQL(ctx, db)
		}()
		go func() {
			errC <- checkFile(path)
		}()

		healthy := true
		for i := 0; i < 2; i++ {
			if err := <-errC; err != nil {
				healthy = false
				slog.Error("health check failed", "error", err)
			}
		}
		if !healthy {
			w.WriteHeader(http.StatusServiceUnavailable)
			fmt.Fprint(w, "DOWN")
			return
		}
		fmt.Fprint(w, "OK")
	}
}

func ok(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
}

func checkMySQL(ctx context.Context, db *sql.DB) error {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	return db.PingContext(ctx)
}

func checkFile(dir string) error {
	filename := path.Join(dir, "healthcheck.txt")
	file, err := os.Create(filename)
	if err != nil {
		return err
	}
	defer file.Close()
	return nil
}
