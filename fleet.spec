%global with_devel 1
%global with_debug 1
%global with_check 1
%global with_unit_test 1

%if 0%{?with_debug}
%global _dwz_low_mem_die_limit 0
%else
%global debug_package   %{nil}
%endif

%global provider	github
%global provider_tld	com
%global project		coreos
%global repo		fleet
# https://github.com/cea-hpc/fleet
%global provider_prefix %{provider}.%{provider_tld}/%{project}/%{repo}
%global import_path     %{provider_prefix}

%global commit		c5a57f744b54c8410f0671377b82de253c7e269b
%global shortcommit	%(c=%{commit}; echo ${c:0:7})
%global baseversion     1.0.0
%global commits_from_base 15

Name:		fleet
# git describe --dirty
Version:	%{baseversion}_%{commits_from_base}_g%{shortcommit}
Release:	1.ocean1%{?dist}
Summary:	A distributed init system

License:	ASL 2.0
Source0:	%{name}-%{shortcommit}.tar.gz
Source1:	%{name}.service
Source2:	%{name}.socket
Source3:	%{name}.tmpfiles.conf
Source4:	%{name}.polkit.rules

# e.g. el6 has ppc64 arch without gcc-go, so EA tag is required
ExclusiveArch:  %{?go_arches:%{go_arches}}%{!?go_arches:%{ix86} x86_64 aarch64 %{arm}}
# If go_compiler is not set to 1, there is no virtual provide. Use golang instead.
BuildRequires:  %{?go_compiler:compiler(go-compiler)}%{!?go_compiler:golang}

BuildRequires:	systemd
BuildRequires:	git

Requires(pre):	shadow-utils
Requires(post): systemd
Requires(preun): systemd
Requires(postun): systemd

%description
%{summary}.

%if 0%{?with_devel}
%package devel
Summary:       %{summary}
BuildArch:     noarch


Provides:      golang(%{import_path}/agent) = %{version}-%{release}
Provides:      golang(%{import_path}/api) = %{version}-%{release}
Provides:      golang(%{import_path}/client) = %{version}-%{release}
Provides:      golang(%{import_path}/config) = %{version}-%{release}
Provides:      golang(%{import_path}/engine) = %{version}-%{release}
Provides:      golang(%{import_path}/etcd) = %{version}-%{release}
Provides:      golang(%{import_path}/functional/platform) = %{version}-%{release}
Provides:      golang(%{import_path}/functional/util) = %{version}-%{release}
Provides:      golang(%{import_path}/heart) = %{version}-%{release}
Provides:      golang(%{import_path}/job) = %{version}-%{release}
Provides:      golang(%{import_path}/log) = %{version}-%{release}
Provides:      golang(%{import_path}/machine) = %{version}-%{release}
Provides:      golang(%{import_path}/pkg) = %{version}-%{release}
Provides:      golang(%{import_path}/registry) = %{version}-%{release}
Provides:      golang(%{import_path}/resource) = %{version}-%{release}
Provides:      golang(%{import_path}/schema) = %{version}-%{release}
Provides:      golang(%{import_path}/scripts) = %{version}-%{release}
Provides:      golang(%{import_path}/server) = %{version}-%{release}
Provides:      golang(%{import_path}/ssh) = %{version}-%{release}
Provides:      golang(%{import_path}/systemd) = %{version}-%{release}
Provides:      golang(%{import_path}/unit) = %{version}-%{release}
Provides:      golang(%{import_path}/version) = %{version}-%{release}

%description devel
%{summary}

This package contains library source intended for
building other packages which use import path with
%{import_path} prefix.
%endif

%if 0%{?with_unit_test} && 0%{?with_devel}
%package unit-test-devel
Summary:         Unit tests for %{name} package

# test subpackage tests code from devel subpackage
Requires:        %{name}-devel = %{version}-%{release}


%description unit-test-devel
%{summary}

This package contains unit tests for project
providing packages with %{import_path} prefix.
%endif

%prep
%setup -q -n %{repo}-%{shortcommit}

%build
export GOPATH=$(pwd):%{gopath}
./build

LAST_TAG=$(git describe --abbrev=0 --tags)
echo "* Changes since ${LAST_TAG}" > Changelog
git log --no-merges --format="-%s - %h (%an)" ${LAST_TAG}..HEAD >> Changelog


%install
install -D -p -m 0755 bin/%{name}ctl %{buildroot}%{_bindir}/%{name}ctl
install -D -p -m 0755 bin/%{name}d %{buildroot}%{_bindir}/%{name}d
install -D -p -m 0644 %{SOURCE1} %{buildroot}%{_unitdir}/%{name}.service
install -D -p -m 0644 %{SOURCE2} %{buildroot}%{_unitdir}/%{name}.socket

# Create home directory
install -d -m 0755 %{buildroot}/run/%{name}

# Create directory for config
install -D -p -m 0644 %{name}.conf.sample %{buildroot}%{_sysconfdir}/%{name}/%{name}.conf

install -D -p -m 0644 %{SOURCE3} %{buildroot}%{_tmpfilesdir}/%{name}.conf

install -D -p -m 0644 %{SOURCE4} %{buildroot}%{_datadir}/polkit-1/rules.d/%{name}.rules

