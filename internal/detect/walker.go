package detect

import (
	"io/fs"
	"path/filepath"
)

// FileEntry represents a single discovered file.
type FileEntry struct {
	AbsPath string
	RelPath string // relative to the root passed to Walk
}

// Walker walks a directory tree, collecting files while respecting:
//  1. Per-directory .gitignore files (gitignore spec, loaded at each dir)
//  2. Per-directory .velignore files (override / complement .gitignore)
//  3. Tech-detected default ignore rules (applied at any dir where a manifest is found)
//
// Ignore rules compose — a file is excluded if ANY active IgnoreList marks it ignored.
// .velignore has highest priority (evaluated last), matching gitignore precedence semantics.
type Walker struct {
	root string
}

// NewWalker creates a Walker rooted at root.
func NewWalker(root string) (*Walker, error) {
	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	return &Walker{root: abs}, nil
}

// Walk traverses the tree and returns all non-ignored files.
func (w *Walker) Walk() ([]FileEntry, error) {
	var files []FileEntry

	// ignoreSets maps each directory (absolute path) to its compiled ignore lists.
	// We accumulate lists as we descend — each dir inherits its parent's lists
	// and adds its own.
	type dirState struct {
		lists []*IgnoreList
	}
	dirStates := map[string]dirState{}

	// Seed root state with tech defaults + root-level ignore files.
	rootLists, err := loadDirIgnoreLists(w.root, nil)
	if err != nil {
		return nil, err
	}
	dirStates[w.root] = dirState{lists: rootLists}

	err = filepath.WalkDir(w.root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			// skip entries we cannot access (permission errors, broken symlinks)
			return nil
		}

		// Resolve parent dir state.
		dir := path
		if !d.IsDir() {
			dir = filepath.Dir(path)
		}

		state, ok := dirStates[dir]
		if !ok {
			// This dir was not skipped, so its parent must be in dirStates.
			parentState := dirStates[filepath.Dir(dir)]
			lists, loadErr := loadDirIgnoreLists(dir, parentState.lists)
			if loadErr != nil {
				return loadErr
			}
			state = dirState{lists: lists}
			dirStates[dir] = state
		}

		isDir := d.IsDir()

		// Skip root itself.
		if path == w.root {
			return nil
		}

		// Check if this entry is ignored by any active list.
		if isIgnoredByAny(state.lists, path, isDir) {
			if isDir {
				return filepath.SkipDir
			}
			return nil
		}

		if !isDir {
			rel, relErr := filepath.Rel(w.root, path)
			if relErr != nil {
				return relErr
			}
			files = append(files, FileEntry{
				AbsPath: path,
				RelPath: rel,
			})
		}

		return nil
	})

	return files, err
}

// loadDirIgnoreLists builds the ignore list set for dir by:
//  1. Starting with parent lists (inherited)
//  2. Adding tech-default rules if a manifest is found in dir
//  3. Adding .gitignore from dir (if present)
//  4. Adding .velignore from dir (if present, highest priority)
func loadDirIgnoreLists(dir string, parentLists []*IgnoreList) ([]*IgnoreList, error) {
	// Copy parent slice so we don't mutate it.
	lists := make([]*IgnoreList, len(parentLists))
	copy(lists, parentLists)

	// Tech defaults — only add if a manifest is directly in this dir.
	tech := DetectTech(dir)
	if tech != TechUnknown {
		defaults := DefaultIgnorePatterns(tech)
		lists = append(lists, NewIgnoreList(dir, defaults))
	}

	// .gitignore
	gi, err := LoadIgnoreFile(dir, ".gitignore")
	if err != nil {
		return nil, err
	}
	if gi != nil {
		lists = append(lists, gi)
	}

	// .velignore (highest priority — evaluated last so its result wins)
	vi, err := LoadIgnoreFile(dir, ".velignore")
	if err != nil {
		return nil, err
	}
	if vi != nil {
		lists = append(lists, vi)
	}

	return lists, nil
}

// isIgnoredByAny returns true if any IgnoreList considers path ignored.
// Lists are evaluated in order (tech defaults → .gitignore → .velignore).
// A file is excluded if ANY list marks it ignored; there is no cross-list
// negation — negation only works within the same file (gitignore semantics).
func isIgnoredByAny(lists []*IgnoreList, absPath string, isDir bool) bool {
	for _, l := range lists {
		if l.Ignored(absPath, isDir) {
			return true
		}
	}
	return false
}

// WalkDir is a convenience function that creates a Walker and calls Walk.
func WalkDir(root string) ([]FileEntry, error) {
	w, err := NewWalker(root)
	if err != nil {
		return nil, err
	}
	return w.Walk()
}
