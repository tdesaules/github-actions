"""Producer identity checks shared by the fixed KAS build wrapper and its tests."""


def validate_producer(config, project):
    if project == 'core':
        name = config.get('name')
    elif project == 'k0s':
        # k0s describes deployable variants under profiles, not a root name.
        name = config.get('profiles', {}).get('base', {}).get('name')
    else:
        raise ValueError('Unsupported producer project')
    if name != project:
        raise ValueError('Project input does not match producer metadata')
