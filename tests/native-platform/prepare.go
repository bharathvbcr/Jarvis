package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

const ubuntuBase = "https://cloud-images.ubuntu.com/noble/20260826/"
const archiveSHA = "6a0c8d75491988f7a51d443be6a8b36455b53684fa87245d1478d5b1643bbbbd"
const kernelSHA = "a6c429cb79db29b987d138d1e8b2a6c9f0bbad28023145e2db7fe95505e96c9d"
const initrdSHA = "f6082d78117fbfc35ccf2aebc7d48b878cbcfe4f97bfa7f40acab48b46569b18"
const rootName = "noble-server-cloudimg-arm64.img"

func verifyFile(path, expected string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	h := sha256.New()
	if _, err = io.Copy(h, f); err != nil {
		return err
	}
	if got := hex.EncodeToString(h.Sum(nil)); got != expected {
		return fmt.Errorf("SHA256 mismatch for %s: got %s", filepath.Base(path), got)
	}
	return nil
}
func download(ctx context.Context, url, path, sha string, limit int64) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	client := http.Client{Timeout: 15 * time.Minute, CheckRedirect: func(req *http.Request, via []*http.Request) error {
		if len(via) > 3 || req.URL.Scheme != "https" || req.URL.Host != "cloud-images.ubuntu.com" {
			return errors.New("download redirect left the approved Ubuntu origin")
		}
		return nil
	}}
	response, err := client.Do(req)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("Ubuntu download status %d", response.StatusCode)
	}
	if response.ContentLength > limit {
		return errors.New("download exceeds byte limit")
	}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return err
	}
	count, copyErr := io.Copy(f, io.LimitReader(response.Body, limit+1))
	closeErr := f.Close()
	if copyErr != nil {
		return copyErr
	}
	if closeErr != nil {
		return closeErr
	}
	if count > limit {
		return errors.New("download exceeds byte limit")
	}
	return verifyFile(path, sha)
}
func extractRoot(archive, destination string) error {
	f, err := os.Open(archive)
	if err != nil {
		return err
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		return err
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	found := false
	for count := 0; count < 16; count++ {
		h, err := tr.Next()
		if errors.Is(err, io.EOF) {
			if !found {
				return errors.New("archive has no root image")
			}
			return nil
		}
		if err != nil {
			return err
		}
		if h.Name == "README" {
			if h.Size > 64<<10 {
				return errors.New("oversized archive README")
			}
			continue
		}
		if h.Name != rootName || found || h.Typeflag != tar.TypeReg || h.Size < 1<<20 || h.Size > 4<<30 {
			return errors.New("unexpected archive member; extraction refused")
		}
		out, err := os.OpenFile(destination, os.O_RDWR|os.O_CREATE|os.O_EXCL, 0600)
		if err != nil {
			return err
		}
		n, copyErr := io.CopyN(out, tr, h.Size)
		if copyErr == nil && n == h.Size {
			var magic [2]byte
			if _, copyErr = out.ReadAt(magic[:], 1080); copyErr == nil && magic != [2]byte{0x53, 0xef} {
				copyErr = errors.New("expected a raw ext4 root filesystem")
			}
		}
		if copyErr == nil {
			copyErr = out.Truncate(64 << 30)
		} // sparse extension; cloud-init resizefs grows ext4 inside the guest
		closeErr := out.Close()
		if copyErr != nil {
			return copyErr
		}
		if closeErr != nil {
			return closeErr
		}
		found = true
	}
	return errors.New("archive member count limit exceeded")
}
func uuid() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	b[6] = (b[6] & 15) | 64
	b[8] = (b[8] & 63) | 128
	return fmt.Sprintf("%X-%X-%X-%X-%X", b[:4], b[4:6], b[6:8], b[8:10], b[10:]), nil
}
func xmlString(value string) string {
	var b bytes.Buffer
	_ = xml.EscapeText(&b, []byte(value))
	return b.String()
}
func command(ctx context.Context, name string, args ...string) error {
	child := exec.CommandContext(ctx, name, args...)
	child.Stdout = os.Stdout
	child.Stderr = os.Stderr
	return child.Run()
}
func prepareLinux(ctx context.Context, bundle, archive string, register bool) error {
	if filepath.Ext(bundle) != ".utm" {
		return errors.New("output must have .utm extension")
	}
	if _, err := os.Lstat(bundle); !errors.Is(err, os.ErrNotExist) {
		return errors.New("bundle already exists or cannot be inspected; choose a fresh path")
	}
	if err := os.MkdirAll(filepath.Dir(bundle), 0700); err != nil {
		return err
	}
	staging := bundle + ".preparing"
	if err := os.Mkdir(staging, 0700); err != nil {
		return fmt.Errorf("create staging bundle: %w", err)
	}
	// On failure preserve only this named staging directory for inspectable recovery.
	data := filepath.Join(staging, "Data")
	if err := os.Mkdir(data, 0700); err != nil {
		return err
	}
	if archive == "" {
		archive = filepath.Join(staging, "ubuntu.tar.gz")
		fmt.Println("Downloading pinned Ubuntu ARM64 root archive (511 MiB)")
		if err := download(ctx, ubuntuBase+"noble-server-cloudimg-arm64.tar.gz", archive, archiveSHA, 600<<20); err != nil {
			return err
		}
	}
	if err := verifyFile(archive, archiveSHA); err != nil {
		return err
	}
	fmt.Println("Extracting verified raw ext4 image; extending its sparse capacity to 64 GiB")
	if err := extractRoot(archive, filepath.Join(data, "root.img")); err != nil {
		return err
	}
	if err := download(ctx, ubuntuBase+"unpacked/noble-server-cloudimg-arm64-vmlinuz-generic", filepath.Join(data, "vmlinuz"), kernelSHA, 64<<20); err != nil {
		return err
	}
	if err := download(ctx, ubuntuBase+"unpacked/noble-server-cloudimg-arm64-initrd-generic", filepath.Join(data, "initrd"), initrdSHA, 64<<20); err != nil {
		return err
	}
	seed := filepath.Join(staging, "seed")
	if err := os.Mkdir(seed, 0700); err != nil {
		return err
	}
	id, err := uuid()
	if err != nil {
		return err
	}
	preparedAt := time.Now().UTC()
	if err = os.WriteFile(filepath.Join(seed, "meta-data"), []byte("instance-id: jarvis-"+id+"\nlocal-hostname: jarvis-linux-x11\n"), 0600); err != nil {
		return err
	}
	if err = os.WriteFile(filepath.Join(seed, "user-data"), []byte(cloudUserData(preparedAt)), 0600); err != nil {
		return err
	}
	if err = os.WriteFile(filepath.Join(seed, "network-config"), []byte(networkConfig), 0600); err != nil {
		return err
	}
	if err = command(ctx, "hdiutil", "makehybrid", "-iso", "-joliet", "-default-volume-name", "CIDATA", "-o", filepath.Join(data, "seed.iso"), seed); err != nil {
		return fmt.Errorf("NoCloud seed ISO: %w", err)
	}
	if err = os.WriteFile(filepath.Join(staging, "config.plist"), []byte(linuxPlist(id, preparedAt)), 0600); err != nil {
		return err
	}
	if err = command(ctx, "plutil", "-lint", filepath.Join(staging, "config.plist")); err != nil {
		return err
	}
	provenance := struct {
		SchemaVersion                                                                    int `json:"schema_version"`
		Platform, ImageURL, ImageSHA256, KernelSHA256, InitrdSHA256, Qualification, UUID string
	}{1, "linux-arm64-x11", ubuntuBase + "noble-server-cloudimg-arm64.tar.gz", archiveSHA, kernelSHA, initrdSHA, "not_booted", id}
	raw, err := json.MarshalIndent(provenance, "", "  ")
	if err != nil {
		return err
	}
	if err = os.WriteFile(filepath.Join(staging, "preparation.json"), raw, 0600); err != nil {
		return err
	}
	if err = os.Rename(staging, bundle); err != nil {
		return err
	}
	fmt.Println("Prepared", bundle, "UUID", id, "(not yet booted or qualified)")
	if register {
		return command(ctx, "open", "-g", bundle)
	}
	return nil
}

