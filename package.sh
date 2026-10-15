#!/bin/bash
set -e

echo "🚀 Starting BosyCat Build & Packaging Process..."

# 1. بناء أحدث نسخة من الـ Backend (Go + eBPF)
echo "🔨 Compiling Go eBPF tracker..."
go generate
go build -o bosycat-tracker

# 2. بناء أحدث نسخة من الواجهة الرسومية (PyInstaller) إذا كان ملف spec موجوداً
if [ -f "BosyCatUI.spec" ] && command -v pyinstaller >/dev/null 2>&1; then
    echo "🔨 Compiling Python UI..."
    pyinstaller --clean -y BosyCatUI.spec > /dev/null
fi

if [ ! -f "bosycat-tracker" ] || [ ! -f "dist/BosyCatUI" ] || [ ! -f "cat-silhouette-01.svg" ]; then
    echo "❌ Error: Binaries or Icon not found!"
    exit 1
fi

VERSION="1.0.0"
PKG_NAME="bosycat"
BUILD_DIR="build_pkg"

# تنظيف وبناء الهيكل
rm -rf ${BUILD_DIR}
mkdir -p ${BUILD_DIR}/deb/DEBIAN
mkdir -p ${BUILD_DIR}/deb/opt/BosyCat
mkdir -p ${BUILD_DIR}/deb/usr/share/applications
mkdir -p ${BUILD_DIR}/deb/usr/share/icons/hicolor/scalable/apps
mkdir -p ${BUILD_DIR}/deb/usr/share/pixmaps
mkdir -p ${BUILD_DIR}/deb/usr/share/metainfo
mkdir -p ${BUILD_DIR}/deb/usr/lib/systemd/system

echo "📁 Assembling files..."
cp bosycat-tracker ${BUILD_DIR}/deb/opt/BosyCat/
cp dist/BosyCatUI ${BUILD_DIR}/deb/opt/BosyCat/
cp cat-silhouette-01.svg ${BUILD_DIR}/deb/opt/BosyCat/

# إرسال الأيقونة لمسارات النظام عشان الـ Software Center يشوفها
cp cat-silhouette-01.svg ${BUILD_DIR}/deb/usr/share/icons/hicolor/scalable/apps/bosycat.svg
cp cat-silhouette-01.svg ${BUILD_DIR}/deb/usr/share/pixmaps/bosycat.svg

# إنشاء ملف الاختصار
cat <<EOF > ${BUILD_DIR}/deb/usr/share/applications/bosycat.desktop
[Desktop Entry]
Name=BosyCat
Comment=eBPF Network Traffic Monitor
Exec=/opt/BosyCat/BosyCatUI
Icon=bosycat
Terminal=false
Type=Application
Categories=Network;Utility;System;
StartupWMClass=BosyCatUI
EOF

# إنشاء ملف AppStream MetaInfo لمتجر برامج اللينكس
cat <<EOF > ${BUILD_DIR}/deb/usr/share/metainfo/bosycat.metainfo.xml
<?xml version="1.0" encoding="UTF-8"?>
<component type="desktop-application">
  <id>bosycat.desktop</id>
  <metadata_license>CC0-1.0</metadata_license>
  <project_license>GPL-3.0</project_license>
  <name>BosyCat</name>
  <summary>eBPF Network Traffic Monitor</summary>
  <description>
    <p>BosyCat is a lightweight, low-overhead network traffic monitor using eBPF technology. It provides per-application bandwidth usage with a clean and simple UI.</p>
  </description>
  <icon type="stock">bosycat</icon>
  <categories>
    <category>Network</category>
    <category>Utility</category>
  </categories>
</component>
EOF

# إعداد خدمة النظام مع كامل الصلاحيات اللازمة لـ eBPF وقراءة /proc
cat <<EOF > ${BUILD_DIR}/deb/usr/lib/systemd/system/bosycat.service
[Unit]
Description=BosyCat eBPF Network Tracker
After=network.target

[Service]
Type=simple
User=root
AmbientCapabilities=CAP_SYS_ADMIN CAP_NET_ADMIN CAP_BPF CAP_PERFMON CAP_SYS_RESOURCE CAP_SYS_PTRACE CAP_DAC_READ_SEARCH
CapabilityBoundingSet=CAP_SYS_ADMIN CAP_NET_ADMIN CAP_BPF CAP_PERFMON CAP_SYS_RESOURCE CAP_SYS_PTRACE CAP_DAC_READ_SEARCH
WorkingDirectory=/opt/BosyCat
ExecStart=/opt/BosyCat/bosycat-tracker
Restart=on-failure
RestartSec=5

[Install]
WantedBy=multi-user.target
EOF

