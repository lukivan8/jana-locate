#!/bin/sh
# Build-time preparation. Requires compiler, autoconf, automake, libtool, curl, git.
# Use a prefix without spaces; libpostal's upstream install scripts require it.
set -eu
repo=$(CDPATH= cd -- "$(dirname "$0")/.." && pwd)
postal_prefix="${LIBPOSTAL_PREFIX:-/tmp/jana-libpostal-runtime}"
case "$postal_prefix" in *' '*) echo 'LIBPOSTAL_PREFIX must not contain spaces' >&2; exit 1;; esac
source_dir=$(mktemp -d /tmp/jana-postal-build.XXXXXX)
trap 'rm -rf "$source_dir"' EXIT INT TERM
git clone https://github.com/openvenues/libpostal "$source_dir"
cd "$source_dir"
git checkout 25099c506612b34b23b1bfe286ca6321fcf06f35
./bootstrap.sh
./configure --prefix="$postal_prefix" --datadir="$postal_prefix/share" --disable-sse2 --disable-data-download
make -j2
make install
./src/libpostal_data download base "$postal_prefix/share/libpostal"
./src/libpostal_data download parser "$postal_prefix/share/libpostal"
printf 'Prepared native libpostal at %s\n' "$postal_prefix"