func linuxPlist(id string, rtc time.Time) string {
	return fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0"><dict>
<key>Backend</key><string>QEMU</string><key>ConfigurationVersion</key><integer>4</integer>
<key>Information</key><dict><key>Name</key><string>Jarvis Linux X11</string><key>UUID</key><string>%s</string><key>IconCustom</key><false/><key>Notes</key><string>Ubuntu 24.04 ARM64; Xfce/X11; 6 GiB RAM; software rendering; native qualification pending.</string></dict>
<key>System</key><dict><key>Architecture</key><string>aarch64</string><key>Target</key><string>virt</string><key>CPU</key><string>default</string><key>CPUFlagsAdd</key><array/><key>CPUFlagsRemove</key><array/><key>CPUCount</key><integer>4</integer><key>ForceMulticore</key><false/><key>MemorySize</key><integer>6144</integer><key>JITCacheSize</key><integer>0</integer></dict>
<key>QEMU</key><dict><key>DebugLog</key><false/><key>UEFIBoot</key><false/><key>RNGDevice</key><true/><key>BalloonDevice</key><false/><key>TPMDevice</key><false/><key>Hypervisor</key><true/><key>TSO</key><false/><key>RTCLocalTime</key><false/><key>PS2Controller</key><false/><key>AdditionalArguments</key><array><string>-append</string><string>"root=/dev/vda rw ds=nocloud systemd.mask=systemd-timesyncd.service console=tty0 console=ttyAMA0,115200"</string><string>-rtc</string><string>base=%s,clock=host</string></array></dict>
<key>Input</key><dict><key>UsbBusSupport</key><string>3.0</string><key>UsbSharing</key><false/><key>MaximumUsbShare</key><integer>0</integer></dict>
<key>Sharing</key><dict><key>DirectoryShareMode</key><string>None</string><key>DirectoryShareReadOnly</key><true/><key>ClipboardSharing</key><false/></dict>
<key>Display</key><array><dict><key>Hardware</key><string>virtio-ramfb</string><key>DynamicResolution</key><false/><key>NativeResolution</key><false/><key>UpscalingFilter</key><string>Nearest</string><key>DownscalingFilter</key><string>Linear</string></dict></array>
<key>Drive</key><array>
<dict><key>ImageName</key><string>root.img</string><key>ImageType</key><string>Disk</string><key>Interface</key><string>VirtIO</string><key>InterfaceVersion</key><integer>1</integer><key>Identifier</key><string>root-disk</string><key>ReadOnly</key><false/></dict>
<dict><key>ImageName</key><string>vmlinuz</string><key>ImageType</key><string>LinuxKernel</string><key>Interface</key><string>None</string><key>Identifier</key><string>kernel</string><key>ReadOnly</key><true/></dict>
<dict><key>ImageName</key><string>initrd</string><key>ImageType</key><string>LinuxInitrd</string><key>Interface</key><string>None</string><key>Identifier</key><string>initrd</string><key>ReadOnly</key><true/></dict>
<dict><key>ImageName</key><string>seed.iso</string><key>ImageType</key><string>Disk</string><key>Interface</key><string>VirtIO</string><key>InterfaceVersion</key><integer>1</integer><key>Identifier</key><string>seed</string><key>ReadOnly</key><true/></dict>
</array>
<key>Network</key><array><dict><key>Mode</key><string>Emulated</string><key>Hardware</key><string>virtio-net-pci</string><key>MacAddress</key><string>52:54:00:4A:41:01</string><key>IsolateFromHost</key><false/><key>PortForward</key><array/></dict></array>
<key>Serial</key><array><dict><key>Mode</key><string>Ptty</string><key>Target</key><string>Auto</string></dict></array><key>Sound</key><array/>
</dict></plist>`, xmlString(id), rtc.UTC().Format("2006-01-02T15:04:05"))
}

// Match the generated NIC explicitly; cloud images can otherwise retain
// interface naming assumptions from their original cloud environment.
const networkConfig = `version: 2
ethernets:
  jarvisnet:
    match:
      macaddress: "52:54:00:4a:41:01"
    set-name: jarvisnet
    dhcp4: true
    dhcp6: false
    optional: true
`

func cloudUserData(preparedAt time.Time) string {
	return strings.ReplaceAll(cloudConfig, "@BOOT_TIME@", preparedAt.UTC().Format(time.RFC3339))
}

const cloudConfig = `#cloud-config
hostname: jarvis-linux-x11
bootcmd:
  - [date, --utc, --set, "@BOOT_TIME@"]
manage_etc_hosts: true
disable_root: true
ssh_pwauth: false
users:
  - name: jarvis
    gecos: Jarvis Synthetic Desktop Lab
    groups: [adm, sudo, video, input]
    sudo: ["ALL=(ALL) NOPASSWD:ALL"]
    shell: /bin/bash
    lock_passwd: true
package_update: true
package_upgrade: false
apt:
  preserve_sources_list: false
  primary:
    - arches: [default]
      uri: https://ports.ubuntu.com/ubuntu-ports
  security:
    - arches: [default]
      uri: https://ports.ubuntu.com/ubuntu-ports
  conf: |
    APT::Install-Recommends "false";
    APT::Install-Suggests "false";
    Acquire::Retries "3";
    Acquire::https::No-Cache "true";
packages:
  - qemu-guest-agent
  - xfce4
  - lightdm
  - xserver-xorg
  - dbus-x11
  - at-spi2-core
  - libatk-bridge2.0-0t64
  - libgl1-mesa-dri
  - mesa-utils
  - build-essential
  - pkg-config
  - libssl-dev
  - libx11-dev
  - libxi-dev
  - libxcursor-dev
  - libxrandr-dev
  - libxinerama-dev
  - libxkbcommon-dev
  - libegl1-mesa-dev
  - git
  - curl
  - ca-certificates
resize_rootfs: true
write_files:
  - path: /etc/lightdm/lightdm.conf.d/50-jarvis.conf
    permissions: '0644'
    content: |
      [Seat:*]
      autologin-user=jarvis
      autologin-user-timeout=0
      user-session=xfce
  - path: /etc/xdg/autostart/jarvis-accessibility.desktop
    permissions: '0644'
    content: |
      [Desktop Entry]
      Type=Application
      Name=Enable Native Accessibility
      Exec=gsettings set org.gnome.desktop.interface toolkit-accessibility true
      OnlyShowIn=XFCE;
  - path: /etc/xdg/autostart/jarvis-accesskit.desktop
    permissions: '0644'
    content: |
      [Desktop Entry]
      Type=Application
      Name=Activate AccessKit for the Synthetic Desktop Lab
      Exec=/usr/bin/gdbus call --session --dest org.a11y.Bus --object-path /org/a11y/bus --method org.freedesktop.DBus.Properties.Set org.a11y.Status ScreenReaderEnabled "<true>"
      OnlyShowIn=XFCE;
  - path: /etc/xdg/autostart/jarvis-display.desktop
    permissions: '0644'
    content: |
      [Desktop Entry]
      Type=Application
      Name=Size the Synthetic Desktop Lab
      Exec=/usr/bin/xrandr --output Virtual-1 --mode 1440x900
      OnlyShowIn=XFCE;
  - path: /etc/environment
    permissions: '0644'
    content: |
      LIBGL_ALWAYS_SOFTWARE=1
      GDK_BACKEND=x11
runcmd:
  - [apt-get, install, --yes, --no-install-recommends, qemu-guest-agent]
  - [systemctl, enable, --now, qemu-guest-agent]
  - [systemctl, set-default, graphical.target]
  - [systemctl, restart, lightdm]
final_message: "Jarvis provision attempt ended; inspect cloud-init status before claiming desktop readiness."
`
