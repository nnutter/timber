package review

import (
	"context"
	"encoding/json/jsontext"
	"encoding/json/v2"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"
	"uuid"
)

// commitMessagePrefix names the pseudo-file that tuicr adds for the commit
// message that a review shows. Its key carries the abbreviated tip commit.
const commitMessagePrefix = "Commit Message ("

// carriedSession is the saved review that a rewrite moved onto a new range.
type carriedSession struct {
	store    string
	path     string
	slug     string
	document object
}

// anchorFor reports the branch name of the review checkout, or its abbreviated
// commit when the checkout is detached.
func anchorFor(ctx context.Context, repository Repository, checkout string, stack Stack) (string, error) {
	if branch, err := branchName(ctx, repository, checkout); err == nil && isNotEmpty(branch) {
		return branch, nil
	}
	// A review checkout is detached at the commit that the review shows.
	short, err := shortSHA(ctx, repository, stack.Tip)
	if err != nil {
		return "", err
	}
	return "~" + short, nil
}

// branchName reports the branch that the review checkout has checked out.
func branchName(ctx context.Context, repository Repository, checkout string) (string, error) {
	if _, err := os.Lstat(checkout); err != nil {
		return "", err
	}
	repository.Dir = checkout
	return repository.git(ctx, "symbolic-ref", "--quiet", "--short", "HEAD")
}

// carrySavedReviews moves the saved session of a reviewed patch onto the stack
// that a rewrite moved it to. A rewrite that keeps the patch keeps every file,
// hunk, and comment that the session describes, so tuicr reopens the review at
// the current range. Only a changed commit message asks the reviewer to read
// that message again.
// Carrying is best effort: a review must still start when the store cannot be
// read or holds a document that this build does not understand.
func carrySavedReviews(ctx context.Context, repository Repository, reader SavedReader) {
	view, err := repository.Snapshot(ctx)
	if err != nil {
		warnCarry(repository, err)
		return
	}
	state, err := (Store{Path: view.Path}).Load()
	if err != nil {
		warnCarry(repository, err)
		return
	}
	for _, stack := range view.Stacks {
		prior := carryablePrior(state, stack)
		if prior == nil {
			continue
		}
		if err := carrySavedSession(ctx, repository, reader, *prior, stack); err != nil {
			warnCarry(repository, err)
		}
	}
}

// carrySavedSession moves the session that captured from onto the current stack.
func carrySavedSession(ctx context.Context, repository Repository, reader SavedReader, from Attempt, to Stack) error {
	source, err := carrySource(reader, from)
	if err != nil {
		return err
	}
	if source == nil {
		return nil
	}
	checkout, err := WorktreePath(to.Repo)
	if err != nil {
		return err
	}
	described, err := describesRange(source, to, checkout)
	if err != nil {
		return err
	}
	if described {
		// A session already describes the current range.
		return nil
	}
	target, err := carryTargetFor(ctx, repository, checkout, to)
	if err != nil {
		return err
	}
	return moveCarriedSession(source, to, target, sameMessage(from.Stack, to))
}

// carrySource reads the session that captured an attempt, or nil when the
// attempt left no session to move.
func carrySource(reader SavedReader, from Attempt) (*carriedSession, error) {
	saved, err := readAttempt(reader, from, "")
	if err != nil {
		return nil, err
	}
	if saved == nil || isEmpty(saved.Store) {
		return nil, nil
	}
	document, err := readObject(saved.Path)
	if err != nil {
		return nil, err
	}
	if err := validateCarriedSession(document, from.Stack); err != nil {
		return nil, err
	}
	return &carriedSession{store: saved.Store, path: saved.Path, slug: saved.Slug, document: document}, nil
}

// carryTarget names the session and the message key that the current stack
// needs.
func carryTargetFor(ctx context.Context, repository Repository, checkout string, to Stack) (carryTarget, error) {
	anchor, err := anchorFor(ctx, repository, checkout, to)
	if err != nil {
		return carryTarget{}, err
	}
	short, err := shortSHA(ctx, repository, to.Tip)
	if err != nil {
		return carryTarget{}, err
	}
	slug, err := sessionSlug(ctx, repository, checkout, to, anchor)
	if err != nil {
		return carryTarget{}, err
	}
	return carryTarget{anchor: anchor, short: short, slug: slug, now: time.Now().UTC().Format(time.RFC3339Nano)}, nil
}

