// SPDX-License-Identifier: AGPL-3.0-or-later
package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"
)

func NewSourcesCmd() *cobra.Command {
	defaultServer := os.Getenv("FLWD_API")
	if strings.TrimSpace(defaultServer) == "" {
		defaultServer = "http://127.0.0.1:8080"
	}
	cmd := &cobra.Command{
		Use:   ":sources",
		Short: "Manage configuration sources via the Runner API",
	}
	cmd.PersistentFlags().String("server", defaultServer, "Runner API base URL (or set FLWD_API)")
	cmd.PersistentFlags().String("token", os.Getenv("FLWD_TOKEN"), "Bearer token for Runner API (or set FLWD_TOKEN)")
	cmd.AddCommand(newSourcesListCmd())
	cmd.AddCommand(newSourcesAddCmd())
	cmd.AddCommand(newSourcesRemoveCmd())
	return cmd
}

type sourcesClient struct {
	base       string
	token      string
	httpClient *http.Client
}

func resolveSourcesClient(cmd *cobra.Command) (*sourcesClient, error) {
	server, err := cmd.InheritedFlags().GetString("server")
	if err != nil {
		return nil, err
	}
	token, err := cmd.InheritedFlags().GetString("token")
	if err != nil {
		return nil, err
	}
	base := normalizeBaseURL(server)
	return &sourcesClient{
		base:       base,
		token:      strings.TrimSpace(token),
		httpClient: &http.Client{Timeout: 15 * time.Second},
	}, nil
}

func (c *sourcesClient) do(ctx context.Context, method, path string, body []byte) (*http.Response, error) {
	endpoint := c.base + path
	var reader io.Reader
	if len(body) > 0 {
		reader = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, endpoint, reader)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	if len(body) > 0 {
		req.Header.Set("Content-Type", "application/json")
	}
	if c.token != "" {
		req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", c.token))
	}
	return c.httpClient.Do(req)
}

func newSourcesListCmd() *cobra.Command {
	var jsonOut bool
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List configured sources from the Runner API",
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := resolveSourcesClient(cmd)
			if err != nil {
				return err
			}
			resp, err := client.do(cmd.Context(), http.MethodGet, "/sources", nil)
			if err != nil {
				return err
			}
			defer resp.Body.Close()
			if resp.StatusCode != http.StatusOK {
				return apiError(resp)
			}
			var payload []apiSource
			if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
				return err
			}
			sort.Slice(payload, func(i, j int) bool { return payload[i].Name < payload[j].Name })
			if jsonOut {
				enc := json.NewEncoder(os.Stdout)
				enc.SetIndent("", "  ")
				return enc.Encode(payload)
			}
			if len(payload) == 0 {
				fmt.Println("(no sources configured)")
				return nil
			}
			tw := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
			fmt.Fprintln(tw, "NAME\tTYPE\tREF\tDIGEST\tPULL POLICY\tEXPOSE")
			for _, src := range payload {
				fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\n", src.Name, src.Type, src.Ref, src.Digest, src.PullPolicy, src.Expose)
			}
			tw.Flush()
			return nil
		},
	}
	cmd.Flags().BoolVar(&jsonOut, "json", false, "Output sources as JSON")
	return cmd
}

