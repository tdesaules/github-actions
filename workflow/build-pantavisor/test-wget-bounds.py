"""Run the actual patched Wget functions with ASan/UBSan and a vulnerable control."""
import os
from pathlib import Path
import re
import subprocess

root = Path('/backport-source/source')
# dpkg-buildpackage restores the pristine tree on completion. Restore the same
# quilt series used for compilation so the probe tests the packaged functions.
subprocess.run(['dpkg-source', '--before-build', '.'], cwd=root, check=True)
prefix = r'''
#include <assert.h>
#include <stdint.h>
#include <stdlib.h>
#include <string.h>
#include <signal.h>
#include <sys/resource.h>
#include <sys/wait.h>
#include <unistd.h>
#define xmalloc malloc
#define xfree free
static void *xmemdup0(const void *data, size_t n) {
  char *p = malloc(n+1); assert(p); memcpy(p, data, n); p[n]=0; return p;
}
'''


def function(file, name, result):
    text = file.read_text()
    match = re.search(r'^' + name + r'\s*\([^\n]+\)\s*\{.*?^\}', text, re.M | re.S)
    if not match:
        raise ValueError('Missing source function: ' + name)
    return result + '\n' + match[0] + '\n'


def compile_probe(name, code):
    source = root / (name + '.c')
    binary = root / name
    source.write_text(prefix + code)
    subprocess.run(['gcc', '-g', '-O1', '-fsanitize=address,undefined',
                    '-fno-sanitize-recover=all', str(source), '-o', str(binary)], check=True)
    return subprocess.run([str(binary)], text=True, capture_output=True,
                          env=os.environ | {'ASAN_OPTIONS': 'detect_leaks=0'}, timeout=30)


baseline = function(root / '.pc/pantavisor-bounds.patch/src/metalink.c', 'clean_metalink_string', 'void')
old = compile_probe('wget-bounds-baseline', baseline + r'''
int main(void) { char *p=strdup(""); clean_metalink_string(&p); free(p); return 0; }
''')
(root / 'wget-bounds-baseline.log').write_text(old.stdout + old.stderr)
assert old.returncode != 0 and 'heap-buffer-overflow' in old.stderr, old.stderr

code = function(root / 'src/metalink.c', 'clean_metalink_string', 'void')
code += function(root / 'src/convert.c', 'checked_quote_add', 'size_t')
code += function(root / 'src/convert.c', 'html_quote_string', 'char *')
code += r'''
int main(void) {
  const char *inputs[]={"", " ", " \t\r\n", "name", " name  ", "two words"};
  const char *expected[]={"", "", "", "name", "name", "two words"};
  for (size_t i=0; i<sizeof(inputs)/sizeof(inputs[0]); i++) {
    char *p=strdup(inputs[i]); clean_metalink_string(&p);
    assert(strcmp(p,expected[i])==0); free(p);
  }
  char *p=html_quote_string("&<>\" ");
  assert(strcmp(p,"&amp;&lt;&gt;&quot;&#32;")==0); free(p);
  p=html_quote_string(""); assert(p[0]==0); free(p);
  assert(checked_quote_add(SIZE_MAX-1,1)==SIZE_MAX);
  struct rlimit limit={0,0}; assert(setrlimit(RLIMIT_CORE,&limit)==0);
  pid_t child=fork(); assert(child>=0);
  if (!child) { checked_quote_add(SIZE_MAX,1); _exit(0); }
  int status; assert(waitpid(child,&status,0)==child);
  assert(WIFSIGNALED(status) && WTERMSIG(status)==SIGABRT);
  return 0;
}
'''
fixed = compile_probe('wget-bounds-fixed', code)
assert fixed.returncode == 0, fixed.stdout + fixed.stderr
print('PASS Wget bounds: original underflow reproduced; fixed scans, entity lengths and size overflow checked under ASan/UBSan')
