package main

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestLinuxBootSelectsEarlySeedAndExplicitClock(t *testing.T) {
	config := linuxPlist("test-uuid", time.Date(2026, 9, 9, 12, 0, 0, 0, time.UTC))
	for _, required := range []string{"ds=nocloud", "console=ttyAMA0,115200", "base=2026-09-09T12:00:00,clock=host", "<string>seed.iso</string><key>ImageType</key><string>Disk</string><key>Interface</key><string>VirtIO</string>"} {
		if !strings.Contains(config, required) {
			t.Fatalf("missing proven boot prerequisite: %s", required)
		}
	}
	seed := cloudUserData(time.Date(2026, 9, 9, 7, 0, 0, 0, time.FixedZone("local", -5*3600)))
	if strings.Contains(seed, "@BOOT_TIME@") || !strings.Contains(seed, `- [date, --utc, --set, "2026-09-09T12:00:00Z"]`) {
		t.Fatal("cloud-init must repair the guest clock before fetching authenticated packages")
	}
}

func TestLinuxSeedActivatesAccessKitThroughScreenReaderProperty(t *testing.T) {
	seed := cloudUserData(time.Unix(0, 0))
	if !strings.Contains(seed, "org.freedesktop.DBus.Properties.Set org.a11y.Status ScreenReaderEnabled") {
		t.Fatal("IsEnabled/toolkit-accessibility alone does not activate AccessKit AT-SPI registration")
	}
}
func TestLinuxSeedUsesDisplayLargeEnoughForScaledBank(t *testing.T) {
	if !strings.Contains(cloudConfig, "/usr/bin/xrandr --output Virtual-1 --mode 1440x900") {
		t.Fatal("default 1280x800 display clips the actual scaled bank into the bottom panel")
	}
}

func TestVerifyFileRejectsMismatch(t *testing.T) {
	path := filepath.Join(t.TempDir(), "asset")
	if err := os.WriteFile(path, []byte("official image bytes"), 0600); err != nil {
		t.Fatal(err)
	}
	hash := sha256.Sum256([]byte("official image bytes"))
	if err := verifyFile(path, hex.EncodeToString(hash[:])); err != nil {
		t.Fatal(err)
	}
	if err := verifyFile(path, strings.Repeat("0", 64)); err == nil {
		t.Fatal("tampered image was accepted")
	}
}

func TestExtractRootRefusesTraversalLinksAndInvalidFilesystem(t *testing.T) {
	for _, tc := range []struct {
		name     string
		typeflag byte
	}{
		{"../escape.img", tar.TypeReg},
		{rootName, tar.TypeSymlink},
		{rootName, tar.TypeReg},
	} {
		t.Run(tc.name+string(tc.typeflag), func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "asset.tar.gz")
			f, err := os.Create(path)
			if err != nil {
				t.Fatal(err)
			}
			gz := gzip.NewWriter(f)
			tw := tar.NewWriter(gz)
			h := tar.Header{Name: tc.name, Typeflag: tc.typeflag, Size: 1 << 20, Mode: 0600}
			if tc.typeflag == tar.TypeSymlink {
				h.Size = 0
				h.Linkname = "outside"
			}
			if err = tw.WriteHeader(&h); err != nil {
				t.Fatal(err)
			}
			if h.Size != 0 {
				if _, err = tw.Write(make([]byte, h.Size)); err != nil {
					t.Fatal(err)
				}
			}
			if err = tw.Close(); err != nil {
				t.Fatal(err)
			}
			if err = gz.Close(); err != nil {
				t.Fatal(err)
			}
			if err = f.Close(); err != nil {
				t.Fatal(err)
			}
			if err = extractRoot(path, filepath.Join(dir, "root.img")); err == nil {
				t.Fatal("unsafe or invalid archive accepted")
			}
		})
	}
}

func TestPrepareNeverOverwritesExistingVM(t *testing.T) {
	path := filepath.Join(t.TempDir(), "lab.utm")
	if err := os.Mkdir(path, 0700); err != nil {
		t.Fatal(err)
	}
	if err := prepareLinux(context.Background(), path, "", false); err == nil {
		t.Fatal("existing VM accepted")
	}
}
