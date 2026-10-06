// Package config reads superwitness's settings from the environment.
package config

import (
	"errors"
	"fmt"
	"net"
	"net/netip"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
)

const (
	DefaultListen        = ":8790"
	DefaultFakeListen    = "127.0.0.1:8790"
	DefaultSourceTimeout = 2 * time.Second
)

// SourceNamePattern is what a run source may be called, in SW_RUN_SOURCES and in run reports.
const SourceNamePattern = `^[a-z][a-z0-9-]{0,31}$`

// PrincipalIDPattern is a hub principal id.
const PrincipalIDPattern = `^prn_[A-Za-z0-9_-]{1,64}$`

var (
	sourceName  = regexp.MustCompile(SourceNamePattern)
	principalID = regexp.MustCompile(PrincipalIDPattern)
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
	RunSources          map[string]string // SW_RUN_SOURCES: reporting principal (prn_…) → the one source it may report for
	AppClientID         string            // SW_APP_CLIENT_ID: the hub OAuth client browsers sign in through
	AllowedPrincipals   []string          // SW_ALLOWED_PRINCIPALS: who may sign in; empty turns sign-in off
	SessionSecretFile   string            // SW_SESSION_SECRET_FILE: 32+ bytes that key the login cookie
	TrustedProxies      []netip.Prefix    // SW_TRUSTED_PROXIES; nil means this host's own addresses

	// The organization plane. SW_ORG_PLANE_ISSUER set switches every token path to the plane:
	// caller verification, browser sign-in and superwitness's own service tokens. There is no
	// dual-accept: unset, the hub does all three exactly as before.
	OrgPlaneIssuer         string // SW_ORG_PLANE_ISSUER: compared exactly with iss, so never trimmed
	OrgPlaneJWKSURL        string // SW_ORG_PLANE_JWKS_URL
	OrgPlaneURL            string // SW_ORG_PLANE_URL: base of /api/auth/oauth2/* and /api/token/service
	OrgPlaneCredentialFile string // SW_ORG_PLANE_SERVICE_CREDENTIAL_FILE: one line, svc_<id>:<secret>
}

// OrgPlane reports whether the organization plane is the issuer.
func (c Config) OrgPlane() bool { return c.OrgPlaneIssuer != "" }

