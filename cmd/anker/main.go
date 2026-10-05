package main

import (
	"anker/internal/anker"
	"anker/internal/client"
	"anker/internal/tui"
	"anker/internal/webassets"
	"context"
	"errors"
	"flag"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "Anker:", err)
		os.Exit(1)
	}
}
func run(args []string) error {
	f := flag.NewFlagSet("anker", flag.ContinueOnError)
	data := f.String("data", env("ANKER_DATA", "/srv/anker"), "Datenverzeichnis")
	socket := f.String("socket", "", "Unix-Socket")
	listen := f.String("listen", "127.0.0.1:8087", "Webadresse")
	cert := f.String("tls-cert", "", "TLS-Zertifikat")
	key := f.String("tls-key", "", "TLS-Key")
	allowHTTP := f.Bool("allow-http", false, "HTTP außerhalb Loopback ausdrücklich zulassen")
	admin := f.String("admin", "admin", "initialer Benutzer")
	if err := f.Parse(args); err != nil {
		return err
	}
	command := f.Args()
	if len(command) == 0 {
		fmt.Println(client.Help)
		return nil
	}
	if command[0] == "help" || command[0] == "version" {
		if command[0] == "version" {
			fmt.Println("Anker 0.1.0")
		} else {
			fmt.Println(client.Help + "\nanker init | serve | demo\nWeb: --listen 127.0.0.1:8087 --tls-cert DATEI --tls-key DATEI")
		}
		return nil
	}
	if command[0] == "demo" && *data == "/srv/anker" {
		*data = "./var/demo"
	}
	root, err := filepath.Abs(*data)
	if err != nil {
		return err
	}
	if *socket == "" {
		*socket = filepath.Join(root, "anker.sock")
	}
	if command[0] != "serve" && command[0] != "demo" && command[0] != "init" {
		c := client.New(*socket)
		if command[0] == "tui" {
			return tui.Run(c)
		}
		return c.Run(context.Background(), command)
	}
	unlock, err := anker.InstanceLock(root)
	if err != nil {
		return err
	}
	defer unlock()
	store, err := anker.OpenStore(filepath.Join(root, "catalog.db"))
	if err != nil {
		return err
	}
	defer store.Close()
	auth := anker.NewAuth(store)
	var collector anker.Collector = anker.SSHCollector{}
	if command[0] == "demo" {
		collector = anker.DemoCollector{Root: root}
	}
	s, err := anker.NewService(root, store, collector)
	if err != nil {
		return err
	}
	if command[0] == "init" {
		password := os.Getenv("ANKER_INITIAL_PASSWORD")
		if password == "" {
			return errors.New("ANKER_INITIAL_PASSWORD mit mindestens 12 Zeichen setzen; keine Standardpasswörter")
		}
		if err = auth.CreateUser(*admin, password, "admin", true); err != nil {
			return err
		}
		fmt.Println("Anker initialisiert:", root)
		return nil
	}
	if command[0] == "demo" {
		if err = s.SeedDemo(auth); err != nil {
			return err
		}
		fmt.Fprintln(os.Stderr, "Lokale Demo · keine echten Hosts verbunden\nAnmeldung: demo / anker-demo-2026")
	}
	host, _, err := net.SplitHostPort(*listen)
	if err != nil {
		return err
	}
	ip := net.ParseIP(host)
	isLocal := host == "localhost" || (ip != nil && ip.IsLoopback())
	if !isLocal && (*cert == "" || *key == "") && !*allowHTTP {
		return errors.New("LAN/VPN-Webzugriff benötigt TLS; --tls-cert und --tls-key setzen")
	}
	if (*cert == "") != (*key == "") {
		return errors.New("TLS-Zertifikat und TLS-Key zusammen angeben")
	}
	if _, err = os.Lstat(*socket); err == nil {
		conn, err := net.DialTimeout("unix", *socket, time.Second)
		if err == nil {
			conn.Close()
			return errors.New("Anker-Dienst läuft bereits")
		}
		st, _ := os.Lstat(*socket)
		if st.Mode()&os.ModeSocket == 0 {
			return errors.New("Socketpfad ist keine Socketdatei")
		}
		if err = os.Remove(*socket); err != nil {
			return err
		}
	}
	if err = os.MkdirAll(filepath.Dir(*socket), 0700); err != nil {
		return err
	}
	unix, err := net.Listen("unix", *socket)
	if err != nil {
		return err
	}
	if err = s.RecoverJobs(); err != nil {
		unix.Close()
		os.Remove(*socket)
		return err
	}
	if err = s.RecoverStaging(); err != nil {
		unix.Close()
		os.Remove(*socket)
		return err
	}
	defer unix.Close()
	defer os.Remove(*socket)
	if err = os.Chmod(*socket, 0600); err != nil {
		return err
	}
	local := &http.Server{Handler: anker.Handler(s, auth, true), ReadHeaderTimeout: 5 * time.Second, IdleTimeout: 60 * time.Second}
	web := &http.Server{Addr: *listen, Handler: webassets.Handler(anker.Handler(s, auth, false)), ReadHeaderTimeout: 5 * time.Second, IdleTimeout: 60 * time.Second}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	errs := make(chan error, 2)
	go func() { errs <- local.Serve(unix) }()
	go func() {
		if *cert != "" {
			errs <- web.ListenAndServeTLS(*cert, *key)
		} else {
			errs <- web.ListenAndServe()
		}
	}()
	if !s.Demo {
		go anker.NewScheduler(s).Run(ctx)
	}
	scheme := "http"
	if *cert != "" {
		scheme = "https"
	}
	fmt.Fprintln(os.Stderr, "Anker:", scheme+"://"+*listen, "· Socket:", *socket)
	select {
	case <-ctx.Done():
	case err = <-errs:
		if !errors.Is(err, http.ErrServerClosed) {
			stop()
		}
	}
	shutdown, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	web.Shutdown(shutdown)
	local.Shutdown(shutdown)
	if stopErr := s.StopJobs(shutdown); stopErr != nil {
		return fmt.Errorf("Aufträge konnten nicht rechtzeitig beendet werden: %w", stopErr)
	}
	if err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}
func env(k, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(k)); v != "" {
		return v
	}
	return fallback
}
