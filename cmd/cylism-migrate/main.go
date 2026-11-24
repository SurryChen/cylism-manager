// cylism-migrate is copied into application images that opt into release migrations.
package main

import (
	"context"
	"database/sql"
	"fmt"
	"net"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/cylism/cylism-manager/internal/service/migration"
	"github.com/go-sql-driver/mysql"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "Migration failed:", err)
		os.Exit(1)
	}
}
func run() error {
	prefix := os.Getenv("CYLISM_MIGRATION_DATABASE_PREFIX")
	if prefix == "" {
		prefix = "DATABASE"
	}
	config := mysql.NewConfig()
	config.User = os.Getenv(prefix + "_USER")
	config.Passwd = os.Getenv(prefix + "_PASSWORD")
	config.DBName = os.Getenv(prefix + "_NAME")
	host := os.Getenv(prefix + "_HOST")
	port := os.Getenv(prefix + "_PORT")
	if port == "" {
		port = "3306"
	}
	n, err := strconv.Atoi(port)
	if err != nil || n < 1 || n > 65535 {
		return fmt.Errorf("invalid database port")
	}
	if config.User == "" || config.DBName == "" || host == "" {
		return fmt.Errorf("%s_HOST, _NAME and _USER must be configured", prefix)
	}
	config.Net = "tcp"
	config.Addr = net.JoinHostPort(host, port)
	config.MultiStatements = true
	config.Timeout = 10 * time.Second
	config.ParseTime = true
	tls := os.Getenv(prefix + "_TLS")
	if tls != "" && tls != "true" {
		return fmt.Errorf("database TLS setting must be true or omitted")
	}
	config.TLSConfig = tls
	directory := os.Getenv("CYLISM_MIGRATION_DIR")
	scripts, err := migration.ReadScripts(directory)
	if err != nil {
		return err
	}
	db, err := sql.Open("mysql", config.FormatDSN())
	if err != nil {
		return fmt.Errorf("invalid database connection configuration")
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	return migration.Run(ctx, db, scripts, os.Getenv("CYLISM_MIGRATION_TARGET"), os.Stdout)
}
