package cli

import (
	"errors"

	"github.com/tamnd/pubmed-cli/pubmed"
)

func isNotFound(err error) bool {
	return errors.Is(err, pubmed.ErrNotFound)
}
