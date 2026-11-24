package cmd

import (
	"os"
	"path/filepath"
	"testing"
)

func TestInstallerCommitFromRefsUsesDefaultHead(t *testing.T) {
	got, err := installerCommitFromRefs("deadbeef\tHEAD\n", "")
	if err != nil || got != "deadbeef" {
		t.Fatalf("installerCommitFromRefs() = %q, %v", got, err)
	}
}

func TestInstallerCommitFromRefsUsesPinnedBranch(t *testing.T) {
	output := "1111111\trefs/tags/release\n2222222\trefs/heads/release\n"
	got, err := installerCommitFromRefs(output, "release")
	if err != nil || got != "2222222" {
		t.Fatalf("installerCommitFromRefs() = %q, %v", got, err)
	}
}

func TestInstallerCommitFromRefsPeelsAnnotatedTag(t *testing.T) {
	output := "1111111\trefs/tags/v1\n2222222\trefs/tags/v1^{}\n"
	got, err := installerCommitFromRefs(output, "refs/tags/v1")
	if err != nil || got != "2222222" {
		t.Fatalf("installerCommitFromRefs() = %q, %v", got, err)
	}
}

func TestResolveInstallerCommitAcceptsFullHash(t *testing.T) {
	ref := "0123456789ABCDEF0123456789ABCDEF01234567"
	got, err := resolveInstallerCommit("https://example.invalid/app_tools.git", ref)
	if err != nil || got != "0123456789abcdef0123456789abcdef01234567" {
		t.Fatalf("resolveInstallerCommit() = %q, %v", got, err)
	}
}

func TestInstallerPatchDigestChangesWithContents(t *testing.T) {
	patch := filepath.Join(t.TempDir(), "installer.patch")
	if err := os.WriteFile(patch, []byte("first"), 0o644); err != nil {
		t.Fatal(err)
	}
	first, err := installerPatchDigest([]string{patch})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(patch, []byte("second"), 0o644); err != nil {
		t.Fatal(err)
	}
	second, err := installerPatchDigest([]string{patch})
	if err != nil {
		t.Fatal(err)
	}
	if first == second {
		t.Fatal("patch digest did not change with patch contents")
	}
}

func TestInstallerCacheMatchesExactInputs(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	want := installerCache{URL: "example", Commit: "deadbeef", PatchDigest: "digest"}
	if err := os.WriteFile(installerCachePath(dir), []byte(`{"url":"example","commit":"deadbeef","patch_digest":"digest"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if !installerCacheMatches(dir, want) {
		t.Fatal("matching cache metadata was rejected")
	}
	want.Commit = "cafebabe"
	if installerCacheMatches(dir, want) {
		t.Fatal("stale cache metadata was accepted")
	}
}
