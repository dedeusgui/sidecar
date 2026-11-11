import sys
sys.path.insert(0, '/home/dedeusgui/orca/sidecar/bench/lab')
import lab
import pathlib
network = lab.sandbox_network_env
def network_with_dns(environ=None):
    env, mounts = network(environ)
    target = pathlib.Path('/etc/resolv.conf').resolve()
    mounts.append((target, str(target)))
    return env, mounts
lab.sandbox_network_env = network_with_dns
original = lab.parse_arms
def parse(spec):
    result = []
    for arm in spec.split(','):
        if arm == 'V35':
            result.append({'name': 'V35', 'engine': 'v3.5', 'env': {}, 'explore': False})
        else:
            result.extend(original(arm))
    return result
lab.parse_arms = parse
lab.main()
