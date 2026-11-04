# Import Pantavisor candidate

Checks the candidate metadata against the base checkout, verifies the Git bundle,
fetches its candidate ref, validates its commit, and checks out that exact commit.
Also checks VERSION. It never reconstructs the commit or fetches a moving branch.
