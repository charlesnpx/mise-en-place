package artifact

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

// FetchTimeout is the upper bound on a single artifact fetch.
const FetchTimeout = 60 * time.Second

// Fetch reads an artifact from a source URL or a local:<path> reference and
// returns its raw bytes. The caller verifies the sha256.
//
// Sources accepted:
//   - https://...                         → HTTP GET
//   - local:/absolute/path/to/file.tgz    → file read
//   - file:///absolute/path/to/file.tgz   → file read
func Fetch(source string) ([]byte, error) {
	switch {
	case strings.HasPrefix(source, "local:"):
		return os.ReadFile(strings.TrimPrefix(source, "local:"))
	case strings.HasPrefix(source, "file://"):
		return os.ReadFile(strings.TrimPrefix(source, "file://"))
	case strings.HasPrefix(source, "http://"), strings.HasPrefix(source, "https://"):
		return fetchHTTP(source)
	default:
		return nil, fmt.Errorf("unsupported source scheme: %s", source)
	}
}

func fetchHTTP(url string) ([]byte, error) {
	client := &http.Client{Timeout: FetchTimeout}
	resp, err := client.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		return nil, fmt.Errorf("fetch %s: HTTP %d", url, resp.StatusCode)
	}
	return io.ReadAll(resp.Body)
}