# source codes for building projects
%if 0%{?with_devel}
install -d -p %{buildroot}/%{gopath}/src/%{import_path}/
echo "%%dir %%{gopath}/src/%%{import_path}/." >> devel.file-list
# find all *.go but no *_test.go files and generate devel.file-list
for file in $(find . -iname "*.go" \! -iname "*_test.go" | egrep -v "./Godeps/_workspace/src") ; do
    echo "%%dir %%{gopath}/src/%%{import_path}/$(dirname $file)" >> devel.file-list
    install -d -p %{buildroot}/%{gopath}/src/%{import_path}/$(dirname $file)
    cp -pav $file %{buildroot}/%{gopath}/src/%{import_path}/$file
    echo "%%{gopath}/src/%%{import_path}/$file" >> devel.file-list
done
%endif

# testing files for this project
%if 0%{?with_unit_test} && 0%{?with_devel}
install -d -p %{buildroot}/%{gopath}/src/%{import_path}/
# find all *_test.go files and generate unit-test-devel.file-list
for file in $(find . -iname "*_test.go" | egrep -v "./Godeps/_workspace/src") ; do
    echo "%%dir %%{gopath}/src/%%{import_path}/$(dirname $file)" >> devel.file-list
    install -d -p %{buildroot}/%{gopath}/src/%{import_path}/$(dirname $file)
    cp -pav $file %{buildroot}/%{gopath}/src/%{import_path}/$file
    echo "%%{gopath}/src/%%{import_path}/$file" >> unit-test-devel.file-list
done
%endif

%if 0%{?with_devel}
sort -u -o devel.file-list devel.file-list
%endif

%check
%if 0%{?with_check} && 0%{?with_unit_test} && 0%{?with_devel}
export GOPATH=%{buildroot}/%{gopath}:%{gopath}

./test -v

%endif

%pre
getent group %{name} >/dev/null || groupadd -r %{name}
getent passwd %{name} >/dev/null || useradd -r -g %{name} -d /run/%{name} \
	-s /sbin/nologin -c "%{name} user" %{name}

%post
%systemd_post %{name}.service
%systemd_post %{name}.socket

%preun
%systemd_preun %{name}.service
%systemd_post %{name}.socket

%postun
%systemd_postun %{name}.service
%systemd_post %{name}.socket

#define license tag if not already defined
%{!?_licensedir:%global license %doc}

%files
%license LICENSE
%doc CONTRIBUTING.md README.md Documentation/ examples/ DCO MAINTAINERS NOTICE Changelog
%dir %attr(750,%{name},%{name}) %{_sysconfdir}/%{name}
%config %attr(750,%{name},%{name}) %{_sysconfdir}/%{name}/%{name}.conf
%{_bindir}/%{name}ctl
%{_bindir}/%{name}d
%dir %attr(750,%{name},%{name}) /run/%{name}
%{_unitdir}/%{name}.service
%{_unitdir}/%{name}.socket
%{_tmpfilesdir}/%{name}.conf
%{_datadir}/polkit-1/rules.d/%{name}.rules

%if 0%{?with_devel}
%files devel -f devel.file-list
%license LICENSE
%doc CONTRIBUTING.md README.md Documentation/ examples/ DCO MAINTAINERS NOTICE Changelog
%dir %{gopath}/src/%{provider}.%{provider_tld}/%{project}
%endif

%if 0%{?with_unit_test} && 0%{?with_devel}
%files unit-test-devel -f unit-test-devel.file-list
%license LICENSE
%doc CONTRIBUTING.md README.md Documentation/ examples/ DCO MAINTAINERS NOTICE Changelog
%endif

%changelog
* Thu Jul 6 2017 Romain FIHUE <romain.fihue@cea.fr> - 1.0.0_15_gc5a57f7-1.ocean1
- server: Syncing etcd client when handling failure. This ensures we try different endpoints in any cases

* Wed Jul 5 2017 Romain FIHUE <romain.fihue@cea.fr> - 1.0.0_14_gg33c2cf-1.ocean1
- Fixed gRPC failure handling by removing the huge TTL set when gRPC is used. Leader elections might happen more often.

* Fri Feb 3 2017 Romain FIHUE <romain.fihue@cea.fr> - 1.0.0_13_gdeb2528-1.ocean1
- Rebased on github master (1.0.0-7-gaad7239). Mostly performance/scalability enhancements (gRPC)

* Fri Nov 18 2016 Romain FIHUE <romain.fihue@cea.fr> - 0.13.0_194_g7031d29-1.ocean1
- Updating MachineLegend to inclue machine hostname. Used in fleetctl outputs
- scheduler: Implementing weighted scheduler
- server: Purging agent reconciler when server monitor is triggered

* Thu Oct 27 2016 Romain FIHUE <romain.fihue@cea.fr> - 0.13.0_191_g8ec7b8c-1.ocean1
- Adding BUILD_ID to compiled binaries (romain.fihue)
- Change default ssh port (romain.fihue)
- Adding Hostname field to list-units and list-machine output (romain.fihue)

* Thu Oct 27 2016 Romain FIHUE <romain.fihue@cea.fr> - 0.13.0_191_g8ec7b8c-1
- Initial import