# ==========================================
# 📦 بناء حزمة ديبيان وأوبونتو (.DEB)
# ==========================================
echo "📦 Building .deb package..."
cat <<EOF > ${BUILD_DIR}/deb/DEBIAN/control
Package: ${PKG_NAME}
Version: ${VERSION}
Section: net
Priority: optional
Architecture: amd64
Maintainer: Developer
Description: BosyCat Network Traffic Monitor using eBPF
EOF

cat <<EOF > ${BUILD_DIR}/deb/DEBIAN/postinst
#!/bin/bash
chmod 777 /opt/BosyCat
chmod 755 /opt/BosyCat/bosycat-tracker
chmod 755 /opt/BosyCat/BosyCatUI
gtk-update-icon-cache -f -t /usr/share/icons/hicolor || true
if command -v chcon >/dev/null 2>&1; then
    chcon -t bin_t /opt/BosyCat/bosycat-tracker || true
fi
systemctl daemon-reload
systemctl enable bosycat.service
systemctl restart bosycat.service
EOF
chmod +x ${BUILD_DIR}/deb/DEBIAN/postinst

cat <<EOF > ${BUILD_DIR}/deb/DEBIAN/prerm
#!/bin/bash
systemctl stop bosycat.service || true
systemctl disable bosycat.service || true
EOF
chmod +x ${BUILD_DIR}/deb/DEBIAN/prerm

# 🚀 سكربت المسح الكلي للداتابيز بعد الـ Uninstall في ديبيان
cat <<EOF > ${BUILD_DIR}/deb/DEBIAN/postrm
#!/bin/bash
if [ "\$1" = "remove" ] || [ "\$1" = "purge" ]; then
    rm -rf /opt/BosyCat
    systemctl daemon-reload
fi
EOF
chmod +x ${BUILD_DIR}/deb/DEBIAN/postrm

dpkg-deb --root-owner-group --build ${BUILD_DIR}/deb ${PKG_NAME}_${VERSION}_amd64.deb > /dev/null

# ==========================================
# 📦 بناء حزمة فيدورا وريدهات (.RPM)
# ==========================================
echo "📦 Building .rpm package..."
RPM_ROOT="${BUILD_DIR}/rpm"
mkdir -p ${RPM_ROOT}/{BUILD,BUILDROOT,RPMS,SOURCES,SPECS,SRPMS}

SRC_DIR="${RPM_ROOT}/SOURCES/${PKG_NAME}-${VERSION}"
mkdir -p ${SRC_DIR}
cp -r ${BUILD_DIR}/deb/opt ${SRC_DIR}/
cp -r ${BUILD_DIR}/deb/usr ${SRC_DIR}/

tar -czf ${RPM_ROOT}/SOURCES/${PKG_NAME}-${VERSION}.tar.gz -C ${RPM_ROOT}/SOURCES ${PKG_NAME}-${VERSION}

cat <<EOF > ${RPM_ROOT}/SPECS/${PKG_NAME}.spec
%define debug_package %{nil}
Name:           ${PKG_NAME}
Version:        ${VERSION}
Release:        1%{?dist}
Summary:        Network Traffic Monitor using eBPF
License:        GPL
Source0:        %{name}-%{version}.tar.gz
AutoReqProv:    no

%description
BosyCat Network Traffic Monitor using eBPF.

%prep
%setup -q

%install
rm -rf %{buildroot}
mkdir -p %{buildroot}
cp -r opt %{buildroot}/
cp -r usr %{buildroot}/

%post
chmod 777 /opt/BosyCat
chmod 755 /opt/BosyCat/bosycat-tracker
chmod 755 /opt/BosyCat/BosyCatUI
gtk-update-icon-cache -f -t /usr/share/icons/hicolor || true
if command -v chcon >/dev/null 2>&1; then
    chcon -t bin_t /opt/BosyCat/bosycat-tracker || true
fi
systemctl daemon-reload
systemctl enable bosycat.service
systemctl restart bosycat.service

%preun
if [ \$1 -eq 0 ]; then
    systemctl stop bosycat.service || true
    systemctl disable bosycat.service || true
fi

%postun
# 🚀 المسح الكلي للداتابيز بعد الـ Uninstall في فيدورا
if [ \$1 -eq 0 ]; then
    rm -rf /opt/BosyCat
    systemctl daemon-reload
fi

%files
/opt/BosyCat/
/usr/share/applications/bosycat.desktop
/usr/share/icons/hicolor/scalable/apps/bosycat.svg
/usr/share/pixmaps/bosycat.svg
/usr/share/metainfo/bosycat.metainfo.xml
/usr/lib/systemd/system/bosycat.service
EOF

rpmbuild --define "_topdir $PWD/${RPM_ROOT}" -bb ${RPM_ROOT}/SPECS/${PKG_NAME}.spec > /dev/null
mv ${RPM_ROOT}/RPMS/x86_64/${PKG_NAME}-${VERSION}-1*.rpm .

echo "✅ DONE! Smart Packages created successfully:"
ls -lh *.deb *.rpm