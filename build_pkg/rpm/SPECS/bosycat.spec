%define debug_package %{nil}
Name:           bosycat
Version:        1.0.0
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
if [ $1 -eq 0 ]; then
    systemctl stop bosycat.service || true
    systemctl disable bosycat.service || true
fi

%postun
# 🚀 المسح الكلي للداتابيز بعد الـ Uninstall في فيدورا
if [ $1 -eq 0 ]; then
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
