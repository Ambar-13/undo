# undo bash integration
# Requires bash-preexec: https://github.com/rcaloras/bash-preexec
# Source this AFTER sourcing bash-preexec.sh

_undo_preexec() {
    undo capture "$1" 2>&1
}

preexec_functions+=(_undo_preexec)

if [[ -z "$UNDO_QUIET" ]]; then
    printf '\n  undo ready  ·  rm, mv, overwrites are captured\n'
    printf '  Type  undo  to restore.  Safety net, not a backup.\n\n'
fi
