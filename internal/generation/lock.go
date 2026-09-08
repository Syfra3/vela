package generation

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type Lock struct {
	Dir         string
	file        *os.File
	directories *directoryGuard
}

func Sync(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	err = f.Sync()
	e := f.Close()
	if err != nil {
		return err
	}
	return e
}

func Acquire(ctx context.Context, out string) (*Lock, error) {
	directories, err := writerDirectories(ctx, out)
	if err != nil {
		return nil, err
	}
	transferred := false
	defer func() {
		if !transferred {
			directories.Close()
		}
	}()
	out = directories.Dir
	abs, err := filepath.Abs(out)
	if err != nil {
		return nil, err
	}
	var created []string
	for p := abs; ; p = filepath.Dir(p) {
		if _, err := os.Stat(p); err == nil {
			resolved, err := filepath.EvalSymlinks(p)
			if err != nil {
				return nil, err
			}
			for a := resolved; filepath.Dir(a) != a; a = filepath.Dir(a) {
				if filepath.Base(a) == ".generations" {
					return nil, fmt.Errorf("immutable generation cannot be a writer target")
				}
			}
			break
		} else if !os.IsNotExist(err) {
			return nil, err
		}
		created = append(created, p)
		if filepath.Dir(p) == p {
			break
		}
	}
	if err := os.MkdirAll(abs, 0755); err != nil {
		return nil, err
	}
	for _, p := range created {
		if err := Sync(p); err != nil {
			return nil, err
		}
		if err := Sync(filepath.Dir(p)); err != nil {
			return nil, err
		}
	}
	abs, err = CanonicalRoot(abs)
	if err != nil {
		return nil, err
	}
	if filePresent(filepath.Join(abs, "seal.json")) {
		return nil, fmt.Errorf("sealed writer target")
	}
	lockPath := filepath.Join(abs, ".writer.lock")
	if info, err := os.Lstat(lockPath); err == nil && !info.Mode().IsRegular() {
		return nil, fmt.Errorf("writer lock must be a regular stable inode")
	} else if err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	f, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, err
	}
	for {
		if err := ctx.Err(); err != nil {
			f.Close()
			return nil, err
		}
		err = lockFile(f, lockExclusive)
		if err == nil {
			transferred = true
			return &Lock{Dir: abs, file: f, directories: directories}, nil
		}
		if !lockUnavailable(err) {
			f.Close()
			return nil, err
		}
		select {
		case <-ctx.Done():
			f.Close()
			return nil, ctx.Err()
		case <-time.After(5 * time.Millisecond):
		}
	}
}
func (l *Lock) Close() error {
	if l == nil || l.file == nil {
		return nil
	}
	defer l.directories.Close()
	f := l.file
	l.file = nil
	err := unlockFile(f)
	e := f.Close()
	if err != nil {
		return err
	}
	return e
}

// Directory locks protect the namespace BEFORE the first .writer.lock exists.
// Writers hold shared locks root-to-output for their entire lifetime. Removal
// holds exclusive target-directory locks (shared ancestors) across preflight
// AND effects. No coordination inode is created inside a deletion target.
type directoryGuard struct {
	Dir   string
	files []*os.File
}

func (g *directoryGuard) Close() {
	if g == nil {
		return
	}
	for i := len(g.files) - 1; i >= 0; i-- {
		_ = unlockFile(g.files[i])
		_ = g.files[i].Close()
	}
	g.files = nil
}

func canonicalFuture(path string) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	var missing []string
	for p := abs; ; p = filepath.Dir(p) {
		if _, err := os.Stat(p); err == nil {
			base, err := filepath.EvalSymlinks(p)
			if err != nil {
				return "", err
			}
			for i := len(missing) - 1; i >= 0; i-- {
				base = filepath.Join(base, missing[i])
			}
			return base, nil
		} else if !os.IsNotExist(err) {
			return "", err
		}
		if _, err := os.Lstat(p); err == nil {
			return "", fmt.Errorf("dangling coordination alias: %s", p)
		}
		if filepath.Dir(p) == p {
			return "", fmt.Errorf("no existing coordination ancestor")
		}
		missing = append(missing, filepath.Base(p))
	}
}

func addDirectoryPlan(plan map[string]lockMode, path string, mode lockMode) {
	for p := path; ; p = filepath.Dir(p) {
		m := lockShared
		if p == path {
			m = mode
		}
		if plan[p] != lockExclusive {
			plan[p] = m
		}
		if filepath.Dir(p) == p {
			break
		}
	}
}

