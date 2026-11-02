package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/abcdlsj/weft/internal/weft"
)

const usage = `weft — weave many hosts into one subscription

usage:
  weft serve [flags]              run the panel and subscription endpoint
  weft agent [flags]              report a local mihomo to a weft server

run "weft serve -h" or "weft agent -h" for flags.`

func main() {
	if len(os.Args) < 2 {
		fmt.Println(usage)
		os.Exit(1)
	}
	switch os.Args[1] {
	case "serve":
		runServe(os.Args[2:])
	case "agent":
		runAgent(os.Args[2:])
	case "version":
		fmt.Println(weft.AgentVersion)
	case "help", "-h", "--help":
		fmt.Println(usage)
	default:
		fmt.Printf("unknown command %q\n\n%s\n", os.Args[1], usage)
		os.Exit(1)
	}
}

func runServe(args []string) {
	fs := flag.NewFlagSet("serve", flag.ExitOnError)
	listen := fs.String("listen", ":8787", "listen address")
	dbPath := fs.String("db", "weft.db", "sqlite database path")
	demo := fs.Bool("demo", false, "seed a demo fleet and serve from memory")
	password := fs.String("password", "", "set the panel password and exit the setup flow")
	fs.Parse(args)

	path := *dbPath
	if *demo {
		path = ":memory:"
	}
	st, err := weft.OpenStore(path)
	if err != nil {
		log.Fatalf("store: %v", err)
	}
	defer st.Close()

	if *demo {
		if _, err := weft.SeedDemo(st); err != nil {
			log.Fatalf("seed: %v", err)
		}
		stop := make(chan struct{})
		defer close(stop)
		go weft.TickDemo(st, stop)
		log.Printf("demo fleet seeded: 4 hosts, 48 nodes, 2 bundles")
	}

	srv := weft.NewServer(st, *demo)
	if *password != "" {
		if err := weft.SetPassword(st, *password); err != nil {
			log.Fatalf("password: %v", err)
		}
	} else if *demo && !srv.PasswordSet() {
		if err := weft.SetPassword(st, "weft-demo"); err != nil {
			log.Fatalf("password: %v", err)
		}
		log.Printf("demo panel password: weft-demo")
	}

	httpSrv := &http.Server{
		Addr:              *listen,
		Handler:           logRequests(srv.Handler()),
		ReadHeaderTimeout: 10 * time.Second,
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	go func() {
		<-ctx.Done()
		shutCtx, c := context.WithTimeout(context.Background(), 5*time.Second)
		defer c()
		httpSrv.Shutdown(shutCtx)
	}()

	log.Printf("weft %s listening on http://localhost%s", weft.AgentVersion, *listen)
	if err := httpSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatal(err)
	}
}

func runAgent(args []string) {
	fs := flag.NewFlagSet("agent", flag.ExitOnError)
	server := fs.String("server", "", "weft server base url, e.g. https://weft.example.com")
	token := fs.String("token", os.Getenv("WEFT_TOKEN"), "agent token issued by the panel")
	controller := fs.String("controller", "http://127.0.0.1:9090", "mihomo external-controller")
	secret := fs.String("secret", os.Getenv("MIHOMO_SECRET"), "mihomo external-controller secret")
	interval := fs.Duration("interval", 15*time.Second, "report interval")
	verbose := fs.Bool("verbose", false, "log each report")
	fs.Parse(args)

	if *server == "" || *token == "" {
		fmt.Println("agent needs --server and --token")
		os.Exit(1)
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	if err := weft.RunAgent(ctx, weft.AgentConfig{
		Server:     *server,
		Token:      *token,
		Controller: *controller,
		Secret:     *secret,
		Interval:   *interval,
		Verbose:    *verbose,
	}); err != nil && ctx.Err() == nil {
		log.Fatal(err)
	}
}

func logRequests(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		if len(r.URL.Path) > 6 && r.URL.Path[:5] == "/api/" {
			log.Printf("%s %s %s", r.Method, r.URL.Path, time.Since(start).Round(time.Millisecond))
		}
	})
}
