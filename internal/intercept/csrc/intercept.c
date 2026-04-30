/*
 * undo_intercept.c — libc-level interception for filesystem undo.
 *
 * Loaded via LD_PRELOAD (Linux) or DYLD_INSERT_LIBRARIES (macOS).
 * Intercepts unlink, rename, truncate, and open(O_TRUNC) to call
 * `undo capture-file <path>` before any destructive operation.
 *
 * Covers: subshells, background jobs, make, C programs, Python C extensions.
 * Does NOT cover SIP-protected binaries on macOS (/bin/rm, etc.) — those are
 * handled by the shell preexec hook.
 */

#define _GNU_SOURCE
#include <dlfcn.h>
#include <errno.h>
#include <fcntl.h>
#include <spawn.h>
#include <stdarg.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <sys/stat.h>
#include <sys/types.h>
#include <sys/wait.h>
#include <unistd.h>

#ifdef __APPLE__
# include <mach-o/dyld.h>
#endif

extern char **environ;

/* ── binary location (resolved once at library load) ──────── */

static char s_undo_bin[4096];

__attribute__((constructor))
static void find_undo_bin(void) {
    /* 1. Explicit env var (set by shell script) */
    const char *env_bin = getenv("UNDO_BIN");
    if (env_bin && access(env_bin, X_OK) == 0) {
        strncpy(s_undo_bin, env_bin, sizeof(s_undo_bin) - 1);
        return;
    }

    /* 2. Search PATH */
    const char *pathenv = getenv("PATH");
    if (!pathenv) pathenv = "/usr/local/bin:/usr/bin:/bin:/opt/homebrew/bin";

    char pbuf[8192];
    strncpy(pbuf, pathenv, sizeof(pbuf) - 1);

    char *save = NULL;
    char *tmp  = pbuf;
    char *dir;
    while ((dir = strtok_r(tmp, ":", &save)) != NULL) {
        tmp = NULL;
        char candidate[4096];
        snprintf(candidate, sizeof(candidate), "%s/undo", dir);
        if (access(candidate, X_OK) == 0) {
            strncpy(s_undo_bin, candidate, sizeof(s_undo_bin) - 1);
            return;
        }
    }
    /* Not found — s_undo_bin stays empty, all captures are no-ops */
}

static const char *undo_bin(void) {
    return s_undo_bin[0] ? s_undo_bin : NULL;
}

/* ── skip guard ────────────────────────────────────────────── */

static int should_skip(const char *path) {
    if (!path || !path[0]) return 1;

    /* Recursion guard: undo capture-file sets this */
    if (getenv("UNDO_NO_INTERCEPT")) return 1;

    /* System / virtual filesystems — never user files */
    if (strncmp(path, "/proc/",   6) == 0) return 1;
    if (strncmp(path, "/sys/",    5) == 0) return 1;
    if (strncmp(path, "/dev/",    5) == 0) return 1;
    if (strncmp(path, "/run/",    5) == 0) return 1;

    /* Undo's own object store — prevents infinite recursion */
    const char *home = getenv("HOME");
    if (home) {
        char store[4096];
        int n = snprintf(store, sizeof(store), "%s/.undo/", home);
        if (n > 0 && (size_t)n < sizeof(store) &&
            strncmp(path, store, (size_t)n) == 0) return 1;
        /* Also skip undo's config dir */
        n = snprintf(store, sizeof(store), "%s/.config/undo/", home);
        if (n > 0 && (size_t)n < sizeof(store) &&
            strncmp(path, store, (size_t)n) == 0) return 1;
    }
    return 0;
}

/* ── capture one path ──────────────────────────────────────── */

static void capture(const char *path) {
    if (should_skip(path)) return;

    /* File must exist — nothing to capture for new files */
    struct stat st;
    if (stat(path, &st) != 0) return;

    const char *bin = undo_bin();
    if (!bin) return;

    /* Build env with recursion guard (drop existing UNDO_NO_INTERCEPT) */
    int n = 0;
    for (char **e = environ; *e; e++) n++;
    char **env2 = malloc((size_t)(n + 2) * sizeof(char *));
    if (!env2) return;

    int j = 0;
    for (int i = 0; i < n; i++) {
        if (strncmp(environ[i], "UNDO_NO_INTERCEPT=", 18) != 0)
            env2[j++] = environ[i];
    }
    env2[j++] = "UNDO_NO_INTERCEPT=1";
    env2[j]   = NULL;

    pid_t pid;
    char *const argv[] = { (char *)bin, "capture-file", (char *)path, NULL };
    int err = posix_spawn(&pid, bin, NULL, NULL, argv, env2);
    free(env2);
    if (err == 0) {
        int wstatus;
        waitpid(pid, &wstatus, 0);
    }
}

/* ── resolve path relative to a dirfd ─────────────────────── */

