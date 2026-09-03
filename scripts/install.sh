#!/bin/sh
# Install Coeus on Ubuntu or Debian, from the web with
#   curl -fsSL https://raw.githubusercontent.com/JaredTate/coeus/main/scripts/install.sh | sh
# or from a checkout with "sh scripts/install.sh". Anything after a bare "--" is
# handed to "coeus init", so a machine with no keyboard can answer every question
# on the command line. Every step prints one line saying what it did. A step that
# fails and can be lived without says what to do and the run carries on; only a
# release that cannot be verified or unpacked stops it, because that is the one
# step that would leave nothing to run.
#
# Borrowed designs: fetching the checksum file before the archive it checks, and
# keeping nothing that does not match, is ZeroClaw's, at ~/Code/zeroclaw/install.sh
# lines 283-340. OpenClaw's ~/Code/openclaw/scripts/install.sh is the example
# avoided: four thousand lines, its own progress-bar program downloaded at run
# time, and sudo handed to scripts fetched from third parties.
set -eu

# The pinned signal-cli, used only when the package manager has never heard of it,
# which is true of Ubuntu and Debian today. It is the native build, so it needs no
# Java, and it is published for x86_64 only.
signal_version=0.14.7
signal_sum=0fe065294adcf35df4c249b635d0ce57de7765d4fec660bffaa2e7f0549d4e5f

# Where things go. The first two are read from the environment so that the tests
# in scripts/release can run every privileged step against a fixture rather than
# against this machine; on a real install they are the real paths.
os_release_file=${COEUS_OS_RELEASE:-/etc/os-release}
apparmor_folder=${COEUS_APPARMOR_DIR:-/etc/apparmor.d}
coeus_home=${COEUS_HOME:-$HOME/.coeus}
work_folder=$HOME/coeus
launcher_folder=$HOME/.local/bin
repository=JaredTate/coeus
from=""
wanted_version=latest
install_signal=yes

say() { printf '==> %s\n' "$*"; }
warn() { printf '!!! %s\n' "$*" >&2; }
die() { printf '!!! %s\n' "$*" >&2; exit 1; }
fetch() { curl --fail --silent --show-error --location --retry 3 --max-time "$1" --output "$2" "$3"; }
read_os_release() { sed -n "s/^$1=//p" "$os_release_file" 2>/dev/null | head -1 | tr -d '"'; }
checksum_of() { sha256sum "$1" | cut -d ' ' -f 1; }
apt_can() { [ "$distribution" = ubuntu ] || [ "$distribution" = debian ]; }

usage() {
	cat <<'ENDOFUSAGE'
usage: install.sh [options] [-- <flags for coeus init>]
  --from <path>    install this archive from "make release" instead of downloading one
  --version <tag>  download this release rather than the newest one
  --no-signal      leave signal-cli out; Coeus needs it only to talk over Signal
  --help           print this and stop
With every question answered on the command line:
  sh install.sh -- --model local --work-folder ~/coeus --signal off --yes
ENDOFUSAGE
}

while [ $# -gt 0 ]; do
	case "$1" in
	--from | --version) [ $# -ge 2 ] || die "the flag $1 needs a value after it; run install.sh --help" ;;
	esac
	case "$1" in
	--from) from=$2; shift 2 ;;
	--version) wanted_version=$2; shift 2 ;;
	--no-signal) install_signal=no; shift ;;
	-h | --help) usage; exit 0 ;;
	--) shift; break ;;
	*) die "install.sh does not know the flag \"$1\"; run install.sh --help to see the ones it does" ;;
	esac
done

# Run one command as the administrator, or straight out when we already are.
as_root() {
	if [ "$(id -u)" = 0 ]; then "$@"
	elif command -v sudo >/dev/null 2>&1; then sudo "$@"
	else
		warn "this step needs administrator rights and there is no sudo here, so it was skipped: $*"
		return 1
	fi
}

distribution=$(read_os_release ID)
distribution_version=$(read_os_release VERSION_ID)
pretty_name=$(read_os_release PRETTY_NAME)
[ -n "$distribution" ] || distribution=unknown
say "found ${pretty_name:-a machine that does not say what it is} ($distribution $distribution_version)"
architecture=$(uname -m)
case "$architecture" in
x86_64) architecture=amd64 ;;
aarch64 | arm64) architecture=arm64 ;;
*) die "Coeus is released for x86_64 and aarch64, and this machine is $architecture, so build it yourself with \"make release\"" ;;
esac

# Step one: get the archive and be sure of it before anything on this machine is
# changed, because a release that cannot be verified means nothing else was worth
# doing.
downloads=$(mktemp -d)
trap 'rm -rf "$downloads"' EXIT INT TERM
if [ -n "$from" ]; then
	[ -f "$from" ] || die "the archive $from is not there; name one that \"make release\" wrote into dist/"
	archive=$from
	sums=$(dirname "$from")/SHA256SUMS
	say "installing from the archive $archive"
else
	base=https://github.com/$repository/releases/latest/download
	[ "$wanted_version" = latest ] || base=https://github.com/$repository/releases/download/$wanted_version
	sums=$downloads/SHA256SUMS
	fetch 120 "$sums" "$base/SHA256SUMS" || die "the checksum file at $base/SHA256SUMS could not be downloaded, so nothing was installed; check the network, or pass --version naming a release that exists"
	name=$(awk -v want="-$architecture.tar.gz" 'index($2, want) { print $2; exit }' "$sums")
	[ -n "$name" ] || die "that release has no archive for $architecture; what it does have is listed in $base/SHA256SUMS"
	archive=$downloads/$name
	say "downloading $name"
	fetch 1800 "$archive" "$base/$name" || die "$name could not be downloaded from $base, so nothing was installed"
