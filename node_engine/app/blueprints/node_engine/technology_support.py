import subprocess


def verify_technology_support():

    technology_info = []
    if verify_technology_support_docker():
        technology_info.append('docker')
    if verify_technology_support_unikernel_mirageos():
        technology_info.append('mirage')
    return technology_info


def verify_technology_support_docker():
    try:
        result = subprocess.run(['docker', '--version'], stdout=subprocess.PIPE).stdout.decode('utf-8')
        if result.startswith('Docker version'):
            print('Docker support')
            return True
    except Exception as e:
        return False


def verify_technology_support_unikernel_mirageos():
    try:
        result = subprocess.run(['mirage', '--version'], stdout=subprocess.PIPE).stdout.decode('utf-8')
        if result.startswith('v3.10.1'):
            print('MirageOS support')
            return True
    except Exception as e:
        return False
