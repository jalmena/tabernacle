package config

import (
	"fmt"
	"log/slog"
	"strings"
	"testing"
	"time"
)

// env builds a getenv function from a map, so no test touches the real
// environment and tests can run in parallel.
func env(m map[string]string) func(string) string {
	return func(k string) string { return m[k] }
}

func TestLoadDefaults(t *testing.T) {
	c, err := Load(env(nil))
	if err != nil {
		t.Fatalf("Load with an empty environment: %v", err)
	}
	if c.Addr != ":8080" {
		t.Errorf("Addr = %q, want :8080", c.Addr)
	}
	if c.DataDir != "/data" {
		t.Errorf("DataDir = %q, want /data", c.DataDir)
	}
	if c.SongsDir != "/data/songs" {
		t.Errorf("SongsDir = %q, want /data/songs", c.SongsDir)
	}
	if c.TrashDir != "/data/trash" {
		t.Errorf("TrashDir = %q, want /data/trash", c.TrashDir)
	}
	if c.DBPath != "/data/index.db" {
		t.Errorf("DBPath = %q, want /data/index.db", c.DBPath)
	}
	if c.BaseURL == nil || c.BaseURL.String() != "http://localhost:8080" {
		t.Errorf("BaseURL = %v, want http://localhost:8080", c.BaseURL)
	}
	if len(c.AllowedHosts) != 0 {
		t.Errorf("AllowedHosts = %v, want empty so the LAN policy applies", c.AllowedHosts)
	}
	if c.ScanInterval != time.Minute {
		t.Errorf("ScanInterval = %v, want 1m", c.ScanInterval)
	}
	if c.MaxUploadBytes != 32<<20 {
		t.Errorf("MaxUploadBytes = %d, want %d", c.MaxUploadBytes, 32<<20)
	}
	if c.MaxSongBytes != 1<<20 {
		t.Errorf("MaxSongBytes = %d, want %d", c.MaxSongBytes, 1<<20)
	}
	if c.Notation != NotationEnglish {
		t.Errorf("Notation = %q, want %q", c.Notation, NotationEnglish)
	}
	if c.ReadOnly {
		t.Error("ReadOnly = true, want false by default")
	}
	if c.LogLevel != slog.LevelInfo {
		t.Errorf("LogLevel = %v, want info", c.LogLevel)
	}
}

func TestLoadDerivedPathsFollowDataDir(t *testing.T) {
	c, err := Load(env(map[string]string{"TABERNACLE_DATA_DIR": "/srv/tab"}))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	for _, tc := range []struct{ got, want string }{
		{c.SongsDir, "/srv/tab/songs"},
		{c.TrashDir, "/srv/tab/trash"},
		{c.DBPath, "/srv/tab/index.db"},
	} {
		if tc.got != tc.want {
			t.Errorf("derived path = %q, want %q", tc.got, tc.want)
		}
	}
}

func TestLoadOverrides(t *testing.T) {
	c, err := Load(env(map[string]string{
		"TABERNACLE_ADDR":             "127.0.0.1:9000",
		"TABERNACLE_DATA_DIR":         "/srv/tab/",
		"TABERNACLE_SONGS_DIR":        "/music/collection",
		"TABERNACLE_TRASH_DIR":        "/music/trash",
		"TABERNACLE_DB_PATH":          "/var/lib/tab/index.db",
		"TABERNACLE_BASE_URL":         "https://songs.example.com/",
		"TABERNACLE_ALLOWED_HOSTS":    "songs.example.com, Tab.local ,",
		"TABERNACLE_SCAN_INTERVAL":    "0",
		"TABERNACLE_MAX_UPLOAD_BYTES": "1000000",
		"TABERNACLE_MAX_SONG_BYTES":   "5000",
		"TABERNACLE_NOTATION":         "Solfege",
		"TABERNACLE_READ_ONLY":        "true",
		"TABERNACLE_LOG_LEVEL":        "DEBUG",
	}))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if c.Addr != "127.0.0.1:9000" {
		t.Errorf("Addr = %q", c.Addr)
	}
	if c.DataDir != "/srv/tab" {
		t.Errorf("DataDir = %q, want the trailing slash cleaned", c.DataDir)
	}
	if c.SongsDir != "/music/collection" {
		t.Errorf("SongsDir = %q", c.SongsDir)
	}
	if c.BaseURL.String() != "https://songs.example.com" {
		t.Errorf("BaseURL = %q, want the trailing slash trimmed", c.BaseURL)
	}
	want := []string{"songs.example.com", "tab.local"}
	if len(c.AllowedHosts) != len(want) {
		t.Fatalf("AllowedHosts = %v, want %v", c.AllowedHosts, want)
	}
	for i := range want {
		if c.AllowedHosts[i] != want[i] {
			t.Errorf("AllowedHosts[%d] = %q, want %q", i, c.AllowedHosts[i], want[i])
		}
	}
	if c.ScanInterval != 0 {
		t.Errorf("ScanInterval = %v, want 0 to disable the ticker", c.ScanInterval)
	}
	if c.Notation != NotationSolfege {
		t.Errorf("Notation = %q, want the value lowercased", c.Notation)
	}
	if !c.ReadOnly {
		t.Error("ReadOnly = false, want true")
	}
	if c.LogLevel != slog.LevelDebug {
		t.Errorf("LogLevel = %v, want debug", c.LogLevel)
	}
}

