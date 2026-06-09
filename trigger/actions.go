package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// defaultActionsFile is the committed death-action list (non-secret: it
// names handlers + their encrypted payload files). It lives at the repo
// root so payload paths inside it resolve against that same root: the
// file's directory is the anchor for everything it references.
const defaultActionsFile = "actions.json"

// loadActions reads the death-action list from path. A missing file is an
// empty list (no actions configured yet), not an error.
//
// Each entry's payload_file is resolved relative to the directory holding
// actions.json, so the file and the blobs it references travel together.
// Absolute payload paths are passed through unchanged.
func loadActions(path string) ([]Action, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read actions %s: %w", path, err)
	}
	var actions []Action
	if err := json.Unmarshal(b, &actions); err != nil {
		return nil, fmt.Errorf("parse actions %s: %w", path, err)
	}
	base := filepath.Dir(path)
	for i, a := range actions {
		if a.PayloadFile == "" || filepath.IsAbs(a.PayloadFile) {
			continue
		}
		actions[i].PayloadFile = filepath.Join(base, a.PayloadFile)
	}
	return actions, nil
}