// carryTargetName holds the names and the time of the range that a session
// moves to.
type carryTarget struct {
	anchor string
	short  string
	slug   string
	now    string
}

// carryablePrior returns the newest reviewed attempt for the patch of stack
// whose captured commits a rewrite replaced.
func carryablePrior(state State, stack Stack) *Attempt {
	for i := len(state.Attempts) - 1; i >= 0; i-- {
		attempt := &state.Attempts[i]
		if isSame(attempt.ApprovalSource, "model") {
			// A model result is an assessment, not a session to reopen.
			continue
		}
		if sameStack(attempt.Stack, stack) {
			continue
		}
		if samePatch(attempt.Stack, stack) {
			return attempt
		}
	}
	return nil
}

// describesRange reports whether a session already describes the current range.
func describesRange(source *carriedSession, to Stack, checkout string) (bool, error) {
	described := describe(to, checkout)
	if described == nil {
		return false, nil
	}
	existing, err := (SavedReader{Stores: []string{source.store}}).Read(*described, "")
	if err != nil {
		return false, err
	}
	return existing != nil, nil
}

// describe returns the scope of stack inside the review checkout, or nil when
// no checkout exists to hold a session.
func describe(stack Stack, checkout string) *Stack {
	info, err := os.Lstat(checkout)
	if err != nil || isFalse(info.IsDir()) {
		return nil
	}
	described := stack
	described.Repo = checkout
	return &described
}

// moveCarriedSession writes the carried document and names its index entry
// under the current range.
func moveCarriedSession(source *carriedSession, to Stack, target carryTarget, keepMessage bool) error {
	if err := rewriteCarriedSession(source.document, to, target.short, keepMessage, target.now); err != nil {
		return err
	}
	data, err := json.Marshal(source.document, jsontext.WithIndent(" "))
	if err != nil {
		return err
	}
	if err := replaceFile(source.path, append(data, '\n')); err != nil {
		return err
	}
	return moveIndexEntry(source.store, source.path, source.slug, target.slug, sessionDisplay(source.document, target.anchor), target.now)
}

// moveIndexEntry names the moved session under its current range and drops the
// entry for the range that a review no longer shows.
func moveIndexEntry(store, path, fromSlug, toSlug string, display object, now string) error {
	index, entries, err := loadIndex(store)
	if err != nil {
		return err
	}
	if index == nil {
		// A store without an index names no sessions to move.
		return nil
	}
	if entries == nil {
		return nil
	}
	// Match on the file name: a store can be reached through a symlinked path,
	// which makes the path that the reader reports differ from the store.
	entry, err := removeIndexEntry(entries, fromSlug, filepath.Base(path))
	if err != nil {
		return err
	}
	if entry == nil {
		return nil
	}
	entry["updated_at"] = quoted(now)
	encoded, err := json.Marshal(display)
	if err != nil {
		return err
	}
	entry["display"] = encoded
	entries[toSlug] = append(entries[toSlug], entry)
	return writeIndex(store, index, entries)
}

// quoted encodes a string as a JSON document.
func quoted(value string) jsontext.Value {
	encoded, err := json.Marshal(value)
	if err != nil {
		return jsontext.Value(`""`)
	}
	return encoded
}

// removeIndexEntry removes the row that names a session file and returns it.
func removeIndexEntry(entries map[string][]object, slug, name string) (object, error) {
	rows := entries[slug]
	kept := make([]object, 0, len(rows))
	var found object
	for _, row := range rows {
		stored, err := decodeRequired[string](row, "path")
		if err != nil {
			return nil, err
		}
		if found == nil && isSame(filepath.Base(stored), name) {
			found = row
			continue
		}
		kept = append(kept, row)
	}
	if len(kept) == 0 {
		delete(entries, slug)
	} else {
		entries[slug] = kept
	}
	return found, nil
}

// renameCommitMessageFile moves the message pseudo-file to the tip's key. A
// reworded message also loses its review mark and its comments, because they
// describe a message that the review no longer shows.
func renameCommitMessageFile(files object, short string, keepMessage bool) error {
	var previous string
	for key := range files {
		if strings.HasPrefix(key, commitMessagePrefix) {
			previous = key
		}
	}
	if isEmpty(previous) {
		return nil
	}
	message, err := decodeRequired[object](files, previous)
	if err != nil {
		return err
	}
	delete(files, previous)
	name := commitMessagePrefix + short + ")"
	message["path"] = quoted(name)
	if isFalse(keepMessage) {
		message["reviewed"] = jsontext.Value("false")
		message["line_comments"] = jsontext.Value("{}")
		message["file_comments"] = jsontext.Value("[]")
	}
	encoded, err := json.Marshal(message)
	if err != nil {
		return err
	}
	files[name] = encoded
	return nil
}

