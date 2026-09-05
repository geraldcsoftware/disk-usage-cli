//go:build darwin

package scan

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func write(t *testing.T, path string, size int) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, make([]byte, size), 0o644); err != nil {
		t.Fatal(err)
	}
}

func unitByPath(units []Unit, rel string) *Unit {
	for i := range units {
		if units[i].RelPath == rel {
			return &units[i]
		}
	}
	return nil
}

func TestAllocatedVersusApparent(t *testing.T) {
	root := t.TempDir()
	write(t, filepath.Join(root, "tiny"), 1)
	sparse, err := os.Create(filepath.Join(root, "sparse"))
	if err != nil {
		t.Fatal(err)
	}
	if err := sparse.Truncate(10 << 20); err != nil {
		t.Fatal(err)
	}
	sparse.Close()

	res, err := Scan(root, Options{CollectUnits: true})
	if err != nil {
		t.Fatal(err)
	}
	tiny := unitByPath(res.Units, "tiny")
	if tiny == nil || tiny.Allocated != 4096 || tiny.Apparent != 1 {
		t.Errorf("tiny = %+v, want one 4 KiB block allocated for 1 byte", tiny)
	}
	sp := unitByPath(res.Units, "sparse")
	if sp == nil || sp.Allocated != 0 || sp.Apparent != 10<<20 {
		t.Errorf("sparse = %+v, want 0 allocated and 10 MiB apparent", sp)
	}
	if res.FileCount != 2 {
		t.Errorf("file count = %d", res.FileCount)
	}
}

func TestHardLinksCountOnceAndAreNotFreeable(t *testing.T) {
	root := t.TempDir()
	write(t, filepath.Join(root, "a"), 8192)
	if err := os.Link(filepath.Join(root, "a"), filepath.Join(root, "b")); err != nil {
		t.Fatal(err)
	}
	res, err := Scan(root, Options{CollectUnits: true, Workers: 1})
	if err != nil {
		t.Fatal(err)
	}
	if res.Allocated != 8192 {
		t.Errorf("allocated = %d, want 8192 counted once", res.Allocated)
	}
	if len(res.Units) != 2 {
		t.Fatalf("units = %d, want both names", len(res.Units))
	}
	for _, u := range res.Units {
		if u.Freeable {
			t.Errorf("%s: hard linked unit must not be freeable", u.RelPath)
		}
		if u.Allocated != 8192 {
			t.Errorf("%s: allocated = %d, want each hard linked name to report its own size", u.RelPath, u.Allocated)
		}
	}
}

