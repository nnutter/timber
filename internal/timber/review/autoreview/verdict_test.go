package autoreview

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestVerdictValidate(t *testing.T) {
	good := Verdict{Decision: "approve", Reason: "equivalent", Evidence: []string{"same behavior"}, Model: "model", ResponseID: "response"}
	require.NoError(t, good.Validate())
	for _, change := range []func(*Verdict){
		func(v *Verdict) { v.Decision = "yes" },
		func(v *Verdict) { v.Reason = " " },
		func(v *Verdict) { v.Evidence = nil },
		func(v *Verdict) { v.Evidence = []string{" "} },
		func(v *Verdict) { v.Model = "" },
		func(v *Verdict) { v.ResponseID = "" },
	} {
		v := good
		change(&v)
		require.Error(t, v.Validate())
	}
}
