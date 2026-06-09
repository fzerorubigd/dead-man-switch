package dmstate

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/go-github/v53/github"
)

const (
	// StateBranch is the orphan branch that holds the persisted state,
	// decoupled from the code on master and from the public page.
	StateBranch = "state"
	// StateFilePath is the state file on StateBranch.
	StateFilePath = "state.json"
)

// Load reads the state file from the state branch. It returns the parsed
// state, the file's blob SHA (needed to update it next time), and
// whether the file already existed. A missing branch or file yields a
// zero State with exists=false (first run).
func Load(ctx context.Context, gh *github.Client, owner, repo string) (st State, sha string, exists bool, err error) {
	fc, _, resp, err := gh.Repositories.GetContents(ctx, owner, repo, StateFilePath,
		&github.RepositoryContentGetOptions{Ref: StateBranch})
	if err != nil {
		if resp != nil && resp.StatusCode == 404 {
			return State{}, "", false, nil
		}
		return State{}, "", false, fmt.Errorf("load state: %w", err)
	}
	content, err := fc.GetContent()
	if err != nil {
		return State{}, "", false, fmt.Errorf("decode state content: %w", err)
	}
	if err := json.Unmarshal([]byte(content), &st); err != nil {
		return State{}, "", false, fmt.Errorf("parse state.json: %w", err)
	}
	return st, fc.GetSHA(), true, nil
}

// Save writes the state file to the state branch, creating the orphan
// branch on first use. sha is the prior blob SHA from Load ("" when the
// file/branch did not exist yet).
func Save(ctx context.Context, gh *github.Client, owner, repo string, st State, sha string) error {
	data, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal state: %w", err)
	}
	data = append(data, '\n')

	exists, err := branchExists(ctx, gh, owner, repo, StateBranch)
	if err != nil {
		return err
	}
	if !exists {
		return bootstrapStateBranch(ctx, gh, owner, repo, data)
	}

	opts := &github.RepositoryContentFileOptions{
		Message: github.String("aggregate: update dead-man-switch state"),
		Content: data,
		Branch:  github.String(StateBranch),
	}
	if sha == "" {
		_, _, err = gh.Repositories.CreateFile(ctx, owner, repo, StateFilePath, opts)
	} else {
		opts.SHA = github.String(sha)
		_, _, err = gh.Repositories.UpdateFile(ctx, owner, repo, StateFilePath, opts)
	}
	if err != nil {
		return fmt.Errorf("write state: %w", err)
	}
	return nil
}

func branchExists(ctx context.Context, gh *github.Client, owner, repo, branch string) (bool, error) {
	_, resp, err := gh.Git.GetRef(ctx, owner, repo, "refs/heads/"+branch)
	if err != nil {
		if resp != nil && resp.StatusCode == 404 {
			return false, nil
		}
		return false, fmt.Errorf("get ref %s: %w", branch, err)
	}
	return true, nil
}

// bootstrapStateBranch creates the orphan state branch with an initial
// state.json commit (no parents).
func bootstrapStateBranch(ctx context.Context, gh *github.Client, owner, repo string, data []byte) error {
	blob, _, err := gh.Git.CreateBlob(ctx, owner, repo, &github.Blob{
		Content:  github.String(string(data)),
		Encoding: github.String("utf-8"),
	})
	if err != nil {
		return fmt.Errorf("bootstrap blob: %w", err)
	}
	tree, _, err := gh.Git.CreateTree(ctx, owner, repo, "", []*github.TreeEntry{{
		Path: github.String(StateFilePath),
		Mode: github.String("100644"),
		Type: github.String("blob"),
		SHA:  blob.SHA,
	}})
	if err != nil {
		return fmt.Errorf("bootstrap tree: %w", err)
	}
	commit, _, err := gh.Git.CreateCommit(ctx, owner, repo, &github.Commit{
		Message: github.String("aggregate: initialize dead-man-switch state"),
		Tree:    tree,
	})
	if err != nil {
		return fmt.Errorf("bootstrap commit: %w", err)
	}
	_, _, err = gh.Git.CreateRef(ctx, owner, repo, &github.Reference{
		Ref:    github.String("refs/heads/" + StateBranch),
		Object: &github.GitObject{SHA: commit.SHA},
	})
	if err != nil {
		return fmt.Errorf("bootstrap ref: %w", err)
	}
	return nil
}
