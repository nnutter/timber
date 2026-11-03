package review

import (
	"crypto/sha256"
	"encoding/hex"
	"slices"
	"strings"
)

// A message fingerprint identifies the message that a review of a stack shows.
// Commit identities and patch bytes are absent by design: a rebase rewrites the
// commits without changing the message, and an applied amend! commit carries
// the message that the amend! commit carried.
const messageFingerprintVersion = "v1"

// messageFingerprint hashes the effective message of one stack's commits.
func messageFingerprint(commits []historyCommit) string {
	sum := sha256.Sum256([]byte("message-" + messageFingerprintVersion + "\x00" + effectiveMessage(commits)))
	return messageFingerprintVersion + ":" + hex.EncodeToString(sum[:])
}

// effectiveMessage returns the message that a review of the stack shows. An
// amend! or reword! commit replaces the original message and a fixup! commit
// carries none, so the newest message-bearing commit decides. Autosquash applies
// the markers without changing the result, because the rewritten commit carries
// the same message.
func effectiveMessage(commits []historyCommit) string {
	for _, commit := range slices.Backward(commits) {
		switch feedbackKind(commit.subject) {
		case "fixup!":
			continue
		case "amend!", "reword!":
			return messageBody(commit.message)
		default:
			return normalizeMessage(commit.message)
		}
	}
	// Only a stack that a marker starts, which happens when no stack captured
	// the target, has no message-bearing commit.
	return normalizeMessage(commits[0].message)
}

// messageBody removes the marker paragraph and its separator, so an amended
// message compares equal to the message that it replaces.
func messageBody(message string) string {
	_, body, found := strings.Cut(message, "\n\n")
	if !found {
		return ""
	}
	return normalizeMessage(body)
}

// normalizeMessage removes the trailing newline that Git appends.
func normalizeMessage(message string) string {
	return strings.TrimRight(message, "\n")
}

// feedbackKind returns the autosquash marker that starts a subject, or "".
func feedbackKind(subject string) string {
	match := feedbackSubject.FindStringSubmatch(subject)
	if match == nil {
		return ""
	}
	return match[1]
}
