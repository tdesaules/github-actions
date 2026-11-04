"""Report only kernel-denied prerequisites for two upstream container tests."""
import ctypes
import errno
import os
import sys

libc = ctypes.CDLL(None, use_errno=True)
operation = sys.argv[1]
if operation == 'personality':
    libc.personality.argtypes = [ctypes.c_ulong]
    current = libc.personality(0xffffffff)
    if current == -1:
        raise OSError(ctypes.get_errno(), 'read personality')
    result = libc.personality(current | 0x40000)  # ADDR_NO_RANDOMIZE, child only
elif operation == 'inotify':
    fd = libc.inotify_init1(os.O_CLOEXEC)
    if fd == -1:
        raise OSError(ctypes.get_errno(), 'inotify_init1')
    try:
        result = libc.inotify_add_watch(fd, b'/proc', 0x00000fff)
        error = ctypes.get_errno()
    finally:
        os.close(fd)
    ctypes.set_errno(error)
else:
    raise ValueError(operation)
if result == -1:
    error = ctypes.get_errno()
    if error not in (errno.EPERM, errno.EACCES, errno.ENOSYS):
        raise OSError(error, operation)
    print('restricted')
else:
    print('available')
