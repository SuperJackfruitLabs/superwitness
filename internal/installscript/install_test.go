// Package installscript tests scripts/install.sh against a local server that mimics GitHub's
// release layout. The script is run for real (bash + curl + tar); only the network is fake.
package installscript

import (
	"archive/tar"
	"compress/gzip"
	"crypto/sha256"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

const tag = "v9.9.9"

func hostPlatform() (string, string) {
	arch := runtime.GOARCH
	return runtime.GOOS, arch
}

// stub is a fake binary: a shell script that prints the tag and carries the module path the way a
// real Go build does.
func stub(version string) string {
	return "#!/bin/sh\n# github.com/SuperJackfruitLabs/superwitness\necho " + version + "\n"
}

func tarball(t *testing.T, name, body string) []byte {
	t.Helper()
	var sb strings.Builder
	gz := gzip.NewWriter(&sb)
	tw := tar.NewWriter(gz)
	dir := strings.TrimSuffix(name, ".tar.gz")
	if err := tw.WriteHeader(&tar.Header{Name: dir + "/superwitness", Mode: 0o755, Size: int64(len(body))}); err != nil {
		t.Fatal(err)
	}
	tw.Write([]byte(body))
	tw.Close()
	gz.Close()
	return []byte(sb.String())
}

type release struct {
	tag      string
	files    map[string][]byte
	noSums   bool
	badSum   bool
	skipAsst bool // list in SHA256SUMS but do not serve the tarball
}

func newRelease(t *testing.T, tg string) *release {
	os, arch := hostPlatform()
	name := fmt.Sprintf("superwitness_%s_%s_%s.tar.gz", strings.TrimPrefix(tg, "v"), os, arch)
	return &release{tag: tg, files: map[string][]byte{name: tarball(t, name, stub(tg))}}
}

func (r *release) serve(t *testing.T) string {
	mux := http.NewServeMux()
	mux.HandleFunc("/latest", func(w http.ResponseWriter, q *http.Request) {
		http.Redirect(w, q, "/tag/"+r.tag, http.StatusFound)
	})
	mux.HandleFunc("/tag/", func(w http.ResponseWriter, q *http.Request) { w.Write([]byte("release page")) })
	mux.HandleFunc("/download/"+r.tag+"/", func(w http.ResponseWriter, q *http.Request) {
		f := filepath.Base(q.URL.Path)
		if f == "SHA256SUMS" {
			if r.noSums {
				http.NotFound(w, q)
				return
			}
			var sb strings.Builder
			for n, b := range r.files {
				sum := fmt.Sprintf("%x", sha256.Sum256(b))
				if r.badSum {
					sum = strings.Repeat("0", 64)
				}
				fmt.Fprintf(&sb, "%s  %s\n", sum, n)
			}
			w.Write([]byte(sb.String()))
			return
		}
		b, ok := r.files[f]
		if !ok || r.skipAsst {
			http.NotFound(w, q)
			return
		}
		w.Write(b)
	})
	s := httptest.NewServer(mux)
	t.Cleanup(s.Close)
	return s.URL
}

func script(t *testing.T) string {
	p, err := filepath.Abs("../../scripts/install.sh")
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func run(t *testing.T, base, binDir string, env ...string) (string, error) {
	t.Helper()
	cmd := exec.Command("bash", script(t))
	cmd.Env = append(os.Environ(), "SUPERWITNESS_RELEASE_BASE="+base, "BIN_DIR="+binDir, "VERSION=")
	cmd.Env = append(cmd.Env, env...)
	out, err := cmd.CombinedOutput()
	return string(out), err
}

func runOut(t *testing.T, path string, arg string) string {
	t.Helper()
	b, err := exec.Command(path, arg).Output()
	if err != nil {
		t.Fatalf("%s %s: %v", path, arg, err)
	}
	return strings.TrimSpace(string(b))
}

func TestInstallsBothNames(t *testing.T) {
	base := newRelease(t, tag).serve(t)
	dir := t.TempDir()
	out, err := run(t, base, dir)
	if err != nil {
		t.Fatalf("install failed: %v\n%s", err, out)
	}
	if !strings.Contains(out, "Installed "+tag) || !strings.Contains(out, "no service") {
		t.Fatalf("unexpected output:\n%s", out)
	}
	sw, suwi := runOut(t, filepath.Join(dir, "superwitness"), "--version"), runOut(t, filepath.Join(dir, "suwi"), "--version")
	if sw != tag || suwi != sw {
		t.Fatalf("versions: superwitness=%q suwi=%q", sw, suwi)
	}
	if l, _ := os.Readlink(filepath.Join(dir, "suwi")); l != "superwitness" {
		t.Fatalf("suwi should link to superwitness, got %q", l)
	}
}

func TestPinnedVersion(t *testing.T) {
	base := newRelease(t, tag).serve(t)
	dir := t.TempDir()
	if out, err := run(t, base, dir, "VERSION="+tag); err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
}

func assertNothingInstalled(t *testing.T, dir string) {
	t.Helper()
	ents, _ := os.ReadDir(dir)
	if len(ents) != 0 {
		t.Fatalf("expected empty BIN_DIR, found %v", ents)
	}
}

func TestWrongChecksumRefuses(t *testing.T) {
	r := newRelease(t, tag)
	r.badSum = true
	dir := t.TempDir()
	out, err := run(t, r.serve(t), dir)
	if err == nil || !strings.Contains(out, "failed its checksum") {
		t.Fatalf("want checksum refusal, got err=%v\n%s", err, out)
	}
	assertNothingInstalled(t, dir)
}

func TestMissingSumsRefuses(t *testing.T) {
	r := newRelease(t, tag)
	r.noSums = true
	dir := t.TempDir()
	out, err := run(t, r.serve(t), dir)
	if err == nil || !strings.Contains(out, "no SHA256SUMS") {
		t.Fatalf("want SHA256SUMS refusal, got err=%v\n%s", err, out)
	}
	assertNothingInstalled(t, dir)
}

func TestMissingPlatformAssetSaysSo(t *testing.T) {
	r := newRelease(t, tag)
	r.files = map[string][]byte{"superwitness_9.9.9_plan9_mips.tar.gz": []byte("x")}
	dir := t.TempDir()
	out, err := run(t, r.serve(t), dir)
	os_, arch := hostPlatform()
	if err == nil || !strings.Contains(out, "has no "+os_+"/"+arch+" build") {
		t.Fatalf("want platform message, got err=%v\n%s", err, out)
	}
	assertNothingInstalled(t, dir)
}

func TestStrangerInTheWayRefuses(t *testing.T) {
	base := newRelease(t, tag).serve(t)
	for _, name := range []string{"superwitness", "suwi"} {
		dir := t.TempDir()
		stranger := filepath.Join(dir, name)
		os.WriteFile(stranger, []byte("#!/bin/sh\necho mine\n"), 0o755)
		out, err := run(t, base, dir)
		if err == nil || !strings.Contains(out, "refusing to overwrite") {
			t.Fatalf("%s: want refusal, got err=%v\n%s", name, err, out)
		}
		if b, _ := os.ReadFile(stranger); string(b) != "#!/bin/sh\necho mine\n" {
			t.Fatalf("%s: stranger was modified", name)
		}
		ents, _ := os.ReadDir(dir)
		if len(ents) != 1 {
			t.Fatalf("%s: installed something beside the stranger: %v", name, ents)
		}
	}
}

func TestStrangerSymlinkAtSuwiRefuses(t *testing.T) {
	base := newRelease(t, tag).serve(t)
	dir := t.TempDir()
	os.Symlink("/bin/ls", filepath.Join(dir, "suwi"))
	out, err := run(t, base, dir)
	if err == nil || !strings.Contains(out, "refusing to overwrite") {
		t.Fatalf("want refusal, got err=%v\n%s", err, out)
	}
}

func TestRerunUpgrades(t *testing.T) {
	dir := t.TempDir()
	for _, v := range []string{"v9.9.8", "v9.9.9"} {
		out, err := run(t, newRelease(t, v).serve(t), dir)
		if err != nil {
			t.Fatalf("%s: %v\n%s", v, err, out)
		}
		if got := runOut(t, filepath.Join(dir, "suwi"), "--version"); got != v {
			t.Fatalf("after installing %s, suwi reports %s", v, got)
		}
	}
}
