package daemon

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"time"

	"zeroclaw/internal/config"
)

// Info is the client's handle to a running zeroclawd: loopback port, bearer
// token, and pid, written to ~/.zeroclaw/daemon.json (0600). Loopback HTTP
// with a random token is the prototype control plane; a unix socket / named
// pipe can replace it later without touching clients beyond this package.
type Info struct {
	Port  int    `json:"port"`
	Token string `json:"token"`
	PID   int    `json:"pid"`
}

func infoPath() (string, error) { return config.Path("daemon.json") }

func saveInfo(i Info) error {
	p, err := infoPath()
	if err != nil {
		return err
	}
	data, err := json.Marshal(i)
	if err != nil {
		return err
	}
	return os.WriteFile(p, data, 0o600)
}

func LoadInfo() (Info, error) {
	p, err := infoPath()
	if err != nil {
		return Info{}, err
	}
	data, err := os.ReadFile(p)
	if err != nil {
		return Info{}, fmt.Errorf("no daemon handle at %s; run `zeroclaw up`", p)
	}
	var i Info
	if err := json.Unmarshal(data, &i); err != nil {
		return Info{}, err
	}
	return i, nil
}

func removeInfo() {
	if p, err := infoPath(); err == nil {
		os.Remove(p)
	}
}

func (i Info) url(path string) string {
	return fmt.Sprintf("http://127.0.0.1:%d%s", i.Port, path)
}

// Ping reports whether the daemon described by i is alive and ours.
func (i Info) Ping() bool {
	req, err := http.NewRequest(http.MethodGet, i.url("/status"), nil)
	if err != nil {
		return false
	}
	req.Header.Set("Authorization", "Bearer "+i.Token)
	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}

// Running loads the handle and pings it.
func Running() (Info, bool) {
	i, err := LoadInfo()
	if err != nil {
		return Info{}, false
	}
	return i, i.Ping()
}
