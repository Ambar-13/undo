# undo bash integration
# Requires bash-preexec: https://github.com/rcaloras/bash-preexec
# Source this AFTER sourcing bash-preexec.sh

_undo_preexec() {
    undo capture "$1" 2>&1
}

preexec_functions+=(_undo_preexec)

# Export binary path so the intercept library can find undo without PATH search
export UNDO_BIN="${UNDO_BIN:-$(command -v undo 2>/dev/null)}"

# Deep intercept: covers subshells, make, subprocess calls, C programs.
# Activated automatically when `undo install` has compiled the library.
_undo_lib_so="$HOME/.config/undo/lib/libundo_intercept.so"
_undo_lib_dylib="$HOME/.config/undo/lib/libundo_intercept.dylib"
if [[ -f "$_undo_lib_so" ]]; then
    export LD_PRELOAD="$_undo_lib_so${LD_PRELOAD:+:$LD_PRELOAD}"
elif [[ -f "$_undo_lib_dylib" ]]; then
    export DYLD_INSERT_LIBRARIES="$_undo_lib_dylib${DYLD_INSERT_LIBRARIES:+:$DYLD_INSERT_LIBRARIES}"
fi
unset _undo_lib_so _undo_lib_dylib

if [[ -z "$UNDO_QUIET" ]]; then
    printf '\n  undo ready  ·  rm, mv, overwrites are captured\n'
    printf '  Type  undo  to restore.  Safety net, not a backup.\n\n'
fi
