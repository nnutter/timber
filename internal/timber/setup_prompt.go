package timber

import "io"

// SetupKind identifies the automatic setup decision.
type SetupKind string

const (
	SetupMiseTrust SetupKind = "mise"
	SetupHKInstall SetupKind = "hk"
)

// SetupIdentity contains the identity shown before a setup decision.
type SetupIdentity struct {
	Username      string
	Organizations []string
}

// SetupPrompter presents setup choices and saved refusal messages.
type SetupPrompter interface {
	Choose(io.Reader, io.Writer, SetupKind, SetupIdentity) (bool, error)
	ReportDisabled(io.Writer, SetupKind) error
}
