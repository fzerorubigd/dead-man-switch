package main

import (
	"context"
	"log"
	"os"
	"regexp"
	"strconv"
	"time"

	"github.com/google/go-github/v53/github"
	"golang.org/x/oauth2"

	"github.com/fzerorubigd/life-tracker/internal/dmstate"
)

// wfPattern selects the life-signal workflows: every workflow whose file
// is named "*-check.yaml". Infra workflows (e.g. crypt-sanity.yaml) are
// deliberately not named that way so they are not counted as signals.
var wfPattern = regexp.MustCompile("-check.yaml$")

func getEnvDefault(e, def string) string {
	if v := os.Getenv(e); v != "" {
		return v
	}
	return def
}

func envIntDefault(e string, def int) int {
	if v := os.Getenv(e); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return def
}

func configFromEnv() Config {
	d := DefaultConfig()
	return Config{
		ThresholdFailsDays: envIntDefault("THRESHOLD_FAILS_DAYS", d.ThresholdFailsDays),
		WaitingPeriodDays:  envIntDefault("WAITING_PERIOD_DAYS", d.WaitingPeriodDays),
	}
}

// collectSignalRuns reads every life-signal workflow's run history and
// returns one SignalRun per recorded run.
func collectSignalRuns(ctx context.Context, gh *github.Client, owner, repo string) ([]SignalRun, error) {
	wfs, _, err := gh.Actions.ListWorkflows(ctx, owner, repo, nil)
	if err != nil {
		return nil, err
	}
	var out []SignalRun
	for _, wf := range wfs.Workflows {
		if !wfPattern.MatchString(wf.GetPath()) {
			continue
		}
		runs, _, err := gh.Actions.ListWorkflowRunsByID(ctx, owner, repo, wf.GetID(), nil)
		if err != nil {
			return nil, err
		}
		for _, r := range runs.WorkflowRuns {
			out = append(out, SignalRun{
				Conclusion: r.GetConclusion(),
				At:         r.GetCreatedAt().Time,
			})
		}
	}
	return out, nil
}

func main() {
	owner := getEnvDefault("REPOSITORY_OWNER", "fzerorubigd")
	repo := getEnvDefault("REPOSITORY_NAME", "life-tracker")
	reset := os.Getenv("RESET") == "true"
	cfg := configFromEnv()

	ctx := context.Background()
	gh := github.NewClient(oauth2.NewClient(ctx, oauth2.StaticTokenSource(
		&oauth2.Token{AccessToken: os.Getenv("GITHUB_TOKEN")},
	)))

	now := time.Now().UTC()

	runs, err := collectSignalRuns(ctx, gh, owner, repo)
	if err != nil {
		log.Fatalf("collect signal runs: %v", err)
	}
	last := lastSignOfLife(runs)
	isAlive := alive(last, now, cfg)

	prev, sha, _, err := dmstate.Load(ctx, gh, owner, repo)
	if err != nil {
		log.Fatalf("load state: %v", err)
	}

	next := Transition(prev, isAlive, reset, now, cfg)
	if !last.IsZero() {
		next.LastSignOfLife = &last
	}

	if err := dmstate.Save(ctx, gh, owner, repo, next, sha); err != nil {
		log.Fatalf("save state: %v", err)
	}

	lastStr := "never"
	if !last.IsZero() {
		lastStr = last.Format(time.RFC3339)
	}
	log.Printf("status=%s alive=%v last_sign_of_life=%s reset=%v threshold=%dd waiting=%dd",
		next.Status, isAlive, lastStr, reset, cfg.ThresholdFailsDays, cfg.WaitingPeriodDays)
	if next.Status == dmstate.StatusTriggerReady {
		log.Printf("TRIGGER-READY: waiting-period elapsed with no sign of life; the trigger layer may fire")
	}
}
