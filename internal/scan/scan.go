//go:build darwin

// Package scan measures directory trees from metadata alone. No regular file
// is ever opened, so a dataless placeholder can never be materialised by a
// scan.
package scan

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"

	"golang.org/x/sys/unix"

	"github.com/geraldcsoftware/disk-usage-cli/internal/sys"
)

// Options controls one scan.
type Options struct {
	// Unit is "files" (every regular file and symlink is a unit) or
	// "top_level_dirs" (each immediate child of the root is a unit).
	Unit string
	// Workers bounds the goroutines walking immediate children. Defaults to
	// the CPU count.
	Workers int
	// CollectUnits keeps the unit list. Totals only when false, which is what
	// the scheduled check needs.
	CollectUnits bool
	// Home locates the privacy protected roots for classification.
	Home string
}

// Unit is one deletion candidate at the configured granularity.
type Unit struct {
	RelPath   string
	IsDir     bool
	Allocated int64
	Apparent  int64
	CloudOnly int64
	ModTime   time.Time
	Dev       int32
	Ino       uint64
	Freeable  bool
}

// Result is the measurement of one rule path.
type Result struct {
	Allocated       int64
	Apparent        int64
	CloudOnly       int64
	FileCount       int
	Units           []Unit
	Skipped         map[Class]int
	UnmeasuredRoots []string
}

type devIno struct {
	dev int32
	ino uint64
}

type scanner struct {
	root    string
	rootDev int32
	opts    Options

	mu         sync.Mutex
	seen       map[devIno]bool
	skipped    map[Class]int
	unmeasured map[string]bool
}

// subtree accumulates one branch of the walk.
type subtree struct {
	allocated, apparent, cloudOnly int64
	files                          int
	newest                         time.Time
	units                          []Unit
}

func (s *subtree) add(o subtree) {
	s.allocated += o.allocated
	s.apparent += o.apparent
	s.cloudOnly += o.cloudOnly
	s.files += o.files
	if o.newest.After(s.newest) {
		s.newest = o.newest
	}
	s.units = append(s.units, o.units...)
}

// Scan measures root. It errors only when root cannot be lstat'ed or is not
// a directory; everything else is counted in Result.Skipped.
func Scan(root string, opts Options) (Result, error) {
	if opts.Unit == "" {
		opts.Unit = "files"
	}
	if opts.Workers <= 0 {
		opts.Workers = runtime.NumCPU()
	}
	var st unix.Stat_t
	if err := unix.Lstat(root, &st); err != nil {
		return Result{}, &os.PathError{Op: "lstat", Path: root, Err: err}
	}
	if st.Mode&unix.S_IFMT != unix.S_IFDIR {
		return Result{}, fmt.Errorf("scan %s: not a directory", root)
	}
	s := &scanner{root: root, rootDev: st.Dev, opts: opts, seen: map[devIno]bool{}, skipped: map[Class]int{}, unmeasured: map[string]bool{}}

	var total subtree
	entries, err := os.ReadDir(root)
	if err != nil {
		s.record(err, root, st.Flags)
	} else {
		results := make([]subtree, len(entries))
		sem := make(chan struct{}, opts.Workers)
		var wg sync.WaitGroup
		for i, e := range entries {
			wg.Add(1)
			sem <- struct{}{}
			go func(i int, name string) {
				defer wg.Done()
				defer func() { <-sem }()
				results[i] = s.walk(name, filepath.Join(root, name), true)
			}(i, e.Name())
		}
		wg.Wait()
		total.allocated = sys.AllocatedBytes(&st)
		for _, r := range results {
			total.add(r)
		}
	}

	res := Result{Allocated: total.allocated, Apparent: total.apparent, CloudOnly: total.cloudOnly, FileCount: total.files, Skipped: s.skipped}
	if opts.CollectUnits {
		units := total.units
		if opts.Unit == "files" {
			units = groupSQLiteCompanions(units)
		}
		sort.Slice(units, func(i, j int) bool { return units[i].RelPath < units[j].RelPath })
		res.Units = units
	}
	for r := range s.unmeasured {
		res.UnmeasuredRoots = append(res.UnmeasuredRoots, r)
	}
	sort.Strings(res.UnmeasuredRoots)
	return res, nil
}