func lockDirectories(ctx context.Context, plan map[string]lockMode, create bool) (*directoryGuard, error) {
	paths := make([]string, 0, len(plan))
	for p := range plan {
		covered := false
		for a := filepath.Dir(p); a != p; a = filepath.Dir(a) {
			if plan[a] == lockExclusive {
				covered = true
				break
			}
			if filepath.Dir(a) == a {
				break
			}
		}
		if !covered {
			paths = append(paths, p)
		}
	}
	sort.Slice(paths, func(i, j int) bool {
		di, dj := strings.Count(paths[i], string(filepath.Separator)), strings.Count(paths[j], string(filepath.Separator))
		if di == dj {
			return paths[i] < paths[j]
		}
		return di < dj
	})
	g := &directoryGuard{}
	ok := false
	defer func() {
		if !ok {
			g.Close()
		}
	}()
	for _, p := range paths {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if create {
			if _, err := os.Lstat(p); os.IsNotExist(err) {
				if err := os.Mkdir(p, 0755); err != nil && !os.IsExist(err) {
					return nil, err
				}
				if err := Sync(p); err != nil {
					return nil, err
				}
				if err := Sync(filepath.Dir(p)); err != nil {
					return nil, err
				}
			}
		}
		f, err := openDirectory(p)
		if err != nil {
			return nil, err
		}
		g.files = append(g.files, f)
		for {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			err = lockFile(f, plan[p])
			if err == nil {
				break
			}
			if !lockUnavailable(err) {
				return nil, err
			}
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(5 * time.Millisecond):
			}
		}
		// A waiter may have opened a directory just before removal unlinked it.
		// Never create output/lock files using a stale locked directory inode.
		locked, err := f.Stat()
		if err != nil {
			return nil, err
		}
		current, err := os.Lstat(p)
		if err != nil {
			return nil, err
		}
		if !current.IsDir() || !os.SameFile(locked, current) {
			return nil, fmt.Errorf("directory changed during coordination: %s", p)
		}
	}
	ok = true
	return g, nil
}

func writerDirectories(ctx context.Context, out string) (*directoryGuard, error) {
	canonical, err := canonicalFuture(out)
	if err != nil {
		return nil, err
	}
	for p := canonical; ; p = filepath.Dir(p) {
		if filepath.Base(p) == ".generations" || filePresent(filepath.Join(p, "seal.json")) {
			return nil, fmt.Errorf("immutable generation cannot be a writer target")
		}
		if filepath.Dir(p) == p {
			break
		}
	}
	plan := map[string]lockMode{}
	addDirectoryPlan(plan, canonical, lockShared)
	g, err := lockDirectories(ctx, plan, true)
	if err != nil {
		return nil, err
	}
	g.Dir = canonical
	return g, nil
}

func removalDirectory(path string) (string, error) {
	if info, err := os.Stat(path); err == nil && info.IsDir() {
		return CanonicalRoot(path)
	} else if err != nil && !os.IsNotExist(err) {
		return "", err
	}
	canonical, err := canonicalFuture(filepath.Dir(path))
	if err != nil {
		return "", err
	}
	for {
		if info, err := os.Stat(canonical); err == nil {
			if !info.IsDir() {
				return "", fmt.Errorf("removal parent is not directory")
			}
			return canonical, nil
		} else if !os.IsNotExist(err) {
			return "", err
		}
		canonical = filepath.Dir(canonical)
	}
}

// WithRemoval coordinates the complete operation, not an inspect-release-delete
// sequence. Callbacks must NOT reacquire writer/removal locks for guarded roots.
// Refusal is still the only behavior for adopted layouts/stable lock deletion.
func WithRemoval(ctx context.Context, targets []string, effects func() error) error {
	plan := map[string]lockMode{}
	identities := map[string]string{}
	for _, target := range targets {
		if strings.TrimSpace(target) == "" {
			continue
		}
		identity, err := canonicalFuture(target)
		if err != nil {
			return err
		}
		identities[target] = identity
		dir, err := removalDirectory(target)
		if err != nil {
			return err
		}
		if filepath.Dir(dir) == dir {
			return fmt.Errorf("filesystem-root removal refused")
		}
		addDirectoryPlan(plan, dir, lockExclusive)
		// Protect the lexical alias namespace as well as its canonical owner.
		if info, err := os.Lstat(target); err == nil && info.Mode()&os.ModeSymlink != 0 {
			parent, err := removalDirectory(filepath.Dir(target))
			if err != nil {
				return err
			}
			addDirectoryPlan(plan, parent, lockExclusive)
			actual, err := canonicalFuture(target)
			if err != nil {
				return err
			}
			owner, err := removalDirectory(actual)
			if err != nil {
				return err
			}
			addDirectoryPlan(plan, owner, lockExclusive)
		}
	}
	g, err := lockDirectories(ctx, plan, false)
	if err != nil {
		return err
	}
	defer g.Close()
	for _, target := range targets {
		if strings.TrimSpace(target) != "" {
			identity, err := canonicalFuture(target)
			if err != nil {
				return err
			}
			if identity != identities[target] {
				return fmt.Errorf("removal target changed during coordination: %s", target)
			}
			if err := RefuseRemoval(target); err != nil {
				return err
			}
		}
	}
	return effects()
}