// rewriteCarriedSession points a session at the commits that the review shows
// now. Marks, comments, and content hashes stay because the patch is unchanged.
func rewriteCarriedSession(document object, to Stack, short string, keepMessage bool, now string) error {
	document["id"] = quoted(uuid.New().String())
	document["created_at"] = quoted(now)
	document["updated_at"] = quoted(now)
	commits, err := json.Marshal(to.Commits)
	if err != nil {
		return err
	}
	document["commit_range"] = commits
	document["base_commit"] = quoted(to.Commits[0])
	files, err := decodeRequired[object](document, "files")
	if err != nil {
		return err
	}
	if err := renameCommitMessageFile(files, short, keepMessage); err != nil {
		return err
	}
	encoded, err := json.Marshal(files)
	if err != nil {
		return err
	}
	document["files"] = encoded
	return nil
}

// countSessionReview counts the reviewed files and the comments of a session.
func countSessionReview(files object) (int, int) {
	reviewed, comments := 0, 0
	for _, raw := range files {
		var file object
		if err := json.Unmarshal(raw, &file); err != nil {
			continue
		}
		if value, err := decodeRequired[bool](file, "reviewed"); err == nil && isTrue(value) {
			reviewed++
		}
		if value, err := decodeRequired[object](file, "line_comments"); err == nil {
			for _, items := range value {
				var list []jsontext.Value
				if err := json.Unmarshal(items, &list); err == nil {
					comments += len(list)
				}
			}
		}
		if value, err := decodeRequired[[]jsontext.Value](file, "file_comments"); err == nil {
			comments += len(value)
		}
	}
	return reviewed, comments
}

// sessionDisplay counts what the moved session shows, because the index that
// tuicr lists sessions from keeps its own copy of those counts.
func sessionDisplay(document object, anchor string) object {
	display := object{
		"comment_count":  jsontext.Value("0"),
		"reviewed_count": jsontext.Value("0"),
		"file_count":     jsontext.Value("0"),
		"anchor":         quoted(anchor),
	}
	files, err := decodeRequired[object](document, "files")
	if err != nil {
		return display
	}
	reviewed, comments := countSessionReview(files)
	display["reviewed_count"] = jsontext.Value(fmt.Sprintf("%d", reviewed))
	display["comment_count"] = jsontext.Value(fmt.Sprintf("%d", comments))
	display["file_count"] = jsontext.Value(fmt.Sprintf("%d", len(files)))
	return display
}

// sessionSlug names the session that tuicr keeps for a stack in its review
// checkout.
func sessionSlug(ctx context.Context, repository Repository, checkout string, stack Stack, anchor string) (string, error) {
	first, err := shortSHA(ctx, repository, stack.Commits[0])
	if err != nil {
		return "", err
	}
	tip, err := shortSHA(ctx, repository, stack.Tip)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%s@%s/commits/%s..%s", filepath.Base(checkout), anchor, first, tip), nil
}

// shortSHA reports the abbreviation that git prints for a revision.
func shortSHA(ctx context.Context, repository Repository, revision string) (string, error) {
	return repository.git(ctx, "rev-parse", "--short", revision)
}

// validateCarriedSession rejects a document whose range is not the range that
// the attempt captured. Carrying another document would attach a review to
// content that its marks and comments never described.
func validateCarriedSession(document object, from Stack) error {
	source, err := decodeRequired[string](document, "diff_source")
	if err != nil {
		return err
	}
	if isFalse(isSame(source, "commit_range")) {
		return fmt.Errorf("saved review does not capture a commit range")
	}
	commits, err := decodeRequired[[]string](document, "commit_range")
	if err != nil {
		return err
	}
	if isFalse(slices.Equal(commits, from.Commits)) {
		return fmt.Errorf("saved review captured a different commit range")
	}
	return nil
}

// warnCarry reports a session that could not move, because the review still
// has to start.
func warnCarry(repository Repository, cause error) {
	repository.logger().Warn("Saved review carry failed", "error", cause)
}
