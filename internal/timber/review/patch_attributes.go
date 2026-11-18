package review

import (
	"context"
	"fmt"
	"maps"
	"slices"
	"strings"
)

// Ask Git to resolve attributes once, but key each stack with only the
// attributes of its own changed paths.
func stackPatchAttributes(ctx context.Context, r Repository, stacks []Stack) (map[string]string, error) {
	pathsByTip, err := snapshotPatchPaths(ctx, r, stacks)
	if err != nil {
		return nil, err
	}
	attributesByPath, err := resolvePatchAttributes(ctx, r, pathsByTip)
	if err != nil {
		return nil, err
	}
	attributesByTip := make(map[string]string, len(stacks))
	for tip, paths := range pathsByTip {
		slices.Sort(paths)
		var attributes strings.Builder
		for _, path := range slices.Compact(paths) {
			attributes.WriteString(attributesByPath[path])
		}
		attributesByTip[tip] = attributes.String()
	}
	return attributesByTip, nil
}

func resolvePatchAttributes(ctx context.Context, r Repository, pathsByTip map[string][]string) (map[string]string, error) {
	uniquePaths := make(map[string]struct{})
	for paths := range maps.Values(pathsByTip) {
		for _, path := range paths {
			uniquePaths[path] = struct{}{}
		}
	}
	if len(uniquePaths) == 0 {
		return nil, nil
	}
	paths := slices.Sorted(maps.Keys(uniquePaths))
	out, err := r.gitInput(ctx, strings.Join(paths, "\x00")+"\x00", "check-attr", "--all", "--stdin", "-z")
	if err != nil {
		return nil, err
	}
	return parsePatchAttributes(out)
}

func parsePatchAttributes(out string) (map[string]string, error) {
	if out == "" {
		return nil, nil
	}
	records := strings.Split(out, "\x00")
	if !validAttributeRecords(records) {
		return nil, fmt.Errorf("invalid Git check-attr records")
	}
	byPath := make(map[string][]string)
	for i := 0; i < len(records)-1; i += 3 {
		path, name, value := records[i], records[i+1], records[i+2]
		byPath[path] = append(byPath[path], path+"\x00"+name+"\x00"+value+"\x00")
	}
	attributes := make(map[string]string, len(byPath))
	for path, entries := range byPath {
		// Git's attribute output order can depend on the other paths queried.
		slices.Sort(entries)
		attributes[path] = strings.Join(entries, "")
	}
	return attributes, nil
}

func validAttributeRecords(records []string) bool {
	return isSame(len(records)%3, 1) && isSame(records[len(records)-1], "")
}