fi
if [ ! -f "$sums" ]; then
	warn "there is no SHA256SUMS beside $archive, so its checksum could not be checked"
else
	expected=$(awk -v want="$(basename "$archive")" '$2 == want { print $1; exit }' "$sums")
	[ -n "$expected" ] || die "$sums has no line for $(basename "$archive"), so what to install could not be checked; download the release again"
	[ "$(checksum_of "$archive")" = "$expected" ] || die "the checksum of $archive is $(checksum_of "$archive") and $sums says it should be $expected, so nothing was installed; download the release again"
	say "the checksum of $(basename "$archive") matches SHA256SUMS"
fi

# Step two: the programs Coeus runs. None of these is fatal: a machine missing one
# runs with that one tool switched off, and "coeus doctor" says which.
if apt_can; then
	say "installing bubblewrap and ripgrep with apt-get"
	as_root apt-get update -qq || warn "apt-get update did not finish, so the next step may not find the packages"
	as_root apt-get install -y -qq bubblewrap ripgrep || warn "bubblewrap and ripgrep could not be installed; install them by hand, then run \"coeus doctor\""
else
	warn "this installer knows how to install packages on Ubuntu and Debian only, and this is $distribution, so install bubblewrap, ripgrep, and signal-cli yourself with its package manager"
fi
if [ "$install_signal" != yes ]; then
	say "signal-cli was left out because of --no-signal; install it later if you want to talk to Coeus over Signal"
elif apt_can && as_root apt-get install -y -qq signal-cli 2>/dev/null; then
	say "signal-cli came from apt-get"
elif [ "$architecture" != amd64 ]; then
	warn "there is no pinned signal-cli build for $architecture; install it yourself from https://github.com/AsamK/signal-cli, then run \"coeus doctor\""
else
	say "the package manager has no signal-cli, so downloading the pinned $signal_version build"
	signal_archive=$downloads/signal-cli.tar.gz
	if ! fetch 900 "$signal_archive" "https://github.com/AsamK/signal-cli/releases/download/v$signal_version/signal-cli-$signal_version-Linux-native.tar.gz"; then
		warn "signal-cli could not be downloaded from github.com; Signal stays switched off until you install it and run \"coeus doctor\""
	elif [ "$(checksum_of "$signal_archive")" != "$signal_sum" ]; then
		warn "the checksum of the signal-cli download is not the pinned $signal_sum, so it was thrown away; Signal stays switched off, and \"coeus doctor\" will say so"
	else
		mkdir -p "$coeus_home/tools" "$launcher_folder"
		tar -xzf "$signal_archive" -C "$coeus_home/tools"
		ln -sfn "$coeus_home/tools/signal-cli" "$launcher_folder/signal-cli"
		say "signal-cli $signal_version is in $coeus_home/tools"
	fi
fi

# Step three: Ubuntu 24.04 and later refuse an unconfined program a new user
# namespace, so an installed bwrap is not the same as a working one.
apparmor_major=${distribution_version%%.*}
case "$apparmor_major" in '' | *[!0-9]*) apparmor_major=0 ;; esac
if [ "$distribution" = ubuntu ] && [ "$apparmor_major" -ge 24 ]; then
	say "Ubuntu $distribution_version blocks unprivileged user namespaces, so writing the AppArmor profile for bwrap"
	profile='abi <abi/4.0>,\ninclude <tunables/global>,\nprofile bwrap /usr/bin/bwrap flags=(unconfined) {\n  userns,\n  include if exists <local/bwrap>\n}\n'
	if as_root mkdir -p "$apparmor_folder" && printf '%b' "$profile" | as_root tee "$apparmor_folder/bwrap" >/dev/null; then
		as_root apparmor_parser -r "$apparmor_folder/bwrap" || warn "apparmor_parser could not load $apparmor_folder/bwrap; load it yourself, or set kernel.apparmor_restrict_unprivileged_userns to 0"
	else
		warn "$apparmor_folder/bwrap could not be written, so the sandbox may stay blocked"
	fi
	if bwrap --unshare-user --ro-bind / / /bin/true >/dev/null 2>&1; then
		say "bwrap can make a user namespace, so the sandbox works"
	else
		warn "bwrap still cannot make a user namespace; run \"coeus doctor\", which says what is in the way"
	fi
fi

say "Google Chrome is yours to install: get it from https://www.google.com/chrome, or run \"sudo apt-get install chromium\". The browser tools need one of the two; everything else works without it."
mkdir -p "$work_folder"
say "the work folder $work_folder is ready, and Coeus may read and write there and nowhere else"

# Step four: put the release where the service unit looks for it. "current" is a
# link to one version's binary, and the updater of brief 6.3 moves that link.
staging=$coeus_home/releases/.unpacking
rm -rf "$staging"
mkdir -p "$staging"
tar -xzf "$archive" -C "$staging"
version=$(cat "$staging/VERSION" 2>/dev/null || true)
[ -n "$version" ] || die "$archive holds no VERSION file, so it is not an archive \"make release\" wrote, and nothing was installed"
release=$coeus_home/releases/$version
rm -rf "$release"
mv "$staging" "$release"
ln -sfn "$release/coeus" "$coeus_home/releases/current"
mkdir -p "$launcher_folder"
ln -sfn "$coeus_home/releases/current" "$launcher_folder/coeus"
say "coeus $version is unpacked in $release, and $launcher_folder/coeus runs it"
case ":$PATH:" in
*":$launcher_folder:"*) ;;
*) warn "$launcher_folder is not on your PATH; add it in your shell profile, and until you do, type the whole path" ;;
esac

say "running coeus init"
"$launcher_folder/coeus" init "$@"
