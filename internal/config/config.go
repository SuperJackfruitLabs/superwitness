// Package config reads superwitness's settings from the environment.
package config

import (
	"errors"
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const (
	DefaultListen        = ":8790"
	DefaultFakeListen    = "127.0.0.1:8790"
	DefaultSourceTimeout = 2 * time.Second
)

type Config struct {
	Listen              string
	PublicURL           string // the audience caller tokens must carry
	DatabaseURL         string // the runtime role: never the table owner
	MigrateDatabaseURL  string // the owner role, for migrations only; empty means DatabaseURL
	SuperpipelineURL    string
	HubURL              string
	HubClientID         string // the svc_… service credential id
	HubClientSecretFile string
	TracesURL           string
	TracesTokenFile     string
	LogsURL             string
	LogsTokenFile       string
	SourceTimeout       time.Duration
	OTLPEndpoint        string
	FakeSources         bool
}

func Load(getenv func(string) string) (Config, error) {
	c := Config{
		Listen:              strings.TrimSpace(getenv("SW_LISTEN")),
		PublicURL:           trimURL(getenv("SW_PUBLIC_URL")),
		DatabaseURL:         strings.TrimSpace(getenv("SW_DATABASE_URL")),
		MigrateDatabaseURL:  strings.TrimSpace(getenv("SW_MIGRATE_DATABASE_URL")),
		SuperpipelineURL:    trimURL(getenv("SW_SUPERPIPELINE_URL")),
		HubURL:              trimURL(getenv("SW_HUB_URL")),
		HubClientID:         strings.TrimSpace(getenv("SW_HUB_CLIENT_ID")),
		HubClientSecretFile: strings.TrimSpace(getenv("SW_HUB_CLIENT_SECRET_FILE")),
		TracesURL:           trimURL(getenv("SW_TRACES_URL")),
		TracesTokenFile:     strings.TrimSpace(getenv("SW_TRACES_TOKEN_FILE")),
		LogsURL:             trimURL(getenv("SW_LOGS_URL")),
		LogsTokenFile:       strings.TrimSpace(getenv("SW_LOGS_TOKEN_FILE")),
		OTLPEndpoint:        trimURL(getenv("SW_OTLP_ENDPOINT")),
		SourceTimeout:       DefaultSourceTimeout,
	}

	fake, err := parseBool(getenv("SW_FAKE_SOURCES"))
	if err != nil {
		return Config{}, fmt.Errorf("SW_FAKE_SOURCES: %w", err)
	}
	c.FakeSources = fake

	if v := strings.TrimSpace(getenv("SW_SOURCE_TIMEOUT")); v != "" {
		d, err := time.ParseDuration(v)
		if err != nil || d <= 0 {
			return Config{}, fmt.Errorf("SW_SOURCE_TIMEOUT: want a positive duration such as 2s, got %q", v)
		}
		c.SourceTimeout = d
	}

	var missing []string
	need := func(name, v string) {
		if v == "" {
			missing = append(missing, name)
		}
	}
	need("SW_DATABASE_URL", c.DatabaseURL)

	if c.FakeSources {
		if c.Listen == "" {
			c.Listen = DefaultFakeListen
		}
		if !isLoopback(c.Listen) {
			return Config{}, fmt.Errorf("SW_FAKE_SOURCES accepts development tokens, so SW_LISTEN must be a loopback address, got %q", c.Listen)
		}
		if c.PublicURL == "" {
			c.PublicURL = "http://" + c.Listen
		}
	} else {
		if c.Listen == "" {
			c.Listen = DefaultListen
		}
		need("SW_PUBLIC_URL", c.PublicURL)
		need("SW_SUPERPIPELINE_URL", c.SuperpipelineURL)
		need("SW_HUB_URL", c.HubURL)
		need("SW_HUB_CLIENT_ID", c.HubClientID)
		need("SW_HUB_CLIENT_SECRET_FILE", c.HubClientSecretFile)
		need("SW_TRACES_URL", c.TracesURL)
		need("SW_LOGS_URL", c.LogsURL)
	}
	if len(missing) > 0 {
		return Config{}, fmt.Errorf("missing required settings: %s", strings.Join(missing, ", "))
	}

	for _, u := range []struct{ name, v string }{
		{"SW_PUBLIC_URL", c.PublicURL}, {"SW_SUPERPIPELINE_URL", c.SuperpipelineURL}, {"SW_HUB_URL", c.HubURL},
		{"SW_TRACES_URL", c.TracesURL}, {"SW_LOGS_URL", c.LogsURL}, {"SW_OTLP_ENDPOINT", c.OTLPEndpoint},
	} {
		if u.v == "" {
			continue
		}
		p, err := url.Parse(u.v)
		if err != nil || (p.Scheme != "http" && p.Scheme != "https") || p.Host == "" {
			return Config{}, fmt.Errorf("%s: want an absolute http(s) URL, got %q", u.name, u.v)
		}
	}
	return c, nil
}

// MigrateDSN is the DSN migrations run over: SW_MIGRATE_DATABASE_URL, else SW_DATABASE_URL.
func (c Config) MigrateDSN() string {
	if c.MigrateDatabaseURL != "" {
		return c.MigrateDatabaseURL
	}
	return c.DatabaseURL
}

// LoadMigrate reads only what `superwitness migrate` needs: the effective migrate DSN.
func LoadMigrate(getenv func(string) string) (string, error) {
	c := Config{
		DatabaseURL:        strings.TrimSpace(getenv("SW_DATABASE_URL")),
		MigrateDatabaseURL: strings.TrimSpace(getenv("SW_MIGRATE_DATABASE_URL")),
	}
	if dsn := c.MigrateDSN(); dsn != "" {
		return dsn, nil
	}
	return "", ErrNoMigrateDSN
}

// ErrNoMigrateDSN means neither SW_MIGRATE_DATABASE_URL nor SW_DATABASE_URL is set.
var ErrNoMigrateDSN = errors.New("missing required settings: SW_MIGRATE_DATABASE_URL or SW_DATABASE_URL")

func trimURL(s string) string { return strings.TrimRight(strings.TrimSpace(s), "/") }

func parseBool(s string) (bool, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return false, nil
	}
	b, err := strconv.ParseBool(s)
	if err != nil {
		return false, errors.New("want true/false/1/0, got " + strconv.Quote(s))
	}
	return b, nil
}

func isLoopback(listen string) bool {
	host, _, err := net.SplitHostPort(listen)
	if err != nil {
		return false
	}
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}