func TestLoadRejects(t *testing.T) {
	tests := []struct {
		name string
		envs map[string]string
		want string // substring the error must mention
	}{
		{"notation", map[string]string{"TABERNACLE_NOTATION": "spanish"}, "TABERNACLE_NOTATION"},
		{"base url scheme", map[string]string{"TABERNACLE_BASE_URL": "ftp://x/"}, "http or https"},
		{"base url no host", map[string]string{"TABERNACLE_BASE_URL": "http:///songs"}, "must include a host"},
		{"scan interval text", map[string]string{"TABERNACLE_SCAN_INTERVAL": "soon"}, "not a duration"},
		{"scan interval negative", map[string]string{"TABERNACLE_SCAN_INTERVAL": "-5s"}, "must not be negative"},
		{"scan interval tiny", map[string]string{"TABERNACLE_SCAN_INTERVAL": "10ms"}, "too short"},
		{"upload not a number", map[string]string{"TABERNACLE_MAX_UPLOAD_BYTES": "lots"}, "whole number of bytes"},
		{"upload zero", map[string]string{"TABERNACLE_MAX_UPLOAD_BYTES": "0"}, "greater than zero"},
		{"upload negative", map[string]string{"TABERNACLE_MAX_SONG_BYTES": "-1"}, "greater than zero"},
		{"song larger than upload", map[string]string{
			"TABERNACLE_MAX_UPLOAD_BYTES": "1000",
			"TABERNACLE_MAX_SONG_BYTES":   "2000",
		}, "must not exceed"},
		{"read only", map[string]string{"TABERNACLE_READ_ONLY": "sure"}, "use true or false"},
		{"log level", map[string]string{"TABERNACLE_LOG_LEVEL": "chatty"}, "debug, info, warn or error"},
		{"songs dir relative", map[string]string{"TABERNACLE_SONGS_DIR": "songs"}, "absolute path"},
		{"db inside songs", map[string]string{
			"TABERNACLE_SONGS_DIR": "/music",
			"TABERNACLE_DB_PATH":   "/music/index.db",
		}, "must not be inside the songs directory"},
		{"trash inside songs", map[string]string{
			"TABERNACLE_SONGS_DIR": "/music",
			"TABERNACLE_TRASH_DIR": "/music/trash",
		}, "must not be inside the songs directory"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Load(env(tt.envs))
			if err == nil {
				t.Fatalf("Load(%v) succeeded, want an error", tt.envs)
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Errorf("error = %q, want it to mention %q", err, tt.want)
			}
		})
	}
}

// A misconfigured deployment should learn about all of its mistakes at once.
func TestLoadReportsEveryProblem(t *testing.T) {
	_, err := Load(env(map[string]string{
		"TABERNACLE_NOTATION":  "spanish",
		"TABERNACLE_READ_ONLY": "sure",
		"TABERNACLE_LOG_LEVEL": "chatty",
	}))
	if err == nil {
		t.Fatal("Load succeeded, want errors")
	}
	for _, want := range []string{"TABERNACLE_NOTATION", "true or false", "debug, info"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q is missing %q", err, want)
		}
	}
}

func TestHostAllowedDefaultPolicy(t *testing.T) {
	c, err := Load(env(nil)) // BaseURL http://localhost:8080, no explicit allowlist
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	tests := []struct {
		host string
		want bool
	}{
		// A home server legitimately answers to these.
		{"localhost", true},
		{"localhost:8080", true},
		{"127.0.0.1", true},
		{"127.0.0.1:8080", true},
		{"[::1]:8080", true},
		{"192.168.1.50:8080", true},
		{"10.0.0.7", true},
		{"172.16.4.1", true},
		{"169.254.10.10", true},
		{"nas", true}, // bare hostname cannot come from public DNS
		{"tabernacle.local", true},
		{"nas.lan", true},
		{"box.home.arpa", true},
		{"server.internal", true},

		// The shape a DNS rebinding attack must take: a public name pointed at a
		// private address. Refusing these is the entire point.
		{"attacker.com", false},
		{"tabernacle.attacker.com", false},
		{"songs.example.com:8080", false},
		{"8.8.8.8", false},
		{"", false},
	}
	for _, tt := range tests {
		if got := c.HostAllowed(tt.host); got != tt.want {
			t.Errorf("HostAllowed(%q) = %v, want %v", tt.host, got, tt.want)
		}
	}
}

