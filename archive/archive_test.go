package archive

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"sort"
	"testing"
)

const fixturesDir = "./test-fixtures"

var testHasGit bool
var testHasHg bool

func init() {
	if _, err := exec.LookPath("git"); err == nil {
		testHasGit = true
	}

	if _, err := exec.LookPath("hg"); err == nil {
		testHasHg = true
	}
}

func TestArchiveOptsIsSet(t *testing.T) {
	cases := []struct {
		Opts *ArchiveOpts
		Set  bool
	}{
		{
			&ArchiveOpts{},
			false,
		},
		{
			&ArchiveOpts{VCS: true},
			true,
		},
		{
			&ArchiveOpts{Exclude: make([]string, 0, 0)},
			false,
		},
		{
			&ArchiveOpts{Exclude: []string{"foo"}},
			true,
		},
		{
			&ArchiveOpts{Include: make([]string, 0, 0)},
			false,
		},
		{
			&ArchiveOpts{Include: []string{"foo"}},
			true,
		},
	}

	for i, tc := range cases {
		if tc.Opts.IsSet() != tc.Set {
			t.Fatalf("%d: expected %#v", i, tc.Set)
		}
	}
}

func TestArchive_file(t *testing.T) {
	path := filepath.Join(testFixture("archive-file"), "foo.txt")
	r, errCh, err := Archive(path, new(ArchiveOpts))
	if err != nil {
		t.Fatalf("err: %s", err)
	}

	expected := []string{
		"foo.txt",
	}

	entries := testArchive(t, r, errCh)
	if !reflect.DeepEqual(entries, expected) {
		t.Fatalf("bad: %#v", entries)
	}
}

func TestArchive_fileCompressed(t *testing.T) {
	path := filepath.Join(testFixture("archive-file-compressed"), "file.tar.gz")
	r, errCh, err := Archive(path, new(ArchiveOpts))
	if err != nil {
		t.Fatalf("err: %s", err)
	}

	expected := []string{
		"./foo.txt",
	}

	entries := testArchive(t, r, errCh)
	if !reflect.DeepEqual(entries, expected) {
		t.Fatalf("bad: %#v", entries)
	}
}

func TestArchive_fileNoExist(t *testing.T) {
	tf := tempFile(t)
	if err := os.Remove(tf); err != nil {
		t.Fatalf("err: %s", err)
	}

	r, errCh, err := Archive(tf, nil)
	if err == nil {
		t.Fatal("err should not be nil")
	}
	if r != nil {
		t.Fatal("should be nil")
	}
	if errCh != nil {
		t.Fatal("should be nil")
	}
}

func TestArchive_fileWithOpts(t *testing.T) {
	r, errCh, err := Archive(tempFile(t), &ArchiveOpts{VCS: true})
	if err == nil {
		t.Fatal("err should not be nil")
	}
	if r != nil {
		t.Fatal("should be nil")
	}
	if errCh != nil {
		t.Fatal("should be nil")
	}
}

func TestArchive_dirExtra(t *testing.T) {
	opts := &ArchiveOpts{
		Extra: map[string]string{
			"hello.txt": filepath.Join(
				testFixture("archive-subdir"), "subdir", "hello.txt"),
		},
	}

	r, errCh, err := Archive(testFixture("archive-flat"), opts)
	if err != nil {
		t.Fatalf("err: %s", err)
	}

	expected := []string{
		"baz.txt",
		"foo.txt",
		"hello.txt",
	}

	entries := testArchive(t, r, errCh)
	if !reflect.DeepEqual(entries, expected) {
		t.Fatalf("bad: %#v", entries)
	}
}

func TestArchive_dirNoVCS(t *testing.T) {
	r, errCh, err := Archive(testFixture("archive-flat"), new(ArchiveOpts))
	if err != nil {
		t.Fatalf("err: %s", err)
	}

	expected := []string{
		"baz.txt",
		"foo.txt",
	}

	entries := testArchive(t, r, errCh)
	if !reflect.DeepEqual(entries, expected) {
		t.Fatalf("bad: %#v", entries)
	}
}

func TestArchive_dirSubdirsNoVCS(t *testing.T) {
	r, errCh, err := Archive(testFixture("archive-subdir"), new(ArchiveOpts))
	if err != nil {
		t.Fatalf("err: %s", err)
	}

	expected := []string{
		"bar.txt",
		"foo.txt",
		"subdir/",
		"subdir/hello.txt",
	}

	entries := testArchive(t, r, errCh)
	if !reflect.DeepEqual(entries, expected) {
		t.Fatalf("bad: %#v", entries)
	}
}

