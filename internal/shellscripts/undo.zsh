# undo zsh integration
# Sourced automatically by `undo install`, or manually: source ~/.config/undo/undo.zsh

autoload -Uz add-zsh-hook

_undo_preexec() {
    undo capture "$1" 2>&1
}

add-zsh-hook preexec _undo_preexec

if [[ -z "$UNDO_QUIET" ]]; then
    printf '\n  undo ready  ·  rm, mv, overwrites are captured\n'
    printf '  Type  undo  to restore.  Safety net, not a backup.\n\n'
fi
