Name:           cuttlefish-indexer
Version:        %{pkg_version}
Release:        1%{?dist}
Summary:        Cuttlefish filesystem indexer
License:        Proprietary
BuildArch:      x86_64

Requires:       glibc

%description
Walks a filesystem tree in parallel and upserts per-entry metadata into
the Cuttlefish PostgreSQL database (filesystem_index, identity_map,
scan_sessions tables).

The binary reads fs_config.toml from the current working directory.
Run it from /etc/cuttlefish where your config lives:

  cd /etc/cuttlefish
  sudo fs_indexer /path/to/scan [threads]

Default thread count is 8. Root access is typically required to scan
system paths such as /.

%install
install -Dm755 %{_builddir}/fs_indexer %{buildroot}%{_bindir}/fs_indexer

%files
%{_bindir}/fs_indexer

%changelog
* $(date "+%a %b %d %Y") Cuttlefish Build <build@cuttlefish> - %{pkg_version}-1
- Automated RPM build
