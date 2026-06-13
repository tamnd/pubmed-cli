package cli

import (
	"github.com/spf13/cobra"
)

func (a *App) articleCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "article <pmid>",
		Short: "Show metadata for a PubMed article",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			pmid := args[0]
			a.progressf("fetching article %s...", pmid)
			art, err := a.client.Article(cmd.Context(), pmid)
			if err != nil {
				return mapFetchErr(err)
			}
			return a.render(art)
		},
	}
}

func (a *App) abstractCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "abstract <pmid>",
		Short: "Show the abstract for a PubMed article",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			pmid := args[0]
			a.progressf("fetching abstract for %s...", pmid)
			abs, err := a.client.Abstract(cmd.Context(), pmid)
			if err != nil {
				return mapFetchErr(err)
			}
			return a.render(abs)
		},
	}
}