// AppEnabled reports whether browser sign-in is on: someone is allowed to sign in.
func (c Config) AppEnabled() bool { return len(c.AllowedPrincipals) > 0 }

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
	c.OrgPlaneIssuer = strings.TrimSpace(getenv("SW_ORG_PLANE_ISSUER"))
	c.OrgPlaneJWKSURL = strings.TrimSpace(getenv("SW_ORG_PLANE_JWKS_URL"))
	c.OrgPlaneURL = trimURL(getenv("SW_ORG_PLANE_URL"))
	c.OrgPlaneCredentialFile = strings.TrimSpace(getenv("SW_ORG_PLANE_SERVICE_CREDENTIAL_FILE"))
	if !c.OrgPlane() {
		var stray []string
		for _, s := range []struct{ name, v string }{{"SW_ORG_PLANE_JWKS_URL", c.OrgPlaneJWKSURL},
			{"SW_ORG_PLANE_URL", c.OrgPlaneURL}, {"SW_ORG_PLANE_SERVICE_CREDENTIAL_FILE", c.OrgPlaneCredentialFile}} {
			if s.v != "" {
				stray = append(stray, s.name)
			}
		}
		if len(stray) > 0 {
			return Config{}, fmt.Errorf("%s set without SW_ORG_PLANE_ISSUER: set the issuer to switch to the organization plane, or remove them",
				strings.Join(stray, ", "))
		}
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

	rs, err := parseRunSources(getenv("SW_RUN_SOURCES"))
	if err != nil {
		return Config{}, fmt.Errorf("SW_RUN_SOURCES: %w", err)
	}
	c.RunSources = rs

	c.AppClientID = strings.TrimSpace(getenv("SW_APP_CLIENT_ID"))
	c.SessionSecretFile = strings.TrimSpace(getenv("SW_SESSION_SECRET_FILE"))
	if c.AllowedPrincipals, err = parsePrincipals(getenv("SW_ALLOWED_PRINCIPALS")); err != nil {
		return Config{}, fmt.Errorf("SW_ALLOWED_PRINCIPALS: %w", err)
	}
	if c.TrustedProxies, err = parsePrefixes(getenv("SW_TRUSTED_PROXIES")); err != nil {
		return Config{}, fmt.Errorf("SW_TRUSTED_PROXIES: %w", err)
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
		if c.OrgPlane() {
			need("SW_ORG_PLANE_SERVICE_CREDENTIAL_FILE", c.OrgPlaneCredentialFile)
		} else {
			need("SW_HUB_CLIENT_ID", c.HubClientID)
			need("SW_HUB_CLIENT_SECRET_FILE", c.HubClientSecretFile)
		}
		need("SW_TRACES_URL", c.TracesURL)
		need("SW_LOGS_URL", c.LogsURL)
	}
	if c.OrgPlane() {
		need("SW_ORG_PLANE_JWKS_URL", c.OrgPlaneJWKSURL)
		need("SW_ORG_PLANE_URL", c.OrgPlaneURL)
	}
	if c.AppEnabled() {
		need("SW_APP_CLIENT_ID", c.AppClientID)
		need("SW_SESSION_SECRET_FILE", c.SessionSecretFile)
		if c.FakeSources && !c.OrgPlane() {
			need("SW_HUB_URL", c.HubURL) // sign-in still goes through a hub
		}
	}
	if len(missing) > 0 {
		return Config{}, fmt.Errorf("missing required settings: %s", strings.Join(missing, ", "))
	}
	// The session and login cookies are Secure, which a browser drops over plain http. Fake mode
	// is loopback-only and is allowed http so the end-to-end test can sign in.
	if c.AppEnabled() && !c.FakeSources && !strings.HasPrefix(c.PublicURL, "https://") {
		return Config{}, fmt.Errorf("SW_ALLOWED_PRINCIPALS turns sign-in on, which needs an https SW_PUBLIC_URL; got %q", c.PublicURL)
	}

	for _, u := range []struct{ name, v string }{
		{"SW_PUBLIC_URL", c.PublicURL}, {"SW_SUPERPIPELINE_URL", c.SuperpipelineURL}, {"SW_HUB_URL", c.HubURL},
		{"SW_TRACES_URL", c.TracesURL}, {"SW_LOGS_URL", c.LogsURL}, {"SW_OTLP_ENDPOINT", c.OTLPEndpoint},
		{"SW_ORG_PLANE_ISSUER", c.OrgPlaneIssuer}, {"SW_ORG_PLANE_JWKS_URL", c.OrgPlaneJWKSURL}, {"SW_ORG_PLANE_URL", c.OrgPlaneURL},
	} {
		if u.v == "" {
			continue
		}
		p, err := url.Parse(u.v)
		if err != nil || (p.Scheme != "http" && p.Scheme != "https") || p.Host == "" {
			return Config{}, fmt.Errorf("%s: want an absolute http(s) URL, got %q", u.name, u.v)
		}
	}
	if strings.HasSuffix(c.OrgPlaneIssuer, "/") {
		return Config{}, fmt.Errorf("SW_ORG_PLANE_ISSUER is compared exactly with each token's iss; remove the trailing slash from %q", c.OrgPlaneIssuer)
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

// parseRunSources reads "prn_<id>=<source>,…". Each principal is bound to exactly one source.
func parseRunSources(s string) (map[string]string, error) {
	out := map[string]string{}
	if strings.TrimSpace(s) == "" {
		return out, nil
	}
	for _, item := range strings.Split(s, ",") {
		prn, src, ok := strings.Cut(strings.TrimSpace(item), "=")
		prn, src = strings.TrimSpace(prn), strings.TrimSpace(src)
		if !ok || !principalID.MatchString(prn) || !sourceName.MatchString(src) {
			return nil, fmt.Errorf("want prn_<id>=<source>, comma-separated, with a source of lowercase letters, digits and -; got %q", strings.TrimSpace(item))
		}
		if _, dup := out[prn]; dup {
			return nil, fmt.Errorf("%s is bound to more than one source", prn)
		}
		out[prn] = src
	}
	return out, nil
}

// parsePrincipals reads comma-separated prn_ ids.
func parsePrincipals(s string) ([]string, error) {
	var out []string
	for _, item := range strings.Split(s, ",") {
		if item = strings.TrimSpace(item); item == "" {
			continue
		}
		if !principalID.MatchString(item) {
			return nil, fmt.Errorf("want comma-separated prn_ ids, got %q", item)
		}
		out = append(out, item)
	}
	return out, nil
}

// parsePrefixes reads comma-separated IP addresses and CIDR prefixes. An address is its own
// single-address prefix.
func parsePrefixes(s string) ([]netip.Prefix, error) {
	var out []netip.Prefix
	for _, item := range strings.Split(s, ",") {
		if item = strings.TrimSpace(item); item == "" {
			continue
		}
		if strings.Contains(item, "/") {
			p, err := netip.ParsePrefix(item)
			if err != nil {
				return nil, fmt.Errorf("want IP addresses or CIDR prefixes, got %q", item)
			}
			out = append(out, p.Masked())
			continue
		}
		a, err := netip.ParseAddr(item)
		if err != nil {
			return nil, fmt.Errorf("want IP addresses or CIDR prefixes, got %q", item)
		}
		a = a.Unmap()
		out = append(out, netip.PrefixFrom(a, a.BitLen()))
	}
	return out, nil
}
