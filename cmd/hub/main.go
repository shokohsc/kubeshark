package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"

	"github.com/rs/zerolog"

	"github.com/kubeshark/kubeshark/hub"
	"github.com/kubeshark/kubeshark/misc"
)

// ponytail: ring capacity fixed at 100000 to match the proprietary hub; not configurable via flag or env.
const ringSize = 100000

func main() {
	port := flag.Int("port", 8080, "port to listen on")
	logLevel := flag.String("loglevel", "warning", "log level: debug, info, warning, error, disabled")
	flag.Parse()

	zerolog.SetGlobalLevel(levelOf(*logLevel))

	cfg := hub.Config{
		AuthEnabled:     envBool("AUTH_ENABLED"),
		ServiceAccounts: envCSV("AUTH_CLI_SERVICE_ACCOUNTS"),
		License:         os.Getenv("LICENSE"),
		RingSize:        ringSize,
		LogLevel:        *logLevel,
		Version:         misc.Ver,
	}
	srv := hub.NewServer(cfg, hub.NewStore())

	addr := fmt.Sprintf(":%d", *port)
	log.Printf("kubeshark hub listening on %s", addr)
	if err := http.ListenAndServe(addr, srv.Handler()); err != nil {
		log.Fatal(err)
	}
}

func envBool(name string) bool {
	v := os.Getenv(name)
	if v == "" {
		return false
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		log.Fatalf("%s must be a boolean, got %q", name, v)
	}
	return b
}

func envCSV(name string) []string {
	var out []string
	for _, part := range strings.Split(os.Getenv(name), ",") {
		if p := strings.TrimSpace(part); p != "" {
			out = append(out, p)
		}
	}
	return out
}

func levelOf(s string) zerolog.Level {
	switch strings.ToLower(s) {
	case "debug":
		return zerolog.DebugLevel
	case "info":
		return zerolog.InfoLevel
	case "error":
		return zerolog.ErrorLevel
	case "disabled":
		return zerolog.Disabled
	default:
		return zerolog.WarnLevel
	}
}
