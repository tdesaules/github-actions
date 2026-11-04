# Merge Pantavisor qualification

Requires independent successful base and k0s receipts for the same System
revision, version and file inventory. Rechecks both against the actual downloaded
artifacts and emits the combined receipt. The release job uploads that receipt
and the unchanged outputs before finalizing the tag.
