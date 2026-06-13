package cli

import (
	"github.com/spf13/cobra"
)

func (a *App) relatedCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "related <pmid>",
		Short: "Find articles related to a PubMed article",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			pmid := args[0]
			n := a.effectiveLimit(20)
			a.progressf("finding articles related to %s...", pmid)
			articles, err := a.client.Related(cmd.Context(), pmid, n)
			if err != nil {
				return mapFetchErr(err)
			}
			return a.renderOrEmpty(articles, len(articles))
		},
	}
}
