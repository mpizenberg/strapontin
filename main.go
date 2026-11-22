// Command strapontin runs the personal theatre log server and its admin tasks.
package main

import (
	"bufio"
	"context"
	"database/sql"
	"errors"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/mpizenberg/strapontin/internal/api"
	"github.com/mpizenberg/strapontin/internal/auth"
	"github.com/mpizenberg/strapontin/internal/db"
)

type config struct {
	port       string
	dataDir    string
	trustProxy bool
	baseURL    string
	localFetch bool
}

func env(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func usage() {
	fmt.Fprintln(os.Stderr, `usage: strapontin <command> [flags]

commands:
  serve          run the web server
  admin create   create an admin account
  backup <path>  write a copy of the database to a directory`)
	os.Exit(2)
}

func main() {
	if len(os.Args) < 2 {
		usage()
	}
	cmd, args := os.Args[1], os.Args[2:]
	if cmd == "admin" {
		if len(args) == 0 || args[0] != "create" {
			usage()
		}
		cmd, args = "admin create", args[1:]
	}

	fs := flag.NewFlagSet(cmd, flag.ExitOnError)
	var cfg config
	fs.StringVar(&cfg.port, "port", env("PORT", "8080"), "HTTP listen port (PORT)")
	fs.StringVar(&cfg.dataDir, "data", env("DATA_DIR", "./data"), "database and backups directory (DATA_DIR)")
	fs.BoolVar(&cfg.trustProxy, "trust-proxy", os.Getenv("TRUST_PROXY") == "1", "read the client IP from X-Forwarded-For (TRUST_PROXY=1)")
	fs.StringVar(&cfg.baseURL, "base-url", env("BASE_URL", "http://localhost:8080"), "public URL, used in invite links (BASE_URL)")
	fs.BoolVar(&cfg.localFetch, "allow-local-fetch", false, "development only: let imports read local and private addresses")
	fs.Parse(args)

	var err error
	switch cmd {
	case "serve":
		err = serve(cfg)
	case "admin create":
		err = adminCreate(cfg)
	case "backup":
		if fs.NArg() != 1 {
			usage()
		}
		err = withDB(cfg, func(d *sql.DB) error {
			path, err := db.Backup(d, fs.Arg(0), "manual")
			if err == nil {
				fmt.Println(path)
			}
			return err
		})
	default:
		usage()
	}
	if err != nil {
		log.Fatal(err)
	}
}

func openDB(cfg config) (*sql.DB, error) {
	if err := os.MkdirAll(cfg.dataDir, 0o750); err != nil {
		return nil, err
	}
	return db.Open(filepath.Join(cfg.dataDir, "strapontin.db"), filepath.Join(cfg.dataDir, "backups"))
}

func withDB(cfg config, f func(*sql.DB) error) error {
	d, err := openDB(cfg)
	if err != nil {
		return err
	}
	defer d.Close()
	return f(d)
}

func serve(cfg config) error {
	d, err := openDB(cfg)
	if err != nil {
		return err
	}
	defer d.Close()

	srv := &http.Server{
		Addr:              ":" + cfg.port,
		Handler:           api.New(d, api.Config{BaseURL: cfg.baseURL, TrustProxy: cfg.trustProxy, AllowPrivateFetch: cfg.localFetch}, frontendFS()),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      60 * time.Second,
		IdleTimeout:       2 * time.Minute,
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	go func() {
		<-ctx.Done()
		shutdown, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		srv.Shutdown(shutdown)
	}()
	log.Printf("listening on :%s, data in %s", cfg.port, cfg.dataDir)
	if err := srv.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

func adminCreate(cfg config) error {
	in := bufio.NewReader(os.Stdin)
	ask := func(prompt string) string {
		fmt.Print(prompt)
		line, _ := in.ReadString('\n')
		return strings.TrimSpace(line)
	}
	name := ask("Name: ")
	login := strings.ToLower(ask("Login: "))
	password := ask("Password (echoed): ")
	if name == "" || login == "" {
		return errors.New("name and login are required")
	}
	if len(password) < auth.MinPasswordLength {
		return fmt.Errorf("the password needs at least %d characters", auth.MinPasswordLength)
	}
	return withDB(cfg, func(d *sql.DB) error {
		id, err := auth.CreateMember(d, name, login, password, "admin", nil)
		if err == nil {
			fmt.Printf("admin %q created (id %d)\n", login, id)
		}
		return err
	})
}
