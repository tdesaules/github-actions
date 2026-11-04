"""Check stable libcommon's restricted path resolution without mounting anything."""
from pathlib import Path
import subprocess
import tempfile

source = Path('/backport-source/source')
subprocess.run(['dpkg-source', '--before-build', '.'], cwd=source, check=True)
program = r'''
#include "fileutils.h"
#include <assert.h>
#include <errno.h>
#include <stdlib.h>

_Static_assert(RESOLVE_NO_SYMLINKS == 0x04, "Linux UAPI flag mismatch");

int main(void)
{
    char directory[] = "/tmp/util-paths-XXXXXX";
    assert(mkdtemp(directory) != NULL);
    int root = open(directory, O_RDONLY | O_DIRECTORY | O_CLOEXEC);
    assert(root >= 0);
    int fd = openat(root, "file", O_CREAT | O_EXCL | O_RDWR | O_CLOEXEC, 0600);
    assert(fd >= 0);
    close(fd);
    assert(symlinkat("file", root, "link") == 0);
    assert(symlinkat(".", root, "parent") == 0);
    fd = ul_openat_resolve(root, "file", O_RDONLY | O_CLOEXEC, 0,
                           RESOLVE_NO_SYMLINKS);
    assert(fd >= 0);
    close(fd);
    const char *paths[] = {"link", "parent/file"};
    for (size_t i = 0; i < 2; ++i) {
        errno = 0;
        fd = ul_openat_resolve(root, paths[i], O_RDONLY | O_CLOEXEC, 0,
                               RESOLVE_NO_SYMLINKS);
        assert(fd == -1);
        assert(errno == ELOOP);
    }
    assert(unlinkat(root, "link", 0) == 0);
    assert(unlinkat(root, "parent", 0) == 0);
    assert(unlinkat(root, "file", 0) == 0);
    close(root);
    assert(rmdir(directory) == 0);
    puts("PASS regular file and rejection of final/intermediate symlinks");
}
'''
with tempfile.TemporaryDirectory(prefix='util-path-test-') as directory:
    root = Path(directory)
    (root / 'paths.c').write_text(program)
    subprocess.run(['gcc', '-D_GNU_SOURCE', '-include', 'config.h', '-I.',
                    '-Iinclude', str(root / 'paths.c'), '.libs/libcommon.a',
                    '-o', str(root / 'paths')], cwd=source, check=True)
    subprocess.run([str(root / 'paths')], check=True)
