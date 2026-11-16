#!/bin/sh
# Harmless launcher replacement: no network, credentials, git, or agent dispatch.
set -eu
[ "$#" = 3 ] && [ "$1" = W518 ] && [ "$3" = /fixture/brief ]
printf 'stub:01\nstub:02\nstub:03\n'
if [ "$2" = verify/hold ]; then
  exec sleep 120
fi
[ "$2" = verify/finish ]
sleep 1
printf 'stub:04\n'
sleep 1
printf 'stub:05\n' >&2