func TestArchive_dirExclude(t *testing.T) {
	opts := &ArchiveOpts{
		Exclude: []string{"subdir", "subdir/*"},
	}

	r, errCh, err := Archive(testFixture("archive-subdir"), opts)
	if err != nil {
		t.Fatalf("err: %s", err)
	}

	expected := []string{
		"bar.txt",
		"foo.txt",
	}

	entries := testArchive(t, r, errCh)
	if !reflect.DeepEqual(entries, expected) {
		t.Fatalf("bad: %#v", entries)
	}
}

func TestArchive_dirInclude(t *testing.T) {
	opts := &ArchiveOpts{
		Include: []string{"bar.txt"},
	}

	r, errCh, err := Archive(testFixture("archive-subdir"), opts)
	if err != nil {
		t.Fatalf("err: %s", err)
	}

	expected := []string{
		"bar.txt",
	}

	entries := testArchive(t, r, errCh)
	if !reflect.DeepEqual(entries, expected) {
		t.Fatalf("bad: %#v", entries)
	}
}

func TestArchive_git(t *testing.T) {
	if !testHasGit {
		t.Log("git not found, skipping")
		t.Skip()
	}

	// Git doesn't allow nested ".git" directories so we do some hackiness
	// here to get around that...
	testDir := testFixture("archive-git")
	oldName := filepath.Join(testDir, "DOTgit")
	newName := filepath.Join(testDir, ".git")
	if err := os.Rename(oldName, newName); err != nil {
		t.Fatalf("err: %s", err)
	}
	defer os.Rename(newName, oldName)

	// testDir with VCS set to true
	r, errCh, err := Archive(testDir, &ArchiveOpts{VCS: true})
	if err != nil {
		t.Fatalf("err: %s", err)
	}

	expected := []string{
		"bar.txt",
		"foo.txt",
		"subdir/",
		"subdir/hello.txt",
	}

	entries := testArchive(t, r, errCh)
	if !reflect.DeepEqual(entries, expected) {
		t.Fatalf("bad: %#v", entries)
	}
}

func TestArchive_hg(t *testing.T) {
	if !testHasHg {
		t.Log("hg not found, skipping")
		t.Skip()
	}

	// testDir with VCS set to true
	testDir := testFixture("archive-hg")
	r, errCh, err := Archive(testDir, &ArchiveOpts{VCS: true})
	if err != nil {
		t.Fatalf("err: %s", err)
	}

	expected := []string{
		"bar.txt",
		"foo.txt",
		"subdir/",
		"subdir/hello.txt",
	}

	entries := testArchive(t, r, errCh)
	if !reflect.DeepEqual(entries, expected) {
		t.Fatalf("bad: %#v", entries)
	}
}

func TestArchive_hgSubdir(t *testing.T) {
	if !testHasHg {
		t.Log("hg not found, skipping")
		t.Skip()
	}

	// testDir with VCS set to true
	testDir := filepath.Join(testFixture("archive-hg"), "subdir")
	r, errCh, err := Archive(testDir, &ArchiveOpts{VCS: true})
	if err != nil {
		t.Fatalf("err: %s", err)
	}

	expected := []string{
		"hello.txt",
	}

	entries := testArchive(t, r, errCh)
	if !reflect.DeepEqual(entries, expected) {
		t.Fatalf("bad: %#v", entries)
	}
}

func testArchive(t *testing.T, r io.ReadCloser, errCh <-chan error) []string {
	// Finish the archiving process in-memory
	var buf bytes.Buffer
	if _, err := io.Copy(&buf, r); err != nil {
		t.Fatalf("err: %s", err)
	}
	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("err: %s", err)
		}
	default:
	}

	gzipR, err := gzip.NewReader(&buf)
	if err != nil {
		t.Fatalf("err: %s", err)
	}
	tarR := tar.NewReader(gzipR)

	// Read all the entries
	result := make([]string, 0, 5)
	for {
		hdr, err := tarR.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("err: %s", err)
		}

		result = append(result, hdr.Name)
	}

	sort.Strings(result)
	return result
}

func tempFile(t *testing.T) string {
	tf, err := ioutil.TempFile("", "test")
	if err != nil {
		t.Fatalf("err: %s", err)
	}
	defer tf.Close()

	return tf.Name()
}

func testFixture(n string) string {
	return filepath.Join(fixturesDir, n)
}