func TestHardLinksAcrossTopLevelDirsReportOwnSize(t *testing.T) {
	root := t.TempDir()
	write(t, filepath.Join(root, "A", "a"), 8192)
	if err := os.MkdirAll(filepath.Join(root, "B"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Link(filepath.Join(root, "A", "a"), filepath.Join(root, "B", "b")); err != nil {
		t.Fatal(err)
	}
	res, err := Scan(root, Options{CollectUnits: true})
	if err != nil {
		t.Fatal(err)
	}
	if res.Allocated != 8192 {
		t.Errorf("allocated = %d, want 8192 counted once", res.Allocated)
	}
	if len(res.Units) != 2 {
		t.Fatalf("units = %d, want both names", len(res.Units))
	}
	for _, u := range res.Units {
		if u.Freeable {
			t.Errorf("%s: hard linked unit must not be freeable", u.RelPath)
		}
		if u.Allocated != 8192 {
			t.Errorf("%s: allocated = %d, want each hard linked name to report its own size", u.RelPath, u.Allocated)
		}
	}
}

func TestSymlinksAreNotFollowed(t *testing.T) {
	outside := t.TempDir()
	write(t, filepath.Join(outside, "big"), 1<<20)
	root := t.TempDir()
	if err := os.Symlink(filepath.Join(outside, "big"), filepath.Join(root, "link")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "dirlink")); err != nil {
		t.Fatal(err)
	}
	res, err := Scan(root, Options{CollectUnits: true})
	if err != nil {
		t.Fatal(err)
	}
	if res.Allocated >= 1<<20 {
		t.Errorf("allocated = %d; the symlink target must not be counted", res.Allocated)
	}
	link := unitByPath(res.Units, "link")
	if link == nil || link.Apparent != int64(len(filepath.Join(outside, "big"))) || link.IsDir {
		t.Errorf("link = %+v, want a unit sized as the link itself", link)
	}
	if dl := unitByPath(res.Units, "dirlink"); dl == nil || dl.IsDir {
		t.Errorf("dirlink = %+v, want a non directory unit", dl)
	}
}

func TestPackageLeafIsOneUnit(t *testing.T) {
	root := t.TempDir()
	write(t, filepath.Join(root, "Foo.app", "Contents", "MacOS", "foo"), 4096)
	write(t, filepath.Join(root, "plain", "f"), 4096)
	res, err := Scan(root, Options{CollectUnits: true})
	if err != nil {
		t.Fatal(err)
	}
	app := unitByPath(res.Units, "Foo.app")
	if app == nil || !app.IsDir || app.Freeable || app.Allocated < 4096 {
		t.Errorf("Foo.app = %+v, want one non freeable directory unit", app)
	}
	if unitByPath(res.Units, "Foo.app/Contents/MacOS/foo") != nil {
		t.Error("package contents must not appear as units")
	}
	if unitByPath(res.Units, "plain/f") == nil {
		t.Error("ordinary nested files are units in files mode")
	}
}

func TestTopLevelDirsUnits(t *testing.T) {
	root := t.TempDir()
	write(t, filepath.Join(root, "A", "one"), 4096)
	write(t, filepath.Join(root, "A", "deep", "two"), 4096)
	write(t, filepath.Join(root, "B", "three"), 4096)
	write(t, filepath.Join(root, "loose"), 4096)
	old := time.Now().Add(-48 * time.Hour)
	if err := os.Chtimes(filepath.Join(root, "A", "one"), old, old); err != nil {
		t.Fatal(err)
	}
	newest := time.Now().Add(-time.Hour).Truncate(time.Second)
	if err := os.Chtimes(filepath.Join(root, "A", "deep", "two"), newest, newest); err != nil {
		t.Fatal(err)
	}
	res, err := Scan(root, Options{Unit: "top_level_dirs", CollectUnits: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Units) != 3 {
		t.Fatalf("units = %+v, want A, B and loose", res.Units)
	}
	a := unitByPath(res.Units, "A")
	if a == nil || !a.IsDir || a.Allocated < 8192 || !a.Freeable {
		t.Errorf("A = %+v", a)
	}
	if !a.ModTime.Equal(newest) {
		t.Errorf("A mtime = %v, want newest file inside %v", a.ModTime, newest)
	}
	if res.Units[0].RelPath != "A" || res.Units[1].RelPath != "B" || res.Units[2].RelPath != "loose" {
		t.Errorf("units not sorted: %+v", res.Units)
	}
}

func TestSQLiteCompanionsAreGrouped(t *testing.T) {
	root := t.TempDir()
	for _, n := range []string{"cache.db", "cache.db-wal", "cache.db-shm", "other.db-journal"} {
		write(t, filepath.Join(root, n), 4096)
	}
	res, err := Scan(root, Options{CollectUnits: true})
	if err != nil {
		t.Fatal(err)
	}
	db := unitByPath(res.Units, "cache.db")
	if db == nil || db.Allocated != 3*4096 {
		t.Errorf("cache.db = %+v, want companions folded in", db)
	}
	if unitByPath(res.Units, "cache.db-wal") != nil {
		t.Error("companion must not remain a separate unit")
	}
	if unitByPath(res.Units, "other.db-journal") == nil {
		t.Error("a companion without its base file stays its own unit")
	}
}

func TestSQLiteCompanionChainIsGrouped(t *testing.T) {
	root := t.TempDir()
	for _, n := range []string{"x.db", "x.db-wal", "x.db-wal-journal"} {
		write(t, filepath.Join(root, n), 4096)
	}
	res, err := Scan(root, Options{CollectUnits: true})
	if err != nil {
		t.Fatal(err)
	}
	db := unitByPath(res.Units, "x.db")
	if db == nil || db.Allocated != 3*4096 {
		t.Errorf("x.db = %+v, want the whole chain folded in", db)
	}
	if unitByPath(res.Units, "x.db-wal") != nil {
		t.Error("x.db-wal must not remain a separate unit")
	}
	if unitByPath(res.Units, "x.db-wal-journal") != nil {
		t.Error("x.db-wal-journal must not remain a separate unit")
	}
}

func TestUnreadableDirectoryIsCountedNotFatal(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root bypasses permission checks")
	}
	root := t.TempDir()
	locked := filepath.Join(root, "locked")
	write(t, filepath.Join(locked, "f"), 4096)
	if err := os.Chmod(locked, 0); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(locked, 0o755) })
	write(t, filepath.Join(root, "open", "g"), 4096)
	res, err := Scan(root, Options{CollectUnits: true})
	if err != nil {
		t.Fatal(err)
	}
	if res.Skipped[ClassPermission] != 1 {
		t.Errorf("skipped = %v, want one permission skip", res.Skipped)
	}
	if unitByPath(res.Units, "open/g") == nil {
		t.Error("the rest of the tree is still measured")
	}
}

func TestRootErrors(t *testing.T) {
	if _, err := Scan(filepath.Join(t.TempDir(), "missing"), Options{}); err == nil {
		t.Error("missing root must error")
	}
	f := filepath.Join(t.TempDir(), "file")
	write(t, f, 1)
	if _, err := Scan(f, Options{}); err == nil {
		t.Error("a file root must error")
	}
}

func TestTotalsWithoutUnits(t *testing.T) {
	root := t.TempDir()
	write(t, filepath.Join(root, "d", "f"), 4096)
	res, err := Scan(root, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if res.Units != nil || res.Allocated < 4096 || res.FileCount != 1 {
		t.Errorf("result = %+v, want totals only", res)
	}
}
