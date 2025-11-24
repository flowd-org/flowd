// SPDX-License-Identifier: AGPL-3.0-or-later
package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"text/tabwriter"

	"github.com/flowd-org/flowd/internal/indexer"
	"github.com/spf13/cobra"
)

func NewJobsCmd(root *cobra.Command) *cobra.Command {
	var jsonOut bool
	c := &cobra.Command{
		Use:   ":jobs",
		Short: "List discovered jobs (local)",
		RunE: func(cmd *cobra.Command, args []string) error {
			res, err := indexer.Discover("scripts")
			if err != nil {
				return err
			}

			sort.Slice(res.Jobs, func(i, j int) bool {
				return res.Jobs[i].ID < res.Jobs[j].ID
			})

			if jsonOut {
				enc := json.NewEncoder(os.Stdout)
				enc.SetIndent("", "  ")
				return enc.Encode(res)
			}

			if len(res.Jobs) == 0 {
				fmt.Println("(no jobs found under scripts/)")
			} else {
				tw := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
				fmt.Fprintln(tw, "ID\tNAME\tSUMMARY")
				for _, job := range res.Jobs {
					summary := job.Summary
					if summary == "" {
						summary = "(no summary)"
					}
					fmt.Fprintf(tw, "%s\t%s\t%s\n", job.ID, job.Name, summary)
				}
				tw.Flush()
			}

			if len(res.Aliases) > 0 {
				fmt.Println()
				fmt.Println("ALIASES")
				tw := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
				fmt.Fprintln(tw, "NAME\tTARGET\tDESCRIPTION")
				for _, alias := range res.Aliases {
					desc := alias.Description
					if desc == "" {
						desc = "(alias)"
					}
					fmt.Fprintf(tw, "%s\t%s\t%s\n", alias.Name, alias.TargetPath, desc)
				}
				tw.Flush()
			}

			for _, derr := range res.Errors {
				fmt.Fprintf(os.Stderr, "[warn] %s: %s\n", derr.Path, derr.Err)
			}

			return nil
		},
	}
	c.Flags().BoolVar(&jsonOut, "json", false, "Output jobs as JSON")
	return c
}
