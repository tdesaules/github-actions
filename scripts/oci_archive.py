"""Read a single-platform OCI archive without extracting untrusted paths."""
import hashlib
import json
import tarfile


def inspect_archive(path):
    with tarfile.open(path) as tar:
        index = json.load(tar.extractfile("index.json"))
        if len(index.get("manifests", [])) != 1:
            raise ValueError(f"expected one platform manifest in {path}")

        def read_blob(descriptor):
            algorithm, digest = descriptor["digest"].split(":", 1)
            if algorithm != "sha256" or len(digest) != 64 or any(c not in "0123456789abcdef" for c in digest):
                raise ValueError("invalid OCI descriptor digest")
            with tar.extractfile("blobs/sha256/"+digest) as stream:
                checksum = hashlib.file_digest(stream, "sha256").hexdigest()
            if checksum != digest:
                raise ValueError("OCI blob digest mismatch")
            member = tar.getmember("blobs/sha256/"+digest)
            if member.size != descriptor["size"]:
                raise ValueError("OCI blob size mismatch")
            return json.load(tar.extractfile(member))

        manifest = read_blob(index["manifests"][0])
        config = read_blob(manifest["config"])
        if config.get("os") != "linux" or config.get("architecture") not in ("amd64", "arm64"):
            raise ValueError("archive must contain linux/amd64 or linux/arm64")
        return {"digest": index["manifests"][0]["digest"], "image_id": manifest["config"]["digest"], "arch": config["architecture"]}


def check_temporary_directory(path):
    """Check the delivered layer metadata, not the assembly-stage permissions."""
    mode = None
    with tarfile.open(path) as archive:
        index = json.load(archive.extractfile('index.json'))
        manifest = json.load(archive.extractfile('blobs/sha256/'+index['manifests'][0]['digest'].split(':')[1]))
        for descriptor in manifest['layers']:
            with tarfile.open(fileobj=archive.extractfile('blobs/sha256/'+descriptor['digest'].split(':')[1])) as layer:
                for entry in layer:
                    if entry.name.rstrip('/') == 'tmp':
                        mode = entry.mode
    if mode != 0o1777:
        raise ValueError(f'{path}: /tmp must be 01777 for the runner UID, got {mode!r}')