func TestHostAllowedExplicitList(t *testing.T) {
	c, err := Load(env(map[string]string{
		"TABERNACLE_BASE_URL":      "https://songs.example.com",
		"TABERNACLE_ALLOWED_HOSTS": "songs.example.com,tab.local",
	}))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	tests := []struct {
		host string
		want bool
	}{
		{"songs.example.com", true},
		{"songs.example.com:443", true},
		{"SONGS.EXAMPLE.COM", true},
		{"tab.local", true},
		// An explicit allowlist means exactly that: the permissive LAN policy no
		// longer applies, so a private address is not automatically fine.
		{"192.168.1.50:8080", false},
		{"localhost", false},
		{"attacker.com", false},
	}
	for _, tt := range tests {
		if got := c.HostAllowed(tt.host); got != tt.want {
			t.Errorf("HostAllowed(%q) = %v, want %v", tt.host, got, tt.want)
		}
	}
}

func TestAccountsFileDefaultsAndValidation(t *testing.T) {
	c, err := Load(env(map[string]string{"TABERNACLE_DATA_DIR": "/data"}))
	if err != nil {
		t.Fatal(err)
	}
	if c.AccountsFile != "accounts.json" {
		t.Errorf("AccountsFile = %q", c.AccountsFile)
	}

	// A path would let the file land in songs/, where a scan would walk it and a
	// reader browsing their own files would find password hashes.
	for _, bad := range []string{"songs/accounts.json", "../accounts.json", "/etc/passwd", `a\b`, "."} {
		_, err := Load(env(map[string]string{
			"TABERNACLE_DATA_DIR":      "/data",
			"TABERNACLE_ACCOUNTS_FILE": bad,
		}))
		if err == nil {
			t.Errorf("TABERNACLE_ACCOUNTS_FILE=%q was accepted", bad)
			continue
		}
		if !strings.Contains(err.Error(), "TABERNACLE_ACCOUNTS_FILE") {
			t.Errorf("the error for %q does not name the variable: %v", bad, err)
		}
	}
}

// The bootstrap pair is useless by halves, so half of it is a configuration
// error rather than something silently ignored.
func TestTheBootstrapAdminNeedsBothHalves(t *testing.T) {
	tests := []struct {
		name string
		env  map[string]string
		ok   bool
	}{
		{"neither", map[string]string{}, true},
		{"both", map[string]string{"TABERNACLE_ADMIN_USER": "jose", "TABERNACLE_ADMIN_PASSWORD": "una contraseña larga"}, true},
		{"user only", map[string]string{"TABERNACLE_ADMIN_USER": "jose"}, false},
		{"password only", map[string]string{"TABERNACLE_ADMIN_PASSWORD": "una contraseña larga"}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			vars := map[string]string{"TABERNACLE_DATA_DIR": "/data"}
			for k, v := range tt.env {
				vars[k] = v
			}
			_, err := Load(env(vars))
			if tt.ok && err != nil {
				t.Errorf("Load: %v", err)
			}
			if !tt.ok {
				if err == nil {
					t.Fatal("the incomplete pair was accepted")
				}
				if !strings.Contains(err.Error(), "TABERNACLE_ADMIN_") {
					t.Errorf("the error does not name a variable: %v", err)
				}
			}
		})
	}
}

// A password that can only be printed on purpose.
func TestASecretDoesNotPrintItself(t *testing.T) {
	c, err := Load(env(map[string]string{
		"TABERNACLE_DATA_DIR":       "/data",
		"TABERNACLE_ADMIN_USER":     "jose",
		"TABERNACLE_ADMIN_PASSWORD": "correcthorsebattery",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if c.AdminPassword.Reveal() != "correcthorsebattery" {
		t.Fatalf("Reveal = %q", c.AdminPassword.Reveal())
	}

	for _, rendered := range []string{
		fmt.Sprintf("%v", c.AdminPassword),
		fmt.Sprintf("%s", c.AdminPassword),
		fmt.Sprint(c.AdminPassword),
		c.AdminPassword.String(),
		c.AdminPassword.LogValue().String(),
		// The whole config, which is the accident this guards against.
		fmt.Sprintf("%v", c),
		fmt.Sprintf("%+v", c),
	} {
		if strings.Contains(rendered, "correcthorsebattery") {
			t.Errorf("the password appears in %q", rendered)
		}
	}
}
