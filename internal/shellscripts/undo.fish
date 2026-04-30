# undo fish integration

function _undo_preexec --on-event fish_preexec
    undo capture $argv 2>&1
end

if not set -q UNDO_QUIET
    printf '\n  undo ready  ·  rm, mv, overwrites are captured\n'
    printf '  Type  undo  to restore.  Safety net, not a backup.\n\n'
end