static void capture_at(int dirfd, const char *path) {
    if (!path || !path[0]) return;

    if (path[0] == '/') {
        capture(path);
        return;
    }

    /* AT_FDCWD: relative to current working directory */
    if (dirfd == AT_FDCWD) {
        char cwd[4096];
        if (getcwd(cwd, sizeof(cwd))) {
            char abs[4096];
            snprintf(abs, sizeof(abs), "%s/%s", cwd, path);
            capture(abs);
        }
        return;
    }

#ifdef __linux__
    /* Resolve via /proc/self/fd/<n> */
    char fdpath[64], dirpath[4096];
    snprintf(fdpath, sizeof(fdpath), "/proc/self/fd/%d", dirfd);
    ssize_t len = readlink(fdpath, dirpath, sizeof(dirpath) - 1);
    if (len > 0) {
        dirpath[len] = '\0';
        char abs[4096];
        snprintf(abs, sizeof(abs), "%s/%s", dirpath, path);
        capture(abs);
    }
#endif
    /* macOS without /proc: fd-relative paths with non-CWD fd are skipped.
     * This is rare; AT_FDCWD covers the common case. */
}

/* ================================================================
   Intercepted functions
   ================================================================ */

/* ── unlink ─────────────────────────────────────────────────── */
typedef int (*fn_unlink)(const char *);
int unlink(const char *path) {
    static fn_unlink real;
    if (!real) real = (fn_unlink)dlsym(RTLD_NEXT, "unlink");
    capture(path);
    return real(path);
}

/* ── unlinkat ───────────────────────────────────────────────── */
typedef int (*fn_unlinkat)(int, const char *, int);
int unlinkat(int dirfd, const char *path, int flags) {
    static fn_unlinkat real;
    if (!real) real = (fn_unlinkat)dlsym(RTLD_NEXT, "unlinkat");
    /* Only capture files, not directories (AT_REMOVEDIR) */
    if (!(flags & AT_REMOVEDIR))
        capture_at(dirfd, path);
    return real(dirfd, path, flags);
}

/* ── rename ─────────────────────────────────────────────────── */
typedef int (*fn_rename)(const char *, const char *);
int rename(const char *oldpath, const char *newpath) {
    static fn_rename real;
    if (!real) real = (fn_rename)dlsym(RTLD_NEXT, "rename");
    capture(oldpath);
    capture(newpath);    /* destination may be overwritten */
    return real(oldpath, newpath);
}

/* ── renameat ───────────────────────────────────────────────── */
typedef int (*fn_renameat)(int, const char *, int, const char *);
int renameat(int olddirfd, const char *oldpath, int newdirfd, const char *newpath) {
    static fn_renameat real;
    if (!real) real = (fn_renameat)dlsym(RTLD_NEXT, "renameat");
    capture_at(olddirfd, oldpath);
    capture_at(newdirfd, newpath);
    return real(olddirfd, oldpath, newdirfd, newpath);
}

/* ── truncate ───────────────────────────────────────────────── */
typedef int (*fn_truncate)(const char *, off_t);
int truncate(const char *path, off_t length) {
    static fn_truncate real;
    if (!real) real = (fn_truncate)dlsym(RTLD_NEXT, "truncate");
    capture(path);
    return real(path, length);
}

/* ── remove (calls unlink or rmdir) ────────────────────────── */
typedef int (*fn_remove)(const char *);
int remove(const char *path) {
    static fn_remove real;
    if (!real) real = (fn_remove)dlsym(RTLD_NEXT, "remove");
    capture(path);    /* capture() skips dirs gracefully via stat */
    return real(path);
}

/* ── open (capture only when O_TRUNC destroys existing content) */
typedef int (*fn_open)(const char *, int, ...);
int open(const char *path, int flags, ...) {
    static fn_open real;
    if (!real) real = (fn_open)dlsym(RTLD_NEXT, "open");

    /* O_TRUNC on an existing file destroys its current content */
    if (flags & O_TRUNC) capture(path);

    /* Forward with optional mode (required when O_CREAT is set) */
    mode_t mode = 0;
    if (flags & O_CREAT) {
        va_list ap;
        va_start(ap, flags);
        mode = (mode_t)va_arg(ap, int);
        va_end(ap);
    }
    return real(path, flags, mode);
}

/* ── openat ─────────────────────────────────────────────────── */
typedef int (*fn_openat)(int, const char *, int, ...);
int openat(int dirfd, const char *path, int flags, ...) {
    static fn_openat real;
    if (!real) real = (fn_openat)dlsym(RTLD_NEXT, "openat");

    if (flags & O_TRUNC) capture_at(dirfd, path);

    mode_t mode = 0;
    if (flags & O_CREAT) {
        va_list ap;
        va_start(ap, flags);
        mode = (mode_t)va_arg(ap, int);
        va_end(ap);
    }
    return real(dirfd, path, flags, mode);
}