// walk measures one entry. topLevel marks immediate children of the root,
// which are the units in top_level_dirs mode.
func (s *scanner) walk(rel, abs string, topLevel bool) subtree {
	var st unix.Stat_t
	if err := unix.Lstat(abs, &st); err != nil {
		s.record(err, abs, 0)
		return subtree{}
	}
	if st.Dev != s.rootDev {
		return subtree{}
	}
	if st.Mode&unix.S_IFMT != unix.S_IFDIR {
		return s.file(rel, st, topLevel)
	}
	sub := s.walkDir(rel, abs, st)
	mtime := sub.newest
	if mtime.IsZero() {
		mtime = sys.ModTime(&st)
	}
	if IsPackageLeaf(filepath.Base(abs)) {
		sub.units = nil
		if s.opts.CollectUnits && (s.opts.Unit == "files" || topLevel) {
			sub.units = []Unit{{RelPath: rel, IsDir: true, Allocated: sub.allocated, Apparent: sub.apparent, CloudOnly: sub.cloudOnly,
				ModTime: mtime, Dev: st.Dev, Ino: st.Ino, Freeable: s.opts.Unit == "top_level_dirs"}}
		}
		return sub
	}
	if topLevel && s.opts.Unit == "top_level_dirs" {
		sub.units = nil
		if s.opts.CollectUnits {
			sub.units = []Unit{{RelPath: rel, IsDir: true, Allocated: sub.allocated, Apparent: sub.apparent, CloudOnly: sub.cloudOnly,
				ModTime: mtime, Dev: st.Dev, Ino: st.Ino, Freeable: true}}
		}
	}
	return sub
}

// walkDir measures the children of a directory whose Stat_t is already known.
// The directory's own blocks are counted; its mtime is not, because age is
// derived from the files inside.
func (s *scanner) walkDir(rel, abs string, st unix.Stat_t) subtree {
	sub := subtree{allocated: sys.AllocatedBytes(&st)}
	entries, err := os.ReadDir(abs)
	if err != nil {
		s.record(err, abs, st.Flags)
		return sub
	}
	for _, e := range entries {
		sub.add(s.walk(filepath.Join(rel, e.Name()), filepath.Join(abs, e.Name()), false))
	}
	return sub
}

// file accounts for a regular file, symlink or other non directory inode.
func (s *scanner) file(rel string, st unix.Stat_t, topLevel bool) subtree {
	alloc := sys.AllocatedBytes(&st)
	apparent := st.Size
	var cloud int64
	if sys.IsDataless(st.Flags) {
		cloud = st.Size
		alloc = 0
	}
	freeable := true
	if st.Nlink > 1 {
		freeable = false
		if s.markSeen(st.Dev, st.Ino) {
			alloc, apparent, cloud = 0, 0, 0
		}
	}
	sub := subtree{allocated: alloc, apparent: apparent, cloudOnly: cloud, files: 1, newest: sys.ModTime(&st)}
	if s.opts.CollectUnits && (s.opts.Unit == "files" || topLevel) {
		sub.units = []Unit{{RelPath: rel, Allocated: alloc, Apparent: apparent, CloudOnly: cloud, ModTime: sub.newest, Dev: st.Dev, Ino: st.Ino, Freeable: freeable}}
	}
	return sub
}

// markSeen reports whether the inode was already counted.
func (s *scanner) markSeen(dev int32, ino uint64) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	key := devIno{dev, ino}
	if s.seen[key] {
		return true
	}
	s.seen[key] = true
	return false
}

// record classifies and counts a failure. A privacy protected failure also
// records the protected root so the report can say the path was not measured.
func (s *scanner) record(err error, path string, flags uint32) {
	class := Classify(err, flags, path, s.opts.Home)
	s.mu.Lock()
	defer s.mu.Unlock()
	s.skipped[class]++
	if class == ClassPrivacyProtected {
		if root, ok := UnderPrivacyProtectedRoot(path, s.opts.Home); ok {
			s.unmeasured[root] = true
		}
	}
}

var sqliteCompanionSuffixes = []string{"-wal", "-shm", "-journal"}

// groupSQLiteCompanions folds write ahead logs, shared memory files and
// rollback journals into the unit of their database file, so a plan never
// removes one without the others.
func groupSQLiteCompanions(units []Unit) []Unit {
	index := make(map[string]int, len(units))
	for i, u := range units {
		index[u.RelPath] = i
	}
	drop := make(map[int]bool)
	for i, u := range units {
		for _, suffix := range sqliteCompanionSuffixes {
			if !strings.HasSuffix(u.RelPath, suffix) {
				continue
			}
			base, ok := index[strings.TrimSuffix(u.RelPath, suffix)]
			if !ok || base == i {
				continue
			}
			b := &units[base]
			b.Allocated += u.Allocated
			b.Apparent += u.Apparent
			b.CloudOnly += u.CloudOnly
			if u.ModTime.After(b.ModTime) {
				b.ModTime = u.ModTime
			}
			b.Freeable = b.Freeable && u.Freeable
			drop[i] = true
		}
	}
	if len(drop) == 0 {
		return units
	}
	out := make([]Unit, 0, len(units)-len(drop))
	for i, u := range units {
		if !drop[i] {
			out = append(out, u)
		}
	}
	return out
}
