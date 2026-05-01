# undo fish integration

function _undo_preexec --on-event fish_preexec
    undo capture $argv 2>&1
end

# Export binary path so the intercept library can find undo without PATH search
set -x UNDO_BIN (command -v undo 2>/dev/null)

# Deep intercept: covers subshells, make, subprocess calls, C programs.
# Activated automatically when `undo install` has compiled the library.
set _undo_lib_so "$HOME/.config/undo/lib/libundo_intercept.so"
set _undo_lib_dylib "$HOME/.config/undo/lib/libundo_intercept.dylib"
if test -f $_undo_lib_so
    if set -q LD_PRELOAD
        set -x LD_PRELOAD "$_undo_lib_so:$LD_PRELOAD"
    else
        set -x LD_PRELOAD $_undo_lib_so
    end
else if test -f $_undo_lib_dylib
    if set -q DYLD_INSERT_LIBRARIES
        set -x DYLD_INSERT_LIBRARIES "$_undo_lib_dylib:$DYLD_INSERT_LIBRARIES"
    else
        set -x DYLD_INSERT_LIBRARIES $_undo_lib_dylib
    end
end
set -e _undo_lib_so
set -e _undo_lib_dylib

if not set -q UNDO_QUIET
    printf '\n  undo ready  ·  rm, mv, overwrites are captured\n'
    printf '  Type  undo  to restore.  Safety net, not a backup.\n\n'
end
