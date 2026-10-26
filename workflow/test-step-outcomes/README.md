# Test step outcomes

Asserts that specified native action steps actually failed, rather than succeeded
or skipped, and that failed operations left no success outputs.

Inputs: `outcomes` (serialized GitHub steps context), `expected-failures` and
`absent-files` (JSON arrays, default `[]`). Outputs `verified`. Explicit flags
override `INPUT_*`. No shell or arbitrary command is executed.

The fixed Go scratch image was published in `1.2.0` and is called by `test`.
