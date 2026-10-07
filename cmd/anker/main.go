package main

import (
	"anker/internal/anker"
	"anker/internal/buildinfo"
	"anker/internal/client"
	"anker/internal/tui"
	"anker/internal/updater"
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
	listen := f.String("listen", env("ANKER_LISTEN", "127.0.0.1:8087"), "Webadresse")
	cert := f.String("tls-cert", env("ANKER_TLS_CERT", ""), "TLS-Zertifikat")
	key := f.String("tls-key", env("ANKER_TLS_KEY", ""), "TLS-Key")
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
			fmt.Println("Anker " + buildinfo.Version)
		} else {
			fmt.Println(client.Help + "\nanker setup | init | serve | demo\nWeb: --listen 127.0.0.1:8087 --tls-cert DATEI --tls-key DATEI")
		}
		return nil
	}
	if command[0] == "install-local" {
		if len(command) != 1 {
			return errors.New("Installationsbefehl ohne Argumente verwenden")
		}
		return runLocalInstall()
	}
	if command[0] == "updater-serve" {
		ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		defer stop()
		return updater.Serve(ctx, buildinfo.Version)
	}
	if command[0] == "update" {
		return runUpdate(command[1:])
	}
	if command[0] == "setup" {
		if *data != updater.DataDir {
			return errors.New("Servereinrichtung verwendet /srv/anker")
		}
		if len(command) > 2 || (len(command) == 2 && command[1] != "--updates") {
			return errors.New("Einrichtung: anker setup [--updates]")
		}
		return runSetup(len(command) == 2)
	}
	if command[0] == "host" && len(command) > 1 && command[1] == "trust" {
		if *data != updater.DataDir {
			return errors.New("Hostanbindung verwendet die produktive Einrichtung unter /etc/anker")
		}
		return runHostTrust(command[2:])
	}
	demo := command[0] == "demo"
	if demo {
		explicit := map[string]bool{}
		f.Visit(func(v *flag.Flag) { explicit[v.Name] = true })
		if !explicit["data"] {
			*data = "./var/demo"
		}
		if !explicit["listen"] {
			*listen = "127.0.0.1:8087"
		}
	}
	root, err := anker.ResolveDataRoot(*data)
	if err != nil {
		return err
	}
	if demo {
		production, err := anker.ResolveDataRoot(updater.DataDir)
		if err != nil {
			return err
		}
		toProduction, err := filepath.Rel(root, production)
		if err != nil {
			return err
		}
		fromProduction, err := filepath.Rel(production, root)
		if err != nil {
			return err
		}
		within := func(rel string) bool { return rel != ".." && !strings.HasPrefix(rel, ".."+string(os.PathSeparator)) }
		if within(toProduction) || within(fromProduction) {
			return errors.New("Demo darf das Produktionsverzeichnis /srv/anker und seine übergeordneten Verzeichnisse nicht verwenden")
		}
	}
	if *socket == "" {
		*socket = filepath.Join(root, "anker.sock")
	}
	if demo {
		resolved, err := anker.ResolveDataRoot(*socket)
		if err != nil {
			return err
		}
		if !strings.HasPrefix(resolved, root+string(os.PathSeparator)) {
			return errors.New("Demo-Socket muss im eigenen Demo-Datenverzeichnis liegen")
		}
		*socket = resolved
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
	if err = anker.EnsureDataMode(root, demo); err != nil {
		return err
	}
	if command[0] != "init" {
		host, _, err := net.SplitHostPort(*listen)
		if err != nil {
			return err
		}
		ip := net.ParseIP(host)
		isLocal := host == "localhost" || (ip != nil && ip.IsLoopback())
		if demo && !isLocal {
			return errors.New("Demo ist nur über eine lokale Loopback-Adresse erreichbar")
		}
		if !isLocal && (*cert == "" || *key == "") && !*allowHTTP {
			return errors.New("LAN/VPN-Webzugriff benötigt TLS; --tls-cert und --tls-key setzen")
		}
		if (*cert == "") != (*key == "") {
			return errors.New("TLS-Zertifikat und TLS-Key zusammen angeben")
		}
	}
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
		initialized, err := auth.Initialized()
		if err != nil {
			return err
		}
		if initialized {
			fmt.Println("Administratorzugang ist bereits eingerichtet; bestehende Benutzer bleiben erhalten.")
			return nil
		}
		password := os.Getenv("ANKER_INITIAL_PASSWORD")
		if password == "" {
			minimum, policyErr := auth.PasswordMinimum()
			if policyErr != nil {
				return policyErr
			}
			password, err = initialPassword(minimum)
			if err != nil {
				return err
			}
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
	webSocket, tlsConfig, err := bindWeb(*listen, *cert, *key)
	if err != nil {
		return err
	}
	defer webSocket.Close()
	local := &http.Server{Handler: anker.Handler(s, auth, true), ReadHeaderTimeout: 5 * time.Second, IdleTimeout: 60 * time.Second}
	web := &http.Server{TLSConfig: tlsConfig, Addr: *listen, Handler: webassets.Handler(anker.Handler(s, auth, false)), ReadHeaderTimeout: 5 * time.Second, IdleTimeout: 60 * time.Second}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	errs := make(chan error, 2)
	go func() { errs <- local.Serve(unix) }()
	go func() {
		if *cert != "" {
			errs <- web.ServeTLS(webSocket, "", "")
		} else {
			errs <- web.Serve(webSocket)
		}
	}()
	if !s.Demo {
		go anker.NewScheduler(s).Run(ctx)
		go s.RunStorage(ctx)
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
