package config

import (
	"os"
	"path/filepath"
	"testing"

	"strike-core/internal/style"
)

func TestDefault(t *testing.T) {
	c := Default()
	if c.Version == "" {
		t.Error("default config missing identity fields")
	}
	if c.Brightness != 0.35 {
		t.Errorf("brightness = %v, want 0.35", c.Brightness)
	}
	if len(c.AsciiArt) != 3 {
		t.Errorf("ascii art rows = %d, want 3", len(c.AsciiArt))
	}
}

func TestLoadEmptyPathReturnsDefault(t *testing.T) {
	c, err := Load("", t.TempDir())
	if err != nil {
		t.Fatalf("Load(\"\") err = %v", err)
	}
	if c.Version != Default().Version {
		t.Error("empty path should yield defaults")
	}
}

func TestLoadMissingFileReturnsDefaults(t *testing.T) {
	c, err := Load(filepath.Join(t.TempDir(), "nope.json"), "")
	if err != nil {
		t.Fatalf("Load err = %v", err)
	}
	if c.Version != Default().Version {
		t.Error("missing file should yield defaults")
	}
}

func TestLoadOverlay(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "cfg.json")
	json := `{"version":"test-v1","brightness":0.5,"theme":{"hint_fg":"#ff8800"}}`
	if err := os.WriteFile(path, []byte(json), 0o644); err != nil {
		t.Fatal(err)
	}
	c, err := Load(path, "")
	if err != nil {
		t.Fatalf("Load err = %v", err)
	}
	if c.Version != "test-v1" {
		t.Errorf("version = %q, want test-v1", c.Version)
	}
	if c.Brightness != 0.5 {
		t.Errorf("brightness = %v, want 0.5", c.Brightness)
	}
	if c.Theme.HintFg != mustHex(t, "#ff8800") {
		t.Errorf("hint_fg not applied: %+v", c.Theme.HintFg)
	}
	// Unspecified fields keep defaults.
	if c.ModelName != Default().ModelName {
		t.Error("unspecified model_name should stay default")
	}
}

func TestLoadBadJSONErrors(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bad.json")
	if err := os.WriteFile(path, []byte("{not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(path, ""); err == nil {
		t.Error("malformed JSON should error")
	}
}

func TestLoadBadColorErrors(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "badcolor.json")
	if err := os.WriteFile(path, []byte(`{"theme":{"art_left":"xyz"}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(path, ""); err == nil {
		t.Error("invalid hex color should error")
	}
}

func mustHex(t *testing.T, s string) style.Color {
	t.Helper()
	c, err := style.ParseHex(s)
	if err != nil {
		t.Fatal(err)
	}
	return c
}
