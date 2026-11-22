// Command strapontin runs the personal theatre log server and its admin tasks.
package main

import (
	"bufio"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
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

func (c config) backupDir() string { return filepath.Join(c.dataDir, "backups") }

func env(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func usage() {
	fmt.Fprintln(os.Stderr, `usage: strapontin <command> [flags]

commands:
  serve            run the web server
  admin create     create an admin account
  backup <dir>     write a copy of the database to a directory
  export <login>   print a member's data as JSON
  reextract        re-run extraction on pages read by an older extractor
  snapshot <id>    print a stored page (for inspection only)`)
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
	case "export":
		if fs.NArg() != 1 {
			usage()
		}
		err = withDB(cfg, func(d *sql.DB) error {
			var id int64
			if err := d.QueryRow(`SELECT id FROM member WHERE login = ?`, strings.ToLower(fs.Arg(0))).Scan(&id); err != nil {
				return fmt.Errorf("member %q: %w", fs.Arg(0), err)
			}
			e, err := api.BuildExport(d, id)
			if err != nil {
				return err
			}
			enc := json.NewEncoder(os.Stdout)
			enc.SetIndent("", "  ")
			return enc.Encode(e)
		})
	case "reextract":
		err = withDB(cfg, func(d *sql.DB) error {
			n, err := api.Reextract(d)
			if err == nil {
				fmt.Printf("%d snapshots re-extracted\n", n)
			}
			return err
		})
	case "snapshot":
		id, convErr := strconv.ParseInt(fs.Arg(0), 10, 64)
		if fs.NArg() != 1 || convErr != nil {
			usage()
		}
		err = withDB(cfg, func(d *sql.DB) error {
			u, body, err := api.SnapshotBody(d, id)
			if err == nil {
				fmt.Fprintln(os.Stderr, u)
				os.Stdout.Write(body)
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
	return db.Open(filepath.Join(cfg.dataDir, "strapontin.db"), cfg.backupDir())
}

func withDB(cfg config, f func(*sql.DB) error) error {
	d, err := openDB(cfg)
	if err != nil {
		return err
	}
	defer d.Close()
	return f(d)
}

// lockDataDir blocks until no other server uses the data directory. During a
// zero-downtime deploy, Dokku starts the new container before stopping the old
// one: waiting here keeps the new one from migrating the schema under the old
// code. The lock lasts until the returned file is closed.
func lockDataDir(dataDir string) (*os.File, error) {
	if err := os.MkdirAll(dataDir, 0o750); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(filepath.Join(dataDir, "strapontin.lock"), os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, err
	}
	if syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB) != nil {
		log.Printf("another server uses %s, waiting for it to stop", dataDir)
		if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX); err != nil {
			f.Close()
			return nil, err
		}
	}
	return f, nil
}

func serve(cfg config) error {
	lock, err := lockDataDir(cfg.dataDir)
	if err != nil {
		return err
	}
	defer lock.Close()
	d, err := openDB(cfg)
	if err != nil {
		return err
	}
	defer d.Close()

	srv := &http.Server{
		Addr: ":" + cfg.port,
		Handler: api.New(d, api.Config{
			BaseURL: cfg.baseURL, TrustProxy: cfg.trustProxy, AllowPrivateFetch: cfg.localFetch, BackupDir: cfg.backupDir(),
		}, frontendFS()),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      60 * time.Second,
		IdleTimeout:       2 * time.Minute,
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	go db.DailyBackups(ctx, d, cfg.backupDir())
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
