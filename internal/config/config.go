// Package config turns the process environment into a validated Config.
//
// It imports nothing else from this project, and Load takes a getenv function
// rather than reading os.Getenv directly, so the whole surface is table-testable
// without touching the real environment.
package config

import (
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// Notation selects the default note names for a reader who has expressed no
// preference. Stored as a string because config has no business importing the
// chord package.
const (
	NotationEnglish = "english"
	NotationSolfege = "solfege"
)

// Config is the whole of Tabernacle's configuration.
type Config struct {
	Addr     string // listen address, for example ":8080"
	DataDir  string // holds songs, trash and the index unless overridden
	SongsDir string // the collection; the only directory ever written to
	TrashDir string // deleted songs land here rather than being unlinked
	DBPath   string // the disposable index

	// AccountsFile is the name of the accounts file inside DataDir. It holds
	// password hashes and the key that signs every session, so unlike the index
	// it cannot be regenerated and does need backing up.
	AccountsFile string
	// AdminUser and AdminPassword create or reset the first administrator at
	// startup. They are read once and never stored.
	AdminUser     string
	AdminPassword Secret
	// PasswordCost is the PBKDF2 iteration count for new passwords. Zero means
	// the production figure, which is resolved where the hashing lives — this
	// package deliberately imports nothing else from the project.
	//
	// It is not read from the environment: it is a security parameter, not a
	// preference. The end-to-end tests lower it so that logging in is not the
	// slowest thing in the suite.
	PasswordCost int

	BaseURL      *url.URL // canonical URL, used for absolute links and the host allowlist
	AllowedHosts []string // exact Host values to accept; empty means the LAN policy below

	ScanInterval   time.Duration // periodic rescan; zero disables it
	MaxUploadBytes int64         // cap on a whole import request
	MaxSongBytes   int64         // cap on one .song file

	Notation string // NotationEnglish or NotationSolfege
	ReadOnly bool   // when true every mutating route refuses
	LogLevel slog.Level
}

// Secret is a configured value that must not appear in a log line.
//
// The application has one place that logs configuration and it names its fields
// by hand, so this is belt and braces — but a password that can only be printed
// on purpose is worth eight lines.
type Secret string

// String hides the value from fmt, which covers %v and %s.
func (Secret) String() string { return "[redacted]" }

// LogValue hides it from slog, which does not go through String.
func (Secret) LogValue() slog.Value { return slog.StringValue("[redacted]") }

// Reveal is the only way to read a secret, and it reads as what it is at the call
// site.
func (s Secret) Reveal() string { return string(s) }

// Defaults, chosen so that a bare `docker run -v ...:/data` works.
const (
	defaultAccountsFile   = "accounts.json"
	defaultAddr           = ":8080"
	defaultDataDir        = "/data"
	defaultBaseURL        = "http://localhost:8080"
	defaultScanInterval   = time.Minute
	defaultMaxUploadBytes = 32 << 20 // 32 MiB
	defaultMaxSongBytes   = 1 << 20  // 1 MiB
)

// Load reads and validates the configuration.
//
// Every problem is collected before returning, so a misconfigured deployment
// reports all of its mistakes at once instead of one per restart.
func Load(getenv func(string) string) (Config, error) {
	var errs []error
	fail := func(format string, a ...any) { errs = append(errs, fmt.Errorf(format, a...)) }

	get := func(key, def string) string {
		if v := strings.TrimSpace(getenv(key)); v != "" {
			return v
		}
		return def
	}

	c := Config{
		Addr:     get("TABERNACLE_ADDR", defaultAddr),
		DataDir:  get("TABERNACLE_DATA_DIR", defaultDataDir),
		Notation: strings.ToLower(get("TABERNACLE_NOTATION", NotationEnglish)),
	}

	if !filepath.IsAbs(c.DataDir) {
		abs, err := filepath.Abs(c.DataDir)
		if err != nil {
			fail("TABERNACLE_DATA_DIR %q: %v", c.DataDir, err)
		}
		c.DataDir = abs
	}
	c.DataDir = filepath.Clean(c.DataDir)

	c.SongsDir = filepath.Clean(get("TABERNACLE_SONGS_DIR", filepath.Join(c.DataDir, "songs")))
	c.TrashDir = filepath.Clean(get("TABERNACLE_TRASH_DIR", filepath.Join(c.DataDir, "trash")))
	c.DBPath = filepath.Clean(get("TABERNACLE_DB_PATH", filepath.Join(c.DataDir, "index.db")))

	if !filepath.IsAbs(c.SongsDir) {
		fail("TABERNACLE_SONGS_DIR must be an absolute path, got %q", c.SongsDir)
	}
	// The index must not live inside the collection: a scan would then have to
	// know how to skip it, and a stray *.db in songs/ is a confusing thing to
	// hand a user who is browsing their own files.
	if within(c.SongsDir, c.DBPath) {
		fail("TABERNACLE_DB_PATH %q must not be inside the songs directory %q", c.DBPath, c.SongsDir)
	}
	if within(c.SongsDir, c.TrashDir) {
		fail("TABERNACLE_TRASH_DIR %q must not be inside the songs directory %q", c.TrashDir, c.SongsDir)
	}

	// The accounts file is a plain name inside the data directory. Allowing a
	// path would let it land in songs/, where a scan would walk it and a reader
	// browsing their own files would find password hashes.
	//
	// internal/account refuses the same things as its own last line of defence;
	// this check exists to report the mistake at startup, by name, alongside every
	// other one.
	c.AccountsFile = get("TABERNACLE_ACCOUNTS_FILE", defaultAccountsFile)
	if c.AccountsFile != filepath.Base(c.AccountsFile) ||
		strings.ContainsAny(c.AccountsFile, `/\`) ||
		c.AccountsFile == "." || c.AccountsFile == ".." {
		fail("TABERNACLE_ACCOUNTS_FILE must be a filename, not a path, got %q", c.AccountsFile)
	}

	c.AdminUser = strings.TrimSpace(getenv("TABERNACLE_ADMIN_USER"))
	c.AdminPassword = Secret(getenv("TABERNACLE_ADMIN_PASSWORD"))
	switch {
	case c.AdminUser == "" && c.AdminPassword != "":
		fail("TABERNACLE_ADMIN_PASSWORD is set but TABERNACLE_ADMIN_USER is not")
	case c.AdminUser != "" && c.AdminPassword == "":
		fail("TABERNACLE_ADMIN_USER is set but TABERNACLE_ADMIN_PASSWORD is not")
	}

	switch c.Notation {
	case NotationEnglish, NotationSolfege:
	default:
		fail("TABERNACLE_NOTATION must be %q or %q, got %q", NotationEnglish, NotationSolfege, c.Notation)
	}

	raw := get("TABERNACLE_BASE_URL", defaultBaseURL)
	u, err := url.Parse(raw)
	switch {
	case err != nil:
		fail("TABERNACLE_BASE_URL %q: %v", raw, err)
	case u.Scheme != "http" && u.Scheme != "https":
		fail("TABERNACLE_BASE_URL %q must be an http or https URL", raw)
	case u.Host == "":
		fail("TABERNACLE_BASE_URL %q must include a host", raw)
	default:
		u.Path = strings.TrimSuffix(u.Path, "/")
		c.BaseURL = u
	}

	if v := strings.TrimSpace(getenv("TABERNACLE_ALLOWED_HOSTS")); v != "" {
		for _, h := range strings.Split(v, ",") {
			if h = strings.TrimSpace(strings.ToLower(h)); h != "" {
				c.AllowedHosts = append(c.AllowedHosts, h)
			}
		}
	}

	c.ScanInterval = defaultScanInterval
	if v := strings.TrimSpace(getenv("TABERNACLE_SCAN_INTERVAL")); v != "" {
		d, err := time.ParseDuration(v)
		switch {
		case err != nil:
			fail("TABERNACLE_SCAN_INTERVAL %q: not a duration such as 60s or 5m", v)
		case d < 0:
			fail("TABERNACLE_SCAN_INTERVAL %q must not be negative", v)
		case d > 0 && d < time.Second:
			fail("TABERNACLE_SCAN_INTERVAL %q is too short; use 1s or more, or 0 to disable", v)
		default:
			c.ScanInterval = d
		}
	}

	c.MaxUploadBytes = positiveBytes(getenv, "TABERNACLE_MAX_UPLOAD_BYTES", defaultMaxUploadBytes, fail)
	c.MaxSongBytes = positiveBytes(getenv, "TABERNACLE_MAX_SONG_BYTES", defaultMaxSongBytes, fail)
	if c.MaxSongBytes > c.MaxUploadBytes {
		fail("TABERNACLE_MAX_SONG_BYTES (%d) must not exceed TABERNACLE_MAX_UPLOAD_BYTES (%d)",
			c.MaxSongBytes, c.MaxUploadBytes)
	}

	if v := strings.TrimSpace(getenv("TABERNACLE_READ_ONLY")); v != "" {
		b, err := strconv.ParseBool(v)
		if err != nil {
			fail("TABERNACLE_READ_ONLY %q: use true or false", v)
		}
		c.ReadOnly = b
	}

	switch strings.ToLower(get("TABERNACLE_LOG_LEVEL", "info")) {
	case "debug":
		c.LogLevel = slog.LevelDebug
	case "info":
		c.LogLevel = slog.LevelInfo
	case "warn", "warning":
		c.LogLevel = slog.LevelWarn
	case "error":
		c.LogLevel = slog.LevelError
	default:
		fail("TABERNACLE_LOG_LEVEL %q: use debug, info, warn or error",
			getenv("TABERNACLE_LOG_LEVEL"))
	}

	if len(errs) > 0 {
		return Config{}, errors.Join(errs...)
	}
	return c, nil
}

func positiveBytes(getenv func(string) string, key string, def int64, fail func(string, ...any)) int64 {
	v := strings.TrimSpace(getenv(key))
	if v == "" {
		return def
	}
	n, err := strconv.ParseInt(v, 10, 64)
	switch {
	case err != nil:
		fail("%s %q: not a whole number of bytes", key, v)
	case n <= 0:
		fail("%s must be greater than zero, got %d", key, n)
	default:
		return n
	}
	return def
}

// within reports whether path is dir itself or lies beneath it.
func within(dir, path string) bool {
	rel, err := filepath.Rel(dir, path)
	if err != nil {
		return false
	}
	return rel == "." || (!strings.HasPrefix(rel, ".."+string(filepath.Separator)) && rel != "..")
}

// HostAllowed reports whether a request's Host header may be served.
//
// This is the defence against DNS rebinding, which is the real attack on an
// unauthenticated service on a home network: a page on attacker.com whose
// hostname resolves to 192.168.1.50 reaches this app as a same-origin request
// from the attacker's own origin, so checking Origin alone proves nothing.
//
// With no explicit allowlist the policy accepts what a home server legitimately
// answers to — loopback and private IP literals, and names that cannot be
// resolved from the public internet — and refuses public DNS names, which is
// precisely the shape a rebinding attack must take. That keeps a NAS or CasaOS
// install working with no configuration while still closing the hole. Anyone
// serving a real domain through a reverse proxy sets TABERNACLE_ALLOWED_HOSTS
// and gets an exact allowlist instead.
func (c Config) HostAllowed(host string) bool {
	h := strings.ToLower(strings.TrimSpace(host))
	if h == "" {
		return false
	}
	// Strip the port, tolerating IPv6 literals such as [::1]:8080.
	if hostOnly, _, err := net.SplitHostPort(h); err == nil {
		h = hostOnly
	}
	h = strings.TrimSuffix(strings.TrimPrefix(h, "["), "]")
	if h == "" {
		return false
	}

	if len(c.AllowedHosts) > 0 {
		for _, a := range c.AllowedHosts {
			if h == a {
				return true
			}
		}
		return c.BaseURL != nil && h == baseHost(c.BaseURL)
	}

	if c.BaseURL != nil && h == baseHost(c.BaseURL) {
		return true
	}
	if ip := net.ParseIP(h); ip != nil {
		return ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsUnspecified()
	}
	if h == "localhost" || !strings.Contains(h, ".") {
		return true // a bare hostname cannot come from public DNS
	}
	for _, suffix := range []string{".local", ".lan", ".home", ".home.arpa", ".internal", ".localdomain"} {
		if strings.HasSuffix(h, suffix) {
			return true
		}
	}
	return false
}

func baseHost(u *url.URL) string {
	h := u.Hostname()
	return strings.ToLower(strings.TrimSuffix(strings.TrimPrefix(h, "["), "]"))
}
