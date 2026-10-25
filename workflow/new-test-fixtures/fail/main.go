// This native fixture proves a failed RUN propagates its exit status without a network pull.
package main

import "os"

func main() { os.Exit(42) }