func LegacyWrite(out string, write func(string) error) error {
	l, err := Acquire(context.Background(), out)
	if err != nil {
		return err
	}
	defer l.Close()
	if err := CheckLegacyTargets(l.Dir); err != nil {
		return err
	}
	return write(l.Dir)
}

// Call under the output writer lock. Fixed artifact and temporary names may
// not redirect writes into another output's selected/sealed generation.
func CheckLegacyTargets(out string) error {
	if Adopted(out) {
		return fmt.Errorf("independent write refused: adopted generation layout")
	}
	for _, name := range []string{"graph.json", "graph.db", "manifest.json", "graph.json.tmp", "graph.db.tmp", "manifest.json.tmp"} {
		path := filepath.Join(out, name)
		info, err := os.Lstat(path)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			if filepath.Ext(name) == ".tmp" {
				return fmt.Errorf("temporary artifact symlink refused: %s", path)
			}
			pin, err := Pin(path)
			if err != nil {
				return err
			}
			if pin != nil {
				return fmt.Errorf("sealed artifact alias write refused: %s", path)
			}
		}
	}
	return nil
}

func Initialize(out string) error {
	return LegacyWrite(out, func(dir string) error {
		path := filepath.Join(dir, "graph.db")
		if _, err := os.Lstat(path); err == nil {
			return nil
		} else if !os.IsNotExist(err) {
			return err
		}
		tmp, err := os.CreateTemp(dir, ".initialize-")
		if err != nil {
			return err
		}
		name := tmp.Name()
		defer os.Remove(name)
		if _, err := tmp.Write([]byte("SQLite format 3\x00")); err != nil {
			tmp.Close()
			return err
		}
		if err := tmp.Sync(); err != nil {
			tmp.Close()
			return err
		}
		if err := tmp.Close(); err != nil {
			return err
		}
		if err := os.Rename(name, path); err != nil {
			return err
		}
		return Sync(dir)
	})
}

// RefuseRemoval inspects canonical targets, their ancestors and nested layouts
// before callers alter any hooks, registry records, artifacts or lock identity.
// It never deletes, repairs, or reclaims anything. Inaccessible targets fail shut.
func RefuseRemoval(target string) error {
	abs, err := filepath.Abs(target)
	if err != nil {
		return err
	}
	for p := abs; filepath.Dir(p) != p; p = filepath.Dir(p) {
		if Adopted(p) || filepath.Base(p) == ".generations" || filepath.Base(p) == ".writer.lock" {
			return fmt.Errorf("removal refused: managed generation at %s", p)
		}
	}
	resolved, err := filepath.EvalSymlinks(abs)
	if err != nil {
		if os.IsNotExist(err) {
			if _, e := os.Lstat(abs); os.IsNotExist(e) {
				return nil
			}
		}
		return err
	}
	for p := resolved; filepath.Dir(p) != p; p = filepath.Dir(p) {
		if Adopted(p) || filepath.Base(p) == ".generations" || filepath.Base(p) == ".writer.lock" {
			return fmt.Errorf("removal refused: managed generation at %s", p)
		}
	}
	return filepath.WalkDir(resolved, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.Name() == ".writer.lock" || d.Name() == ".current" || d.Name() == ".generations" || d.Name() == "seal.json" {
			return fmt.Errorf("removal refused: managed generation or lock at %s", p)
		}
		if d.Type()&os.ModeSymlink != 0 {
			if target, e := filepath.EvalSymlinks(p); e != nil {
				return e
			} else {
				for a := target; filepath.Dir(a) != a; a = filepath.Dir(a) {
					if Adopted(a) {
						return fmt.Errorf("removal refused: generation alias %s", p)
					}
				}
			}
		}
		return nil
	})
}
