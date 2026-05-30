Name:           cuttlefish-api
Version:        %{pkg_version}
Release:        1%{?dist}
Summary:        Cuttlefish REST API server and web UI
License:        Proprietary
BuildArch:      x86_64

Requires:       systemd

%description
Go REST API server that queries the Cuttlefish PostgreSQL database and
serves the React web UI. Supports local username/password auth and OIDC/SSO.

After installing, copy the config template and edit it:

  cp /etc/cuttlefish/fs_config.toml.example /etc/cuttlefish/fs_config.toml
  $EDITOR /etc/cuttlefish/fs_config.toml

Generate a strong session secret:

  openssl rand -hex 32

Then enable and start the service:

  systemctl enable --now cuttlefish-api

The server listens on :8080 by default.

%pre
getent group  cuttlefish > /dev/null || groupadd  -r cuttlefish
getent passwd cuttlefish > /dev/null || \
  useradd -r -g cuttlefish -d /etc/cuttlefish -s /sbin/nologin \
          -c "Cuttlefish API server" cuttlefish
exit 0

%install
# API binary — installed to /usr/lib/cuttlefish so the executable-relative
# UI path lookup (filepath.Dir(os.Executable()) + "/ui/build") resolves correctly.
install -Dm755 %{_builddir}/fs_api          %{buildroot}/usr/lib/cuttlefish/fs_api

# UI static files served by the API at runtime
cp -r %{_builddir}/ui-build                 %{buildroot}/usr/lib/cuttlefish/ui/build

# Config directory and template
install -dm755                              %{buildroot}/etc/cuttlefish
install -m644 %{_builddir}/fs_config_template.toml \
                                            %{buildroot}/etc/cuttlefish/fs_config.toml.example

# Systemd unit
install -Dm644 %{_builddir}/cuttlefish-api.service \
                                            %{buildroot}/usr/lib/systemd/system/cuttlefish-api.service

%post
%systemd_post cuttlefish-api.service

%preun
%systemd_preun cuttlefish-api.service

%postun
%systemd_postun_with_restart cuttlefish-api.service

%files
/usr/lib/cuttlefish/fs_api
/usr/lib/cuttlefish/ui/
%dir /etc/cuttlefish
%config(noreplace) /etc/cuttlefish/fs_config.toml.example
/usr/lib/systemd/system/cuttlefish-api.service

%changelog
* $(date "+%a %b %d %Y") Cuttlefish Build <build@cuttlefish> - %{pkg_version}-1
- Automated RPM build
