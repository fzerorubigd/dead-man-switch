// Command statuspage renders the public alive-or-dead status page from
// the persisted dead-man-switch state and writes it (plus an optional
// CNAME) into an output directory. A workflow publishes that directory to
// the gh-pages branch on a weekly schedule.
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/go-github/v53/github"
	"golang.org/x/oauth2"

	"github.com/fzerorubigd/life-tracker/internal/dmstate"
)

func getEnvDefault(e, def string) string {
	if v := os.Getenv(e); v != "" {
		return v
	}
	return def
}

func main() {
	outDir := flag.String("out", "public", "output directory for the rendered page")
	flag.Parse()

	owner := getEnvDefault("REPOSITORY_OWNER", "fzerorubigd")
	repo := getEnvDefault("REPOSITORY_NAME", "life-tracker")
	domain := os.Getenv("STATUS_PAGE_DOMAIN")
	note := os.Getenv("STATUS_PAGE_NOTE")

	ctx := context.Background()
	gh := github.NewClient(oauth2.NewClient(ctx, oauth2.StaticTokenSource(
		&oauth2.Token{AccessToken: os.Getenv("GITHUB_TOKEN")},
	)))

	st, _, _, err := dmstate.Load(ctx, gh, owner, repo)
	if err != nil {
		log.Fatalf("load state: %v", err)
	}

	label, class, desc := statusView(st.Status)
	last := "unknown"
	if st.LastSignOfLife != nil {
		last = st.LastSignOfLife.Format(time.RFC1123)
	}

	html, err := RenderPage(PageData{
		Label:          label,
		Class:          class,
		Description:    desc,
		LastSignOfLife: last,
		Note:           note,
		Domain:         domain,
		FallbackURL:    fmt.Sprintf("https://%s.github.io/%s/", strings.ToLower(owner), repo),
		GeneratedAt:    time.Now().UTC().Format(time.RFC1123),
	})
	if err != nil {
		log.Fatalf("render: %v", err)
	}

	if err := os.MkdirAll(*outDir, 0o755); err != nil {
		log.Fatalf("mkdir %s: %v", *outDir, err)
	}
	if err := os.WriteFile(filepath.Join(*outDir, "index.html"), []byte(html), 0o644); err != nil {
		log.Fatalf("write index.html: %v", err)
	}
	// A custom domain needs a CNAME file in the published tree; without a
	// configured domain the page is served from the gh-pages default URL.
	if domain != "" {
		if err := os.WriteFile(filepath.Join(*outDir, "CNAME"), []byte(domain+"\n"), 0o644); err != nil {
			log.Fatalf("write CNAME: %v", err)
		}
	}
	log.Printf("rendered status=%s -> %s (domain=%q)", st.Status, *outDir, domain)
}
