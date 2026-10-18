#!/usr/bin/env python3
"""Validate native actions and the narrowly scoped Buildah composite exception."""
from pathlib import Path
import re

import yaml


class UniqueLoader(yaml.SafeLoader):
    pass


def mapping(loader, node, deep=False):
    result = {}
    for key_node, value_node in node.value:
        key = loader.construct_object(key_node, deep=deep)
        if key in result:
            raise ValueError(f"duplicate YAML key: {key}")
        result[key] = loader.construct_object(value_node, deep=deep)
    return result


UniqueLoader.add_constructor(yaml.resolver.BaseResolver.DEFAULT_MAPPING_TAG, mapping)


def validate(path):
    data = yaml.load(path.read_text(), Loader=UniqueLoader)
    for name in ("name", "description"):
        if not isinstance(data.get(name), str) or not data[name]:
            raise ValueError(f"{path}: {name} must be a non-empty string")
    unknown = set(data) - {"name", "description", "author", "inputs", "outputs", "runs", "branding"}
    if unknown:
        raise ValueError(f"{path}: unknown metadata fields {unknown}")
    runs = data["runs"]
    builder = path.parent.name == 'build-image'
    if builder:
        if runs.get('using') != 'composite' or len(runs.get('steps', [])) != 1:
            raise ValueError(f'{path}: Buildah requires the qualified launcher')
        env = runs['steps'][0]['env']
        if env['BUILDER_OPERATION'] != path.parent.name or not env['BUILDER_IMAGE'].startswith('docker://'):
            raise ValueError(f'{path}: invalid builder image or operation')
    elif runs.get("using") != "docker" or not runs.get("image", "").startswith("docker://"):
        raise ValueError(f"{path}: must be a native prebuilt Docker action")
    if not builder and runs.get('args') != ['--'+name+'=${{ inputs.'+name+' }}' for name in data.get('inputs', {})]:
        raise ValueError(f'{path}: each input must be forwarded as an explicit named argument')
    for filename in ('Containerfile', 'main.go'):
        if not (path.parent / filename).is_file():
            raise ValueError(f'{path}: action must own {filename}')
    for field in ("inputs", "outputs"):
        for name, spec in data.get(field, {}).items():
            if not re.fullmatch(r"[a-z][a-z0-9-]*", name):
                raise ValueError(f"{path}: invalid {field} name: {name}")
            if not isinstance(spec.get("description"), str) or not spec["description"]:
                raise ValueError(f"{path}: undocumented {field}: {name}")
            if field == "inputs":
                if "required" in spec and not isinstance(spec["required"], bool):
                    raise ValueError(f"{path}: required must be boolean")
                if "default" in spec and not isinstance(spec["default"], str):
                    raise ValueError(f"{path}: defaults must be strings (quote boolean/numeric values)")
    return data


if __name__ == "__main__":
    paths = sorted([*Path('oci').glob('*/action.yml'), *Path('check').glob('*/action.yml')])
    for path in paths:
        validate(path)
    print(f"Validated {len(paths)} action metadata files (native tools and qualified Buildah launchers)")
