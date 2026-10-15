# 🐾 BosyCat

**BosyCat** is a lightweight system activity tracker and monitoring tool for Linux, leveraging **eBPF** for low-overhead kernel event tracing, coupled with a Go backend and a dedicated Python GUI.

---

## 🚀 Features

- **Kernel-level Monitoring:** Powered by eBPF (`C` + `cilium/ebpf`) for efficient, non-intrusive event tracking.
- **Go Backend Service:** High-performance daemon (`bosycat-tracker`) managing kernel events and data flow.
- **Desktop Interface:** Intuitive Python-based UI (`BosyCatUI`) for visualizing events in real-time.
- **Native Packaging:** Out-of-the-box build support for both RPM (`.rpm` for Fedora/RHEL) and Debian (`.deb` for Ubuntu/Debian) distributions.
- **systemd Integration:** Seamless background execution via `bosycat.service`.

---

## 📁 Repository Structure

```text
BosyCat/
├── tracker.c               # eBPF kernel C program
├── vmlinux.h               # Kernel headers for BPF CO-RE
├── main.go                 # Core Go service logic
├── tracker_bpf*.go         # bpf2go generated bindings
├── main_ui.py              # Desktop UI entry point
├── BosyCatUI.spec          # PyInstaller packaging specification
├── build_pkg/              # Packaging staging & metadata (RPM/DEB)
│   ├── deb/                # Debian control files and staging tree
│   └── rpm/                # RPM specs and directory skeleton
├── package.sh              # Unified build and packaging automation script
└── cat-silhouette-01.svg   # Application branding & icons

```

---

## 🛠 Prerequisites

Ensure you have the following installed on your host system:

* **Linux Kernel** ≥ 5.8 with BPF CO-RE enabled (`CONFIG_DEBUG_INFO_BTF=y`)
* **Go** ≥ 1.20
* **Python** ≥ 3.10
* **Clang / LLVM** (for compiling eBPF programs)
* **bpftool**
* Packaging tools: `rpmbuild` (for RPMs) and `dpkg-deb` (for DEBs)

---

## ⚡ Building from Source

### 1. Compile eBPF & Go Tracker

Generate the eBPF bytecode and Go scaffolding using `bpf2go`:

```bash
go generate ./...
go build -o bosycat-tracker main.go tracker_bpfel.go

```

### 2. Build the Desktop UI

Freeze the Python interface into a standalone binary using PyInstaller:

```bash
pip install -r requirements.txt  # If requirements.txt is used
pyinstaller BosyCatUI.spec

```

---

## 📦 Packaging

To build native release packages (`.deb` and `.rpm`):

```bash
chmod +x package.sh
./package.sh

```

Compiled packages will be generated inside the project root:

* `bosycat_<version>_amd64.deb`
* `bosycat-<version>-1.<distro>.x86_64.rpm`
