package timber

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestImportRepoRequiresBothReporters(t *testing.T) {
	t.Parallel()
	for _, reporter := range []ImportReporter{
		{},
		{Registered: func(string, string) error { return nil }},
		{Summary: func(ImportSummary) error { return nil }},
	} {
		err := testRuntime(t).ImportRepo("missing", "project", reporter)
		require.ErrorContains(t, err, "requires registration and summary reporters")
	}
}
