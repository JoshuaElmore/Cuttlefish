Name:           cuttlefish-aggregator
Version:        %{pkg_version}
Release:        1%{?dist}
Summary:        Cuttlefish filesystem statistics aggregator
License:        Proprietary
BuildArch:      x86_64

Requires:       glibc

%description
Reads the filesystem_index table populated by cuttlefish-indexer and
computes rolled-up statistics in a single bottom-up pass, writing results
to the dir_stats and user_stats tables.

Run after each indexer pass:

  cd /etc/cuttlefish
  fs_aggregator

The binary reads fs_config.toml from the current working directory.

%install
install -Dm755 %{_builddir}/fs_aggregator %{buildroot}%{_bindir}/fs_aggregator

%files
%{_bindir}/fs_aggregator

%changelog
* $(date "+%a %b %d %Y") Cuttlefish Build <build@cuttlefish> - %{pkg_version}-1
- Automated RPM build
