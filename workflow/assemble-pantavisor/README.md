# Assemble Pantavisor

Invokes System's fixed `assemble` operation against public BSP/Core exports and,
for the k0s profile only, a k0s export. System validates compatibility and original
component identities. The output is a new public platform directory.

This action neither renders private device credentials nor starts a VM. The
producer checkouts are not needed. Publish its bootstrap image before use.