func newSourcesAddCmd() *cobra.Command {
	var (
		sourceType       string
		name             string
		ref              string
		urlValue         string
		pullPolicy       string
		trusted          bool
		expose           string
		verifySignatures bool
		jsonOut          bool
	)
	cmd := &cobra.Command{
		Use:   "add",
		Short: "Add or update a source via the Runner API",
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := resolveSourcesClient(cmd)
			if err != nil {
				return err
			}
			typeVal := strings.ToLower(strings.TrimSpace(sourceType))
			if typeVal == "" {
				return errors.New("--type is required")
			}
			if typeVal == "oci" {
				if !trusted {
					return errors.New("oci sources require --trusted to be set explicitly")
				}
				if strings.TrimSpace(ref) == "" && strings.TrimSpace(urlValue) == "" {
					return errors.New("oci sources require --ref or --url")
				}
			}
			payload := map[string]any{
				"type":    typeVal,
				"trusted": trusted,
			}
			if strings.TrimSpace(name) != "" {
				payload["name"] = name
			}
			if strings.TrimSpace(ref) != "" {
				payload["ref"] = ref
			}
			if strings.TrimSpace(urlValue) != "" {
				payload["url"] = urlValue
			}
			if strings.TrimSpace(pullPolicy) != "" {
				payload["pull_policy"] = strings.TrimSpace(pullPolicy)
			}
			if strings.TrimSpace(expose) != "" {
				payload["expose"] = strings.TrimSpace(expose)
			}
			if verifySignatures {
				payload["verify_signatures"] = true
			}
			body, err := json.Marshal(payload)
			if err != nil {
				return err
			}
			resp, err := client.do(cmd.Context(), http.MethodPost, "/sources", body)
			if err != nil {
				return err
			}
			defer resp.Body.Close()
			if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
				return apiError(resp)
			}
			if jsonOut {
				io.Copy(os.Stdout, resp.Body)
				return nil
			}
			var src apiSource
			if err := json.NewDecoder(resp.Body).Decode(&src); err != nil {
				return err
			}
			fmt.Printf("Source %s (%s) added/updated\n", src.Name, src.Type)
			return nil
		},
	}
	cmd.Flags().StringVar(&sourceType, "type", "oci", "Source type (oci|local|git)")
	cmd.Flags().StringVar(&name, "name", "", "Optional source name (defaults to derived)")
	cmd.Flags().StringVar(&ref, "ref", "", "Source reference (path, git ref, or image reference)")
	cmd.Flags().StringVar(&urlValue, "url", "", "Optional URL for git sources")
	cmd.Flags().StringVar(&pullPolicy, "pull-policy", "", "Pull policy for OCI sources (on-add|on-run)")
	cmd.Flags().BoolVar(&trusted, "trusted", false, "Mark source as trusted (required for oci)")
	cmd.Flags().BoolVar(&verifySignatures, "verify-signatures", false, "Require signature verification for OCI sources")
	cmd.Flags().StringVar(&expose, "expose", "", "Alias exposure level (none|read|readwrite)")
	cmd.Flags().BoolVar(&jsonOut, "json", false, "Output API response as JSON")
	return cmd
}

func newSourcesRemoveCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "remove <name>",
		Short: "Remove a source via the Runner API",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := resolveSourcesClient(cmd)
			if err != nil {
				return err
			}
			name := strings.TrimSpace(args[0])
			if name == "" {
				return errors.New("source name is required")
			}
			resp, err := client.do(cmd.Context(), http.MethodDelete, "/sources/"+urlEscape(name), nil)
			if err != nil {
				return err
			}
			defer resp.Body.Close()
			if resp.StatusCode == http.StatusNoContent {
				fmt.Printf("Source %s removed\n", name)
				return nil
			}
			return apiError(resp)
		},
	}
}

type apiSource struct {
	Name             string         `json:"name"`
	Type             string         `json:"type"`
	Ref              string         `json:"ref"`
	ResolvedRef      string         `json:"resolved_ref"`
	PullPolicy       string         `json:"pull_policy"`
	Digest           string         `json:"digest"`
	Metadata         map[string]any `json:"metadata"`
	ResolvedCommit   string         `json:"resolved_commit"`
	Expose           string         `json:"expose"`
	VerifySignatures bool           `json:"verify_signatures"`
	Provenance       map[string]any `json:"provenance"`
}

func apiError(resp *http.Response) error {
	body, _ := io.ReadAll(resp.Body)
	text := strings.TrimSpace(string(body))
	if text == "" {
		text = resp.Status
	}
	return fmt.Errorf("API error %d: %s", resp.StatusCode, text)
}

func normalizeBaseURL(raw string) string {
	base := strings.TrimSpace(raw)
	if base == "" {
		return "http://127.0.0.1:8080"
	}
	if !strings.HasPrefix(base, "http://") && !strings.HasPrefix(base, "https://") {
		base = "http://" + base
	}
	return strings.TrimRight(base, "/")
}

func urlEscape(value string) string {
	return url.PathEscape(value)
}
