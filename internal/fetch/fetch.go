// Package fetch holds shared HTTP fetch helpers for the life-signal CLIs.
package fetch

import (
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"time"
)

// XML GETs url with a 10s timeout and unmarshals the XML response body
// into data.
func XML(ctx context.Context, url string, data interface{}) error {
	var cnl context.CancelFunc
	ctx, cnl = context.WithTimeout(ctx, 10*time.Second)
	defer cnl()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < http.StatusOK || resp.StatusCode > http.StatusIMUsed {
		return fmt.Errorf("expected 2xx received %q", resp.StatusCode)
	}
	content, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	return xml.Unmarshal(content, data)
}
