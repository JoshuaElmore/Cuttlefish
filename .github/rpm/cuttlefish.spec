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

  cp /etc/cuttlefish/fs_config.yml.example /etc/cuttlefish/fs_config.yml
  $EDITOR /etc/cuttlefish/fs_config.yml

Generate a strong session secret:

  openssl rand -hex 32

Enable the nightly index+aggregate schedule:

  systemctl enable --now cuttlefish-index.timer

Start the API server:

  systemctl enable --now cuttlefish-api

The indexer timer fires at 2am, runs fs_indexer, then automatically triggers
fs_aggregator on success. To re-aggregate without re-indexing:

  systemctl start cuttlefish-aggregate.service

Check logs:

  journalctl -u cuttlefish-index.service
  journalctl -u cuttlefish-aggregate.service

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
install -m644 %{_builddir}/fs_config_template.yml \
              %{buildroot}/etc/cuttlefish/fs_config.yml.example

# Systemd units
install -Dm644 %{_builddir}/cuttlefish-api.service \
               %{buildroot}/usr/lib/systemd/system/cuttlefish-api.service
install -Dm644 %{_builddir}/cuttlefish-index.service \
               %{buildroot}/usr/lib/systemd/system/cuttlefish-index.service
install -Dm644 %{_builddir}/cuttlefish-index.timer \
               %{buildroot}/usr/lib/systemd/system/cuttlefish-index.timer
install -Dm644 %{_builddir}/cuttlefish-aggregate.service \
               %{buildroot}/usr/lib/systemd/system/cuttlefish-aggregate.service

%post
%systemd_post cuttlefish-api.service
%systemd_post cuttlefish-index.service
%systemd_post cuttlefish-index.timer
%systemd_post cuttlefish-aggregate.service

%preun
%systemd_preun cuttlefish-api.service
%systemd_preun cuttlefish-index.timer
%systemd_preun cuttlefish-aggregate.service

%postun
%systemd_postun_with_restart cuttlefish-api.service
%systemd_postun cuttlefish-index.timer
%systemd_postun cuttlefish-aggregate.service

%files
%{_bindir}/fs_indexer
%{_bindir}/fs_aggregator
%dir /usr/lib/cuttlefish
/usr/lib/cuttlefish/fs_api
/usr/lib/cuttlefish/ui
%dir /etc/cuttlefish
%config(noreplace) /etc/cuttlefish/fs_config.yml.example
/usr/lib/systemd/system/cuttlefish-api.service
/usr/lib/systemd/system/cuttlefish-index.service
/usr/lib/systemd/system/cuttlefish-index.timer
/usr/lib/systemd/system/cuttlefish-aggregate.service

%changelog
* Thu May 29 2026 Cuttlefish Build <build@cuttlefish> - %{pkg_version}-1
- Automated RPM build
