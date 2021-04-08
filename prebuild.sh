#!/bin/sh

set -e

archs="$@"
if [ -z "$archs" ]; then
	archs="aarch64 x86_64"
fi

if [ -z "$GZIPPER" ]; then
	GZIPPER=zopfli
fi

muslccversion=10.2.1
ipfsgateway=https://ipfs.io/ipfs/

binarydir=internal/container/child/binary
muslccdir=tmp/muslcc-$muslccversion

. runtime/include/runtime.mk

per_binary() {
	arch=$1
	pre=$2
	name=$3
	version=$4

	case $arch in
		aarch64) goarch=arm64;;
		x86_64)  goarch=amd64;;
		*)       goarch=$arch;;
	esac

	make="make -C runtime/$name CC=${pre}cc"
	$make clean
	$make

	(
		set -x

		${pre}objcopy -R .comment -R .eh_frame lib/gate/gate-runtime-$name.$version tmp/$name.$arch
		${pre}strip tmp/$name.$arch
		$GZIPPER tmp/$name.$arch
		mv -f tmp/$name.$arch.gz $binarydir/$name.linux-$goarch.gz
	)

	$make clean
}

per_arch() {
	arch=$1
	comp=$arch-linux-musl-cross

	case $arch in
		aarch64) url=${ipfsgateway}QmeECAMETVvmAfrnEuoTsCnRPs56VzDY9y4mZQJaGxwFhf;;
		x86_64)  url=${ipfsgateway}QmRrbNwGqmkQfjJict6UHngBxfSKJV2bCx6ewEVB9t5yiB;;
		*)       url=https://more.musl.cc/$muslccversion/x86_64-linux-musl/$comp.tgz;;
	esac

	if [ ! -e $muslccdir/$comp ]; then
		if [ ! -e $muslccdir/$comp.tgz ]; then
			mkdir -p $muslccdir
			curl -o $muslccdir/$comp.tgz $url
		fi

		sum=$(readlink -f muslcc-$arch.sha512sum)
		(cd $muslccdir && sha512sum $sum)
		(cd $muslccdir && tar xfz $comp.tgz)
	fi

	pre=$(readlink -f $muslccdir)/$comp/bin/$arch-linux-musl-
	per_binary $arch $pre executor $GATE_COMPAT_MAJOR
	per_binary $arch $pre loader $GATE_COMPAT_VERSION
}

for arch in $archs; do
	per_arch $arch
done
