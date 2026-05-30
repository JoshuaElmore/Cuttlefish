Name:           cuttlefish
Version:        %{pkg_version}
Release:        1%{?dist}
Summary:        Filesystem intelligence platform — indexer, aggregator, and API server
License:        Proprietary
BuildArch:      x86_64
Prefix:         /

Requires:       glibc
Requires:       systemd

%description
Cuttlefish indexes a filesystem into PostgreSQL, computes recursive directory
and ownership statistics, and serves the data through a REST API with a React
web UI.

Binaries included:
  fs_indexer    — walks the filesystem and writes metadata to PostgreSQL
  fs_aggregator — computes rolled-up directory and user/group statistics
  fs_api        — REST API server and web UI (managed by systemd)

After installing, copy the config template and edit it:

  cp /etc/cuttlefish/fs_config.toml.example /etc/cuttlefish/fs_config.toml
  $EDITOR /etc/cuttlefish/fs_config.toml

Generate a strong session secret:

  openssl rand -hex 32

Run the indexer and aggregator (from the config directory):

  cd /etc/cuttlefish
  sudo fs_indexer /path/to/scan [threads]
  fs_aggregator

Start the API server:

  systemctl enable --now cuttlefish-api

%pre
getent group  cuttlefish > /dev/null || groupadd  -r cuttlefish
getent passwd cuttlefish > /dev/null || \
  useradd -r -g cuttlefish -d /etc/cuttlefish -s /sbin/nologin \
          -c "Cuttlefish API server" cuttlefish
exit 0

%install
# Indexer and aggregator
install -Dm755 %{_builddir}/fs_indexer    %{buildroot}%{_bindir}/fs_indexer
install -Dm755 %{_builddir}/fs_aggregator %{buildroot}%{_bindir}/fs_aggregator

# API binary — lives under /usr/lib/cuttlefish so the exe-relative UI path resolves
install -Dm755 %{_builddir}/fs_api %{buildroot}/usr/lib/cuttlefish/fs_api

# UI static files served by fs_api at runtime
install -dm755 %{buildroot}/usr/lib/cuttlefish/ui
cp -a %{_builddir}/ui-build %{buildroot}/usr/lib/cuttlefish/ui/build

# Config directory and example config
install -dm755 %{buildroot}/etc/cuttlefish
install -m644 %{_builddir}/fs_config_template.toml \
              %{buildroot}/etc/cuttlefish/fs_config.toml.example

# Systemd service unit
install -Dm644 %{_builddir}/cuttlefish-api.service \
               %{buildroot}/usr/lib/systemd/system/cuttlefish-api.service

%post
%systemd_post cuttlefish-api.service

%preun
%systemd_preun cuttlefish-api.service

%postun
%systemd_postun_with_restart cuttlefish-api.service

%files
%{_bindir}/fs_indexer
%{_bindir}/fs_aggregator
%dir /usr/lib/cuttlefish
/usr/lib/cuttlefish/fs_api
/usr/lib/cuttlefish/ui
%dir /etc/cuttlefish
%config(noreplace) /etc/cuttlefish/fs_config.toml.example
/usr/lib/systemd/system/cuttlefish-api.service

%changelog
* Thu May 29 2026 Cuttlefish Build <build@cuttlefish> - %{pkg_version}-1
- Automated RPM build
