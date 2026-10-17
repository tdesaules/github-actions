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
